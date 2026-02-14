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
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gliderlabs/ssh"
	"github.com/rs/zerolog"
	xssh "golang.org/x/crypto/ssh"

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
	SSHHostKey      string
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
	if cfg.SSHHostKey == "" {
		cfg.SSHHostKey = "0.0.0.0:22"
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

	errChan := make(chan error, 2)

	go func() {
		errChan <- s.runSSHServer(ctx)
	}()

	go func() {
		switch s.cfg.Mode {
		case ModeStandalone:
			if s.cfg.TLSCertFile == "" || s.cfg.TLSKeyFile == "" {
				errChan <- errors.New("standalone mode requires tls cert and key")
				return
			}
			errChan <- httpServer.ListenAndServeTLS(s.cfg.TLSCertFile, s.cfg.TLSKeyFile)
		default:
			errChan <- httpServer.ListenAndServe()
		}
	}()

	var err error
	select {
	case <-ctx.Done():
		return nil
	case err = <-errChan:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	}
	return nil
}

func (s *Server) runSSHServer(ctx context.Context) error {
	if s.cfg.SSHHostKey == "" {
		return errors.New("ssh-host-key is required")
	}

	hostKeyData, err := os.ReadFile(s.cfg.SSHHostKey)
	if err != nil {
		return fmt.Errorf("read SSH host key: %w", err)
	}

	hostKey, err := xssh.ParsePrivateKey(hostKeyData)
	if err != nil {
		return fmt.Errorf("parse SSH host key: %w", err)
	}

	sshServer := &ssh.Server{
		Addr:    ":22",
		Handler: s.handleSSH,
	}
	sshServer.AddHostKey(hostKey)
	sshServer.SetOption(ssh.PublicKeyAuth(s.handlePublicKeyAuth))

	errChan := make(chan error, 1)
	go func() {
		errChan <- sshServer.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		return sshServer.Shutdown(context.Background())
	case err := <-errChan:
		return err
	}
}

func (s *Server) handlePublicKeyAuth(ctx ssh.Context, key ssh.PublicKey) bool {
	pubKeyStr := base64.StdEncoding.EncodeToString(key.Marshal())
	s.log.Debug().Str("pubkey", pubKeyStr).Str("client", ctx.ClientVersion()).Msg("public key auth attempt")

	if s.cfg.Authorizer != nil {
		pub := ed25519.PublicKey(key.Marshal())
		if !s.cfg.Authorizer.Allow(pub, "") {
			s.log.Warn().Str("pubkey", pubKeyStr).Msg("public key not authorized")
			return false
		}
	}

	ctx.SetValue("pubkey", pubKeyStr)
	return true
}

