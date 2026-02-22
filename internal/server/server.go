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
	"net"
	"net/http"
	"net/http/httputil"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"

	"github.com/gleicon/remo/internal/auth"
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
	TunnelTimeout   time.Duration // Timeout for tunnel health checks (default 5m)
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
	cfg           Config
	log           zerolog.Logger
	registry      *registry
	store         *store.Store
	started       time.Time
	metrics       *metrics
	httpClient    *http.Client
	requestEvents []RequestEvent
	eventsMu      sync.RWMutex
	maxEvents     int
}

// RequestEvent represents a captured HTTP request for TUI logging
type RequestEvent struct {
	Time     time.Time     `json:"time"`
	Method   string        `json:"method"`
	Path     string        `json:"path"`
	Status   int           `json:"status"`
	Latency  time.Duration `json:"latency"`
	Remote   string        `json:"remote"`
	BytesIn  int           `json:"bytes_in"`
	BytesOut int           `json:"bytes_out"`
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
	if cfg.TunnelTimeout == 0 {
		cfg.TunnelTimeout = 5 * time.Minute // Default 5 minute timeout
	}
	return &Server{
		cfg:           cfg,
		log:           cfg.Logger,
		registry:      newRegistry(cfg.TunnelTimeout),
		store:         cfg.Store,
		started:       time.Now(),
		metrics:       newMetrics(),
		requestEvents: make([]RequestEvent, 0, 100),
		maxEvents:     100,
		httpClient: &http.Client{
			Timeout: cfg.ReadTimeout,
			Transport: &http.Transport{
				DialContext: (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
			},
		},
	}
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
	mux.HandleFunc("/register", s.handleRegister)
	mux.HandleFunc("/unregister", s.handleUnregister)
	mux.HandleFunc("/ping", s.handlePing)
	mux.HandleFunc("/healthz", s.handleHealth)
	mux.HandleFunc("/status", s.handleStatus)
	mux.HandleFunc("/metrics", s.handleMetrics)
	mux.HandleFunc("/events", s.handleEvents)
	mux.HandleFunc("/admin/cleanup", s.handleAdminCleanup)
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

	// Start cleanup goroutine to remove stale tunnels
	go s.cleanupLoop(ctx)

	switch s.cfg.Mode {
	case ModeStandalone:
		if s.cfg.TLSCertFile == "" || s.cfg.TLSKeyFile == "" {
			return errors.New("standalone mode requires tls cert and key")
		}
		return httpServer.ListenAndServeTLS(s.cfg.TLSCertFile, s.cfg.TLSKeyFile)
	default:
		return httpServer.ListenAndServe()
	}
}

func (s *Server) HasTunnel(subdomain string) bool {
	return s.registry.has(subdomain)
}

type registrationRequest struct {
	Subdomain  string `json:"subdomain"`
	RemotePort int    `json:"remote_port"`
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	remoteAddr := s.peerAddress(r)
	isLocalhost := remoteAddr == "127.0.0.1" || remoteAddr == "::1" || remoteAddr == "localhost"

	if !isLocalhost && s.cfg.Authorizer != nil {
		s.log.Warn().Str("remote", remoteAddr).Msg("rejecting non-local registration")
		http.Error(w, "registration must go through SSH tunnel", http.StatusForbidden)
		return
	}

	s.log.Debug().Str("remote", remoteAddr).Msg("registration request")

	var req registrationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.log.Error().Err(err).Msg("failed to decode registration request")
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	subdomain := req.Subdomain

	if subdomain == "" {
		if !s.cfg.AllowRandom {
			http.Error(w, "missing subdomain (random not enabled)", http.StatusBadRequest)
			return
		}
		for i := 0; i < 10; i++ {
			name, err := generateRandomSubdomain()
			if err != nil {
				continue
			}
			if !s.registry.has(name) {
				subdomain = name
				break
			}
		}
		if subdomain == "" {
			http.Error(w, "failed to generate subdomain", http.StatusInternalServerError)
			return
		}
	} else {
		if !s.validateSubdomain(subdomain) {
			http.Error(w, "invalid subdomain format", http.StatusBadRequest)
			return
		}
	}

	if req.RemotePort <= 0 || req.RemotePort > 65535 {
		http.Error(w, "invalid remote port", http.StatusBadRequest)
		return
	}

	authorizedKey := r.Header.Get("X-Remo-Publickey")
	authorizedKeyStr := authorizedKey

	if s.cfg.Authorizer != nil {
		if authorizedKey == "" {
			s.log.Warn().Msg("registration without public key")
			http.Error(w, "unauthorized: no public key", http.StatusForbidden)
			return
		}
		pubBytes, err := base64.StdEncoding.DecodeString(authorizedKey)
		if err != nil || len(pubBytes) != ed25519.PublicKeySize {
			s.log.Warn().Msg("invalid public key in header")
			http.Error(w, "unauthorized", http.StatusForbidden)
			return
		}
		pub := ed25519.PublicKey(pubBytes)
		if !s.cfg.Authorizer.Allow(pub, subdomain) {
			s.log.Warn().Str("pubkey", authorizedKey).Str("subdomain", subdomain).Msg("authorization denied")
			http.Error(w, "unauthorized", http.StatusForbidden)
			return
		}
	}

	if s.store != nil {
		owner, err := s.store.ReservationOwner(r.Context(), subdomain)
		if err != nil {
			s.log.Error().Err(err).Msg("failed to check reservation")
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if owner != "" && owner != authorizedKeyStr {
			s.log.Warn().Str("subdomain", subdomain).Msg("subdomain already reserved")
			http.Error(w, "subdomain reserved", http.StatusForbidden)
			return
		}
		if owner == "" && s.cfg.AutoReserve {
			if err := s.store.ReserveSubdomain(r.Context(), subdomain, authorizedKeyStr); err != nil {
				s.log.Error().Err(err).Msg("failed to reserve subdomain")
			}
		}
		go s.store.LogEvent(r.Context(), "register", subdomain, authorizedKeyStr)
	}

	if !s.registry.register(subdomain, req.RemotePort, authorizedKeyStr) {
		s.log.Warn().Str("subdomain", subdomain).Msg("subdomain already in use")
		http.Error(w, "subdomain already in use", http.StatusConflict)
		return
	}

	s.log.Info().Str("subdomain", subdomain).Int("port", req.RemotePort).Msg("tunnel registered")

	scheme := "http"
	if s.cfg.Mode == ModeStandalone {
		scheme = "https"
	}
	fullURL := fmt.Sprintf("%s://%s.%s", scheme, subdomain, s.routingDomain())

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"subdomain": subdomain,
		"url":       fullURL,
		"status":    "ok",
	})
}

