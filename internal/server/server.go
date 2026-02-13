package server

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"
	"nhooyr.io/websocket"

	"github.com/gleicon/remo/internal/auth"
	"github.com/gleicon/remo/internal/protocol"
	"github.com/gleicon/remo/internal/store"
)

type Mode string

const (
	ModeStandalone Mode = "standalone"
	ModeProxy      Mode = "behind-proxy"
)

type Config struct {
	Domain          string
	SubdomainPrefix string
	Logger          zerolog.Logger
	ReadTimeout     time.Duration
	Authorizer      *auth.AuthorizedKeys
	Mode            Mode
	TLSCertFile     string
	TLSKeyFile      string
	TrustedProxies  []*net.IPNet
	TrustedHops     int
	AdminSecret     string
	Store           *store.Store
	AutoReserve     bool
	AllowRandom     bool
}

type Server struct {
	cfg      Config
	log      zerolog.Logger
	registry *registry
	store    *store.Store
	started  time.Time
	metrics  *metrics
}

func New(cfg Config) *Server {
	if cfg.ReadTimeout == 0 {
		cfg.ReadTimeout = 30 * time.Second
	}
	if cfg.Mode == "" {
		cfg.Mode = ModeProxy
	}
	if cfg.TrustedHops == 0 {
		cfg.TrustedHops = 1
	}
	return &Server{cfg: cfg, log: cfg.Logger, registry: newRegistry(), store: cfg.Store, started: time.Now(), metrics: newMetrics()}
}

func (s *Server) routingDomain() string {
	if s.cfg.SubdomainPrefix != "" {
		return s.cfg.SubdomainPrefix + "." + s.cfg.Domain
	}
	return s.cfg.Domain
}

func generateRandomSubdomain() (string, error) {
	buf := make([]byte, 4)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/tunnel", s.handleTunnel)
	mux.HandleFunc("/healthz", s.handleHealth)
	mux.HandleFunc("/status", s.handleStatus)
	mux.HandleFunc("/metrics", s.handleMetrics)
	mux.HandleFunc("/", s.handleProxy)
	return mux
}

func (s *Server) Run(ctx context.Context, addr string) error {
	httpServer := &http.Server{
		Addr:              addr,
		Handler:           s.Handler(),
		ReadHeaderTimeout: s.cfg.ReadTimeout,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdownCtx)
	}()
	var err error
	switch s.cfg.Mode {
	case ModeStandalone:
		if s.cfg.TLSCertFile == "" || s.cfg.TLSKeyFile == "" {
			return errors.New("standalone mode requires tls cert and key")
		}
		err = httpServer.ListenAndServeTLS(s.cfg.TLSCertFile, s.cfg.TLSKeyFile)
	default:
		err = httpServer.ListenAndServe()
	}
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func (s *Server) HasTunnel(subdomain string) bool {
	return s.registry.has(subdomain)
}

func (s *Server) handleTunnel(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		s.log.Error().Err(err).Msg("websocket accept failed")
		return
	}
	subdomain, _, pubKeyStr, err := s.performHandshake(r.Context(), conn)
	if err != nil {
		conn.Close(websocket.StatusPolicyViolation, err.Error())
		return
	}
	log := s.log.With().Str("subdomain", subdomain).Str("pubkey", pubKeyStr).Logger()
	tunnel := newTunnel(subdomain, pubKeyStr, conn, log)
	if !s.registry.register(subdomain, tunnel) {
		conn.Close(websocket.StatusPolicyViolation, "subdomain busy")
		return
	}
	log.Info().Msg("tunnel connected")
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		defer cancel()
		tunnel.runReader(ctx)
	}()
	go tunnel.keepalive(ctx)
	<-ctx.Done()
	s.registry.unregister(subdomain, tunnel)
	if s.store != nil {
		s.store.LogEvent(context.Background(), "disconnect", subdomain, pubKeyStr)
	}
}