func (s *Server) handleSSH(session ssh.Session) {
	ctx := context.Background()
	pubKeyStr, _ := session.Context().Value("pubkey").(string)

	s.log.Info().Str("client", session.RemoteAddr().String()).Msg("new SSH connection")

	// Session implements io.ReadWriteCloser - use it directly as the channel
	channel := session

	helloCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	env, err := protocol.Read(helloCtx, channel)
	if err != nil {
		s.log.Error().Err(err).Msg("failed to read hello from client")
		return
	}
	if env.Type != protocol.TypeHello || env.Hello == nil {
		s.log.Error().Msg("invalid hello message")
		return
	}

	payload := env.Hello
	originalSubdomain := payload.Subdomain
	randomAssigned := false

	if payload.Subdomain == "" {
		if !s.cfg.AllowRandom {
			protocol.Write(ctx, channel, &protocol.Envelope{
				Type:  protocol.TypeError,
				Error: "missing subdomain (random not enabled)",
			})
			return
		}
		randomAssigned = true
	}

	if !auth.FreshTimestamp(payload.Timestamp) {
		protocol.Write(ctx, channel, &protocol.Envelope{
			Type:  protocol.TypeError,
			Error: "stale handshake",
		})
		return
	}

	pubBytes, err := base64.StdEncoding.DecodeString(payload.PublicKey)
	if err != nil {
		protocol.Write(ctx, channel, &protocol.Envelope{
			Type:  protocol.TypeError,
			Error: "invalid public key",
		})
		return
	}
	if len(pubBytes) != ed25519.PublicKeySize {
		protocol.Write(ctx, channel, &protocol.Envelope{
			Type:  protocol.TypeError,
			Error: "invalid public key size",
		})
		return
	}

	sigBytes, err := base64.StdEncoding.DecodeString(payload.Signature)
	if err != nil {
		protocol.Write(ctx, channel, &protocol.Envelope{
			Type:  protocol.TypeError,
			Error: "invalid signature",
		})
		return
	}

	pub := ed25519.PublicKey(pubBytes)
	message := auth.BuildHandshakeMessage(originalSubdomain, payload.Timestamp)
	if !ed25519.Verify(pub, message, sigBytes) {
		protocol.Write(ctx, channel, &protocol.Envelope{
			Type:  protocol.TypeError,
			Error: "signature mismatch",
		})
		return
	}

	if randomAssigned {
		name, err := generateRandomSubdomain()
		if err != nil {
			protocol.Write(ctx, channel, &protocol.Envelope{
				Type:  protocol.TypeError,
				Error: "failed to generate subdomain",
			})
			return
		}
		for s.registry.has(name) {
			name, err = generateRandomSubdomain()
			if err != nil {
				protocol.Write(ctx, channel, &protocol.Envelope{
					Type:  protocol.TypeError,
					Error: "failed to generate subdomain",
				})
				return
			}
		}
		payload.Subdomain = name
	}

	if s.cfg.Authorizer != nil && !s.cfg.Authorizer.Allow(pub, payload.Subdomain) {
		protocol.Write(ctx, channel, &protocol.Envelope{
			Type:  protocol.TypeError,
			Error: fmt.Sprintf("unauthorized subdomain %s", payload.Subdomain),
		})
		return
	}

	pubKeyStr = base64.StdEncoding.EncodeToString(pub)
	if s.store != nil {
		owner, err := s.store.ReservationOwner(ctx, payload.Subdomain)
		if err != nil {
			protocol.Write(ctx, channel, &protocol.Envelope{
				Type:  protocol.TypeError,
				Error: err.Error(),
			})
			return
		}
		if owner != "" && owner != pubKeyStr {
			protocol.Write(ctx, channel, &protocol.Envelope{
				Type:  protocol.TypeError,
				Error: "subdomain reserved",
			})
			return
		}
		if owner == "" && s.cfg.AutoReserve {
			if err := s.store.ReserveSubdomain(ctx, payload.Subdomain, pubKeyStr); err != nil {
				protocol.Write(ctx, channel, &protocol.Envelope{
					Type:  protocol.TypeError,
					Error: err.Error(),
				})
				return
			}
		}
		go s.store.LogEvent(context.Background(), "connect", payload.Subdomain, pubKeyStr)
	}

	if err := protocol.Write(ctx, channel, &protocol.Envelope{
		Type: protocol.TypeReady,
		Ready: &protocol.ReadyPayload{
			Message:   "ready",
			Subdomain: payload.Subdomain,
		},
	}); err != nil {
		s.log.Error().Err(err).Msg("failed to send ready")
		return
	}

	log := s.log.With().Str("subdomain", payload.Subdomain).Str("pubkey", pubKeyStr).Logger()
	tunnel := newTunnel(payload.Subdomain, pubKeyStr, channel, log)
	if !s.registry.register(payload.Subdomain, tunnel) {
		log.Error().Msg("subdomain already in use")
		protocol.Write(ctx, channel, &protocol.Envelope{
			Type:  protocol.TypeError,
			Error: "subdomain busy",
		})
		return
	}

	log.Info().Msg("tunnel connected and registered")

	tunnelCtx, tunnelCancel := context.WithCancel(ctx)
	go func() {
		tunnel.runReader(tunnelCtx)
		tunnelCancel()
	}()
	go tunnel.keepalive(tunnelCtx)

	<-tunnelCtx.Done()
	s.registry.unregister(payload.Subdomain, tunnel)
	if s.store != nil {
		s.store.LogEvent(context.Background(), "disconnect", payload.Subdomain, pubKeyStr)
	}
}

func (s *Server) HasTunnel(subdomain string) bool {
	return s.registry.has(subdomain)
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
	remoteAddr := s.peerAddress(r)
	s.log.Debug().Str("subdomain", subdomain).Str("method", r.Method).Str("target", r.URL.RequestURI()).Str("remote", remoteAddr).Msg("incoming request")
	tunnel, ok := s.registry.get(subdomain)
	if !ok {
		s.log.Warn().Str("subdomain", subdomain).Msg("tunnel not found for subdomain")
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
		s.log.Error().Err(err).Str("subdomain", subdomain).Msg("dispatch failed - could not send request to tunnel")
		s.metrics.Record(subdomain, int64(len(body)), 0, time.Since(start), true)
		http.Error(w, "tunnel dispatch failed", http.StatusBadGateway)
		return
	}
	s.log.Debug().Str("subdomain", subdomain).Int("status", resp.Status).Dur("latency", time.Since(start)).Msg("request completed")
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