// handlePing receives health check pings from clients to keep tunnel alive
func (s *Server) handlePing(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	authorizedKey := r.Header.Get("X-Remo-Publickey")
	if authorizedKey == "" {
		http.Error(w, "missing X-Remo-Publickey header", http.StatusBadRequest)
		return
	}

	subdomain := r.URL.Query().Get("subdomain")
	if subdomain == "" {
		http.Error(w, "missing subdomain parameter", http.StatusBadRequest)
		return
	}

	// Verify the tunnel exists and belongs to this pubkey
	_, pubKey, ok := s.registry.get(subdomain)
	if !ok {
		http.Error(w, "tunnel not found", http.StatusNotFound)
		return
	}

	if pubKey != authorizedKey {
		http.Error(w, "unauthorized", http.StatusForbidden)
		return
	}

	// Update last ping time
	if !s.registry.ping(subdomain) {
		http.Error(w, "tunnel not found", http.StatusNotFound)
		return
	}

	s.log.Debug().Str("subdomain", subdomain).Msg("health check ping received")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status":    "ok",
		"subdomain": subdomain,
	})
}

// handleUnregister allows clients to explicitly unregister on shutdown
func (s *Server) handleUnregister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	authorizedKey := r.Header.Get("X-Remo-Publickey")
	if authorizedKey == "" {
		http.Error(w, "missing X-Remo-Publickey header", http.StatusBadRequest)
		return
	}

	var req struct {
		Subdomain string `json:"subdomain"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Verify ownership
	_, pubKey, ok := s.registry.get(req.Subdomain)
	if !ok {
		http.Error(w, "tunnel not found", http.StatusNotFound)
		return
	}

	if pubKey != authorizedKey {
		http.Error(w, "unauthorized", http.StatusForbidden)
		return
	}

	s.registry.unregister(req.Subdomain)
	s.log.Info().Str("subdomain", req.Subdomain).Msg("tunnel unregistered")

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status":    "unregistered",
		"subdomain": req.Subdomain,
	})
}

// handleAdminCleanup allows admin to manually clean up stale tunnels
func (s *Server) handleAdminCleanup(w http.ResponseWriter, r *http.Request) {
	// Check admin secret
	secret := r.Header.Get("X-Admin-Secret")
	if secret != s.cfg.AdminSecret {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Run cleanup
	removed := s.registry.cleanup()
	total, stale := s.registry.getStats()

	s.log.Info().Int("removed", removed).Int("total", total).Int("stale", stale).Msg("admin cleanup executed")

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "ok",
		"removed": removed,
		"total":   total,
		"stale":   stale,
	})
}

func (s *Server) validateSubdomain(name string) bool {
	if len(name) < 1 || len(name) > 63 {
		return false
	}
	for _, c := range name {
		if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-') {
			return false
		}
	}
	if strings.HasPrefix(name, "-") || strings.HasSuffix(name, "-") {
		return false
	}
	return true
}

// recordingResponseWriter wraps http.ResponseWriter to capture status code and bytes written
type recordingResponseWriter struct {
	http.ResponseWriter
	statusCode   int
	bytesWritten int
}

func (rw *recordingResponseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *recordingResponseWriter) Write(b []byte) (int, error) {
	n, err := rw.ResponseWriter.Write(b)
	rw.bytesWritten += n
	return n, err
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

	port, pubKey, ok := s.registry.get(subdomain)
	if !ok {
		s.log.Warn().Str("subdomain", subdomain).Msg("tunnel not found for subdomain")
		w.Header().Set("X-Remo-Error", "no-tunnel")
		http.Error(w, "tunnel not available", http.StatusNotFound)
		return
	}

	r.Header.Set("X-Remo-Subdomain", subdomain)
	r.Header.Set("X-Remo-Pubkey", pubKey)

	director := func(req *http.Request) {
		req.URL.Scheme = "http"
		req.URL.Host = fmt.Sprintf("127.0.0.1:%d", port)
		req.Host = fmt.Sprintf("127.0.0.1:%d", port)
	}
	proxy := &httputil.ReverseProxy{Director: director}
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		s.log.Error().Err(err).Str("subdomain", subdomain).Msg("proxy error")
		s.metrics.Record(subdomain, 0, 0, time.Since(start), true)
		w.Header().Set("X-Remo-Error", "no-upstream")
		http.Error(w, "upstream unavailable", http.StatusNotFound)
	}

	rw := &recordingResponseWriter{ResponseWriter: w}
	proxy.ServeHTTP(rw, r)
	latency := time.Since(start)

	s.metrics.Record(subdomain, int64(r.ContentLength), int64(rw.bytesWritten), latency, rw.statusCode >= 500)

	s.recordEvent(RequestEvent{
		Time:     time.Now(),
		Method:   r.Method,
		Path:     r.URL.RequestURI(),
		Status:   rw.statusCode,
		Latency:  latency,
		Remote:   remoteAddr,
		BytesIn:  int(r.ContentLength),
		BytesOut: rw.bytesWritten,
	})
}

// recordEvent adds a request event to the circular buffer
func (s *Server) recordEvent(evt RequestEvent) {
	s.eventsMu.Lock()
	defer s.eventsMu.Unlock()
	s.requestEvents = append(s.requestEvents, evt)
	if len(s.requestEvents) > s.maxEvents {
		s.requestEvents = s.requestEvents[len(s.requestEvents)-s.maxEvents:]
	}
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

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Only allow access through tunnel (localhost)
	remoteAddr := s.peerAddress(r)
	isLocalhost := remoteAddr == "127.0.0.1" || remoteAddr == "::1" || remoteAddr == "localhost"
	if !isLocalhost {
		http.Error(w, "unauthorized", http.StatusForbidden)
		return
	}

	s.eventsMu.RLock()
	events := make([]RequestEvent, len(s.requestEvents))
	copy(events, s.requestEvents)
	s.eventsMu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(events); err != nil {
		s.log.Error().Err(err).Msg("failed to encode events")
	}
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

func (s *Server) authorizeAdmin(r *http.Request) bool {
	if s.cfg.AdminSecret == "" {
		return false
	}
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(auth[7:]), []byte(s.cfg.AdminSecret)) == 1
}

func (s *Server) snapshot(ctx context.Context) statusSnapshot {
	active := s.registry.list()
	snapshot := statusSnapshot{
		ActiveTunnels:  len(active),
		Tunnels:        active,
		AuthorizedKeys: 0,
		Reservations:   0,
		UptimeSeconds:  uint64(time.Since(s.started).Seconds()),
		TotalRequests:  s.metrics.requests.Load(),
		TotalErrors:    s.metrics.errors.Load(),
		TotalBytesIn:   s.metrics.bytesIn.Load(),
		TotalBytesOut:  s.metrics.bytesOut.Load(),
	}
	if s.cfg.Authorizer != nil {
		snapshot.AuthorizedKeys = len(s.cfg.Authorizer.Entries())
	}
	if s.store != nil {
		count, _ := s.store.CountReservations(ctx)
		snapshot.Reservations = count
	}
	return snapshot
}

type statusSnapshot struct {
	ActiveTunnels  int      `json:"active_tunnels"`
	Tunnels        []string `json:"tunnels"`
	AuthorizedKeys int      `json:"authorized_keys"`
	Reservations   int      `json:"reservations"`
	UptimeSeconds  uint64   `json:"uptime_seconds"`
	TotalRequests  uint64   `json:"total_requests"`
	TotalErrors    uint64   `json:"total_errors"`
	TotalBytesIn   uint64   `json:"total_bytes_in"`
	TotalBytesOut  uint64   `json:"total_bytes_out"`
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
}

func newMetrics() *metrics {
	return &metrics{}
}

func (m *metrics) Record(subdomain string, bytesIn, bytesOut int64, latency time.Duration, isError bool) {
	m.requests.Add(1)
	if isError {
		m.errors.Add(1)
	}
	m.bytesIn.Add(uint64(bytesIn))
	m.bytesOut.Add(uint64(bytesOut))
	m.latencySum.Add(uint64(latency.Milliseconds()))

	if val, ok := m.subdomains.Load(subdomain); ok {
		stats := val.(*subdomainStats)
		stats.requests.Add(1)
		if isError {
			stats.errors.Add(1)
		}
	} else {
		stats := &subdomainStats{}
		stats.requests.Add(1)
		if isError {
			stats.errors.Add(1)
		}
		m.subdomains.Store(subdomain, stats)
	}
}

func (m *metrics) snapshot() metricsSnapshot {
	return metricsSnapshot{
		TotalRequests: m.requests.Load(),
		TotalErrors:   m.errors.Load(),
		BytesIn:       m.bytesIn.Load(),
		BytesOut:      m.bytesOut.Load(),
	}
}

// cleanupLoop periodically removes stale tunnels
func (s *Server) cleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second) // Check every 30 seconds
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			removed := s.registry.cleanup()
			if removed > 0 {
				total, stale := s.registry.getStats()
				s.log.Info().
					Int("removed", removed).
					Int("total_active", total).
					Int("stale_remaining", stale).
					Msg("cleaned up stale tunnels")
			}
		}
	}
}