func (s *Server) performHandshake(parent context.Context, conn *websocket.Conn) (string, ed25519.PublicKey, string, error) {
	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	defer cancel()
	env, err := protocol.Read(ctx, conn)
	if err != nil {
		return "", nil, "", err
	}
	if env.Type != protocol.TypeHello || env.Hello == nil {
		return "", nil, "", errors.New("invalid handshake")
	}
	payload := env.Hello
	randomAssigned := false
	originalSubdomain := payload.Subdomain
	if payload.Subdomain == "" {
		if !s.cfg.AllowRandom {
			return "", nil, "", errors.New("missing subdomain (random not enabled)")
		}
		randomAssigned = true
	}
	if !auth.FreshTimestamp(payload.Timestamp) {
		return "", nil, "", errors.New("stale handshake")
	}
	pubBytes, err := base64.StdEncoding.DecodeString(payload.PublicKey)
	if err != nil {
		return "", nil, "", errors.New("invalid public key")
	}
	if len(pubBytes) != ed25519.PublicKeySize {
		return "", nil, "", errors.New("invalid public key size")
	}
	sigBytes, err := base64.StdEncoding.DecodeString(payload.Signature)
	if err != nil {
		return "", nil, "", errors.New("invalid signature")
	}
	pub := ed25519.PublicKey(pubBytes)
	message := auth.BuildHandshakeMessage(originalSubdomain, payload.Timestamp)
	if !ed25519.Verify(pub, message, sigBytes) {
		return "", nil, "", errors.New("signature mismatch")
	}
	if randomAssigned {
		name, err := generateRandomSubdomain()
		if err != nil {
			return "", nil, "", errors.New("failed to generate subdomain")
		}
		for s.registry.has(name) {
			name, err = generateRandomSubdomain()
			if err != nil {
				return "", nil, "", errors.New("failed to generate subdomain")
			}
		}
		payload.Subdomain = name
	}
	if s.cfg.Authorizer != nil && !s.cfg.Authorizer.Allow(pub, payload.Subdomain) {
		return "", nil, "", fmt.Errorf("unauthorized subdomain %s", payload.Subdomain)
	}
	pubKeyStr := base64.StdEncoding.EncodeToString(pub)
	if s.store != nil {
		owner, err := s.store.ReservationOwner(ctx, payload.Subdomain)
		if err != nil {
			return "", nil, "", err
		}
		if owner != "" && owner != pubKeyStr {
			return "", nil, "", fmt.Errorf("subdomain reserved")
		}
		if owner == "" && s.cfg.AutoReserve {
			if err := s.store.ReserveSubdomain(ctx, payload.Subdomain, pubKeyStr); err != nil {
				return "", nil, "", err
			}
		}
		go s.store.LogEvent(context.Background(), "handshake", payload.Subdomain, pubKeyStr)
	}
	readyCtx, readyCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer readyCancel()
	if err := protocol.Write(readyCtx, conn, &protocol.Envelope{Type: protocol.TypeReady, Ready: &protocol.ReadyPayload{Message: "ready", Subdomain: payload.Subdomain}}); err != nil {
		return "", nil, "", err
	}
	return payload.Subdomain, pub, pubKeyStr, nil
}

func (s *Server) handleProxy(w http.ResponseWriter, r *http.Request) {
	subdomain := s.extractSubdomain(r.Host)
	if subdomain == "" {
		http.Error(w, "missing subdomain", http.StatusBadRequest)
		return
	}
	if s.cfg.TrustedHops > 0 && !s.validateHops(r) {
		http.Error(w, "too many proxy hops", http.StatusBadRequest)
		return
	}
	start := time.Now()
	tunnel, ok := s.registry.get(subdomain)
	if !ok {
		http.Error(w, "tunnel not available", http.StatusBadGateway)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, protocol.MaxBodyBytes))
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}
	_ = r.Body.Close()
	headers := s.forwardHeaders(r, subdomain)
	requestPayload := &protocol.RequestPayload{
		Method:  r.Method,
		Target:  r.URL.RequestURI(),
		Headers: headers,
		Body:    body,
	}
	ctx, cancel := context.WithTimeout(r.Context(), s.cfg.ReadTimeout)
	defer cancel()
	resp, err := tunnel.sendRequest(ctx, requestPayload)
	if err != nil {
		s.log.Error().Err(err).Str("subdomain", subdomain).Msg("dispatch failed")
		s.metrics.Record(subdomain, int64(len(body)), 0, time.Since(start), true)
		http.Error(w, "tunnel dispatch failed", http.StatusBadGateway)
		return
	}
	for key, values := range resp.Headers {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(resp.Status)
	if len(resp.Body) > 0 {
		w.Write(resp.Body)
	}
	s.metrics.Record(subdomain, int64(len(body)), int64(len(resp.Body)), time.Since(start), false)
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeAdmin(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	status := s.snapshot(r.Context())
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(status); err != nil {
		s.log.Error().Err(err).Msg("status encode failed")
	}
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeAdmin(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	status := s.snapshot(r.Context())
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	fmt.Fprintf(w, "remo_active_tunnels %d\n", status.ActiveTunnels)
	fmt.Fprintf(w, "remo_authorized_keys %d\n", status.AuthorizedKeys)
	fmt.Fprintf(w, "remo_reservations %d\n", status.Reservations)
}

func (s *Server) extractSubdomain(host string) string {
	host = strings.TrimSpace(host)
	if host == "" {
		return ""
	}
	if idx := strings.Index(host, ":"); idx >= 0 {
		host = host[:idx]
	}
	domain := s.routingDomain()
	if !strings.HasSuffix(host, domain) {
		return ""
	}
	trimmed := strings.TrimSuffix(host, domain)
	trimmed = strings.TrimSuffix(trimmed, ".")
	parts := strings.Split(trimmed, ".")
	if len(parts) == 0 || parts[0] == "" {
		return ""
	}
	return parts[len(parts)-1]
}

func (s *Server) forwardHeaders(r *http.Request, subdomain string) map[string][]string {
	headers := cloneHeader(r.Header)
	clientIP := s.peerAddress(r)
	forwardedFor := clientIP
	if prior := r.Header.Get("X-Forwarded-For"); prior != "" && s.trustedProxy(r) {
		forwardedFor = prior + ", " + clientIP
	}
	headers["X-Forwarded-For"] = []string{forwardedFor}
	proto := "http"
	if s.cfg.Mode == ModeStandalone {
		proto = "https"
	} else if s.trustedProxy(r) {
		if hdr := r.Header.Get("X-Forwarded-Proto"); hdr != "" {
			proto = hdr
		}
	}
	headers["X-Forwarded-Proto"] = []string{proto}
	headers["X-Remo-Subdomain"] = []string{subdomain}
	return headers
}

func (s *Server) validateHops(r *http.Request) bool {
	if !s.trustedProxy(r) {
		return true
	}
	xff := r.Header.Get("X-Forwarded-For")
	if xff == "" {
		return true
	}
	count := len(strings.Split(xff, ","))
	return count <= s.cfg.TrustedHops
}

func (s *Server) trustedProxy(r *http.Request) bool {
	if len(s.cfg.TrustedProxies) == 0 {
		return false
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(strings.TrimSpace(host))
	if ip == nil {
		return false
	}
	for _, cidr := range s.cfg.TrustedProxies {
		if cidr.Contains(ip) {
			return true
		}
	}
	return false
}

func (s *Server) peerAddress(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

func cloneHeader(h http.Header) map[string][]string {
	result := make(map[string][]string, len(h))
	for k, values := range h {
		copyValues := make([]string, len(values))
		copy(copyValues, values)
		result[k] = copyValues
	}
	return result
}

type metrics struct {
	requests   atomic.Uint64
	errors     atomic.Uint64
	bytesIn    atomic.Uint64
	bytesOut   atomic.Uint64
	latencySum atomic.Uint64
	subdomains sync.Map
}

type subdomainStats struct {
	requests atomic.Uint64
	errors   atomic.Uint64
}

type metricsSnapshot struct {
	TotalRequests uint64
	TotalErrors   uint64
	BytesIn       uint64
	BytesOut      uint64
	AvgLatencyMs  float64
}

func newMetrics() *metrics {
	return &metrics{}
}

func (m *metrics) Record(subdomain string, reqBytes, respBytes int64, latency time.Duration, failed bool) {
	m.requests.Add(1)
	m.bytesIn.Add(uint64(max64(reqBytes, 0)))
	m.bytesOut.Add(uint64(max64(respBytes, 0)))
	m.latencySum.Add(uint64(latency.Microseconds()))
	stat := m.getSubdomain(subdomain)
	stat.requests.Add(1)
	if failed {
		m.errors.Add(1)
		stat.errors.Add(1)
	}
}

func (m *metrics) getSubdomain(name string) *subdomainStats {
	value, _ := m.subdomains.LoadOrStore(name, &subdomainStats{})
	return value.(*subdomainStats)
}

func (m *metrics) Snapshot() metricsSnapshot {
	total := m.requests.Load()
	avg := 0.0
	if total > 0 {
		avg = float64(m.latencySum.Load()) / float64(total) / 1000.0
	}
	return metricsSnapshot{
		TotalRequests: total,
		TotalErrors:   m.errors.Load(),
		BytesIn:       m.bytesIn.Load(),
		BytesOut:      m.bytesOut.Load(),
		AvgLatencyMs:  avg,
	}
}

func max64(value int64, min int64) int64 {
	if value < min {
		return min
	}
	return value
}

func (s *Server) authorizeAdmin(r *http.Request) bool {
	if s.cfg.AdminSecret == "" {
		return false
	}
	const prefix = "Bearer "
	authHeader := r.Header.Get("Authorization")
	if !strings.HasPrefix(authHeader, prefix) {
		return false
	}
	token := strings.TrimSpace(strings.TrimPrefix(authHeader, prefix))
	return subtle.ConstantTimeCompare([]byte(token), []byte(s.cfg.AdminSecret)) == 1
}

type statusResponse struct {
	Domain         string    `json:"domain"`
	Mode           Mode      `json:"mode"`
	StartedAt      time.Time `json:"started_at"`
	UptimeSeconds  int64     `json:"uptime_seconds"`
	ActiveTunnels  int       `json:"active_tunnels"`
	Subdomains     []string  `json:"subdomains"`
	AuthorizedKeys int       `json:"authorized_keys"`
	Reservations   int       `json:"reservations"`
	TotalRequests  uint64    `json:"total_requests"`
	TotalErrors    uint64    `json:"total_errors"`
	BytesIn        uint64    `json:"bytes_in"`
	BytesOut       uint64    `json:"bytes_out"`
	AvgLatencyMs   float64   `json:"avg_latency_ms"`
}

func (s *Server) snapshot(ctx context.Context) statusResponse {
	subdomains := s.registry.list()
	stats := store.StoreStats{}
	if s.store != nil {
		if st, err := s.store.Stats(ctx); err == nil {
			stats = st
		}
	}
	metricsSnapshot := s.metrics.Snapshot()
	uptime := time.Since(s.started)
	return statusResponse{
		Domain:         s.cfg.Domain,
		Mode:           s.cfg.Mode,
		StartedAt:      s.started,
		UptimeSeconds:  int64(uptime.Seconds()),
		ActiveTunnels:  len(subdomains),
		Subdomains:     subdomains,
		AuthorizedKeys: stats.AuthorizedKeys,
		Reservations:   stats.Reservations,
		TotalRequests:  metricsSnapshot.TotalRequests,
		TotalErrors:    metricsSnapshot.TotalErrors,
		BytesIn:        metricsSnapshot.BytesIn,
		BytesOut:       metricsSnapshot.BytesOut,
		AvgLatencyMs:   metricsSnapshot.AvgLatencyMs,
	}
}
