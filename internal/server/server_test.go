package server

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/gleicon/remo/internal/auth"
)

func testServer(opts ...func(*Config)) *Server {
	cfg := Config{Domain: "rempapps.site", Logger: zerolog.New(io.Discard), AdminSecret: "secret"}
	for _, opt := range opts {
		opt(&cfg)
	}
	return New(cfg)
}

func TestHealthHandler(t *testing.T) {
	srv := testServer()
	rec := httptest.NewRecorder()
	srv.handleHealth(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if rec.Body.String() != "ok" {
		t.Fatalf("expected ok, got %s", rec.Body.String())
	}
}

func TestExtractSubdomain(t *testing.T) {
	srv := testServer()
	tests := []struct {
		host     string
		expected string
	}{
		{"foo.rempapps.site", "foo"},
		{"foo.rempapps.site:443", "foo"},
		{"bar.rempapps.site", "bar"},
		{"rempapps.site", ""},
		{"other.example.com", ""},
		{"", ""},
		{"deep.sub.rempapps.site", "sub"},
	}
	for _, tt := range tests {
		got := srv.extractSubdomain(tt.host)
		if got != tt.expected {
			t.Errorf("extractSubdomain(%q) = %q, want %q", tt.host, got, tt.expected)
		}
	}
}

func TestForwardHeadersStandalone(t *testing.T) {
	srv := testServer(func(c *Config) {
		c.Mode = ModeStandalone
	})
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "1.2.3.4:5678"
	headers := srv.forwardHeaders(req, "foo")
	if headers["X-Forwarded-Proto"][0] != "https" {
		t.Fatalf("expected https, got %s", headers["X-Forwarded-Proto"][0])
	}
	if headers["X-Forwarded-For"][0] != "1.2.3.4" {
		t.Fatalf("expected 1.2.3.4, got %s", headers["X-Forwarded-For"][0])
	}
	if headers["X-Remo-Subdomain"][0] != "foo" {
		t.Fatalf("expected foo, got %s", headers["X-Remo-Subdomain"][0])
	}
}

func TestForwardHeadersBehindProxy(t *testing.T) {
	_, cidr, _ := net.ParseCIDR("127.0.0.0/8")
	srv := testServer(func(c *Config) {
		c.Mode = ModeProxy
		c.TrustedProxies = []*net.IPNet{cidr}
	})
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-For", "10.0.0.1")
	headers := srv.forwardHeaders(req, "bar")
	if headers["X-Forwarded-Proto"][0] != "https" {
		t.Fatalf("expected https from proxy, got %s", headers["X-Forwarded-Proto"][0])
	}
	if headers["X-Forwarded-For"][0] != "10.0.0.1, 127.0.0.1" {
		t.Fatalf("expected appended XFF, got %s", headers["X-Forwarded-For"][0])
	}
}

func TestForwardHeadersUntrustedProxy(t *testing.T) {
	srv := testServer(func(c *Config) {
		c.Mode = ModeProxy
	})
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	req.Header.Set("X-Forwarded-For", "spoofed")
	headers := srv.forwardHeaders(req, "foo")
	if headers["X-Forwarded-For"][0] != "10.0.0.1" {
		t.Fatalf("expected peer IP only, got %s", headers["X-Forwarded-For"][0])
	}
	if headers["X-Forwarded-Proto"][0] != "http" {
		t.Fatalf("expected http for untrusted proxy, got %s", headers["X-Forwarded-Proto"][0])
	}
}

func TestProxyNoSubdomain(t *testing.T) {
	srv := testServer()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "rempapps.site"
	srv.handleProxy(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestProxyNoTunnel(t *testing.T) {
	srv := testServer()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "foo.rempapps.site"
	srv.handleProxy(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d", rec.Code)
	}
}

func TestMetricsHandlerAuth(t *testing.T) {
	srv := testServer()
	rec := httptest.NewRecorder()
	srv.handleMetrics(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rec = httptest.NewRecorder()
	srv.handleMetrics(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if body == "" {
		t.Fatal("expected metrics output")
	}
}

func TestAuthorizeAdminEmptySecret(t *testing.T) {
	srv := testServer(func(c *Config) { c.AdminSecret = "" })
	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	req.Header.Set("Authorization", "Bearer anything")
	if srv.authorizeAdmin(req) {
		t.Fatal("should not authorize with empty admin secret")
	}
}

func TestAuthorizeAdminWrongToken(t *testing.T) {
	srv := testServer()
	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	req.Header.Set("Authorization", "Bearer wrong")
	if srv.authorizeAdmin(req) {
		t.Fatal("should not authorize with wrong token")
	}
}

func TestAuthorizeAdminNoBearer(t *testing.T) {
	srv := testServer()
	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	req.Header.Set("Authorization", "Basic secret")
	if srv.authorizeAdmin(req) {
		t.Fatal("should not authorize without Bearer prefix")
	}
}

func TestRegistryOperations(t *testing.T) {
	reg := newRegistry()
	tunnel := &Tunnel{subdomain: "foo"}
	if !reg.register("foo", tunnel) {
		t.Fatal("register should succeed")
	}
	if reg.register("foo", &Tunnel{subdomain: "foo"}) {
		t.Fatal("duplicate register should fail")
	}
	if !reg.has("foo") {
		t.Fatal("should have foo")
	}
	if reg.has("bar") {
		t.Fatal("should not have bar")
	}
	got, ok := reg.get("foo")
	if !ok || got != tunnel {
		t.Fatal("get should return the registered tunnel")
	}
	_, ok = reg.get("bar")
	if ok {
		t.Fatal("get should return false for unregistered")
	}
	list := reg.list()
	if len(list) != 1 || list[0] != "foo" {
		t.Fatalf("list mismatch: %v", list)
	}
	reg.unregister("foo", tunnel)
	if reg.has("foo") {
		t.Fatal("should not have foo after unregister")
	}
}

func TestRegistryUnregisterWrongTunnel(t *testing.T) {
	reg := newRegistry()
	t1 := &Tunnel{subdomain: "foo"}
	t2 := &Tunnel{subdomain: "foo"}
	reg.register("foo", t1)
	reg.unregister("foo", t2)
	if !reg.has("foo") {
		t.Fatal("should still have foo because different tunnel pointer")
	}
}

func TestRegistryListSorted(t *testing.T) {
	reg := newRegistry()
	reg.register("zebra", &Tunnel{subdomain: "zebra"})
	reg.register("alpha", &Tunnel{subdomain: "alpha"})
	reg.register("mid", &Tunnel{subdomain: "mid"})
	list := reg.list()
	if len(list) != 3 {
		t.Fatalf("expected 3, got %d", len(list))
	}
	if list[0] != "alpha" || list[1] != "mid" || list[2] != "zebra" {
		t.Fatalf("expected sorted, got %v", list)
	}
}

func TestMetricsRecord(t *testing.T) {
	m := newMetrics()
	m.Record("foo", 100, 200, 10*time.Millisecond, false)
	m.Record("foo", 50, 100, 20*time.Millisecond, true)
	snap := m.Snapshot()
	if snap.TotalRequests != 2 {
		t.Fatalf("expected 2 requests, got %d", snap.TotalRequests)
	}
	if snap.TotalErrors != 1 {
		t.Fatalf("expected 1 error, got %d", snap.TotalErrors)
	}
	if snap.BytesIn != 150 {
		t.Fatalf("expected 150 bytes in, got %d", snap.BytesIn)
	}
	if snap.BytesOut != 300 {
		t.Fatalf("expected 300 bytes out, got %d", snap.BytesOut)
	}
	if snap.AvgLatencyMs <= 0 {
		t.Fatal("expected positive avg latency")
	}
}

func TestMetricsSnapshotEmpty(t *testing.T) {
	m := newMetrics()
	snap := m.Snapshot()
	if snap.TotalRequests != 0 || snap.AvgLatencyMs != 0 {
		t.Fatal("empty metrics should be zero")
	}
}

func TestStatusEndpointContent(t *testing.T) {
	srv := testServer()
	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	srv.handleStatus(rec, req)
	var status statusResponse
	if err := json.NewDecoder(rec.Body).Decode(&status); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if status.Domain != "rempapps.site" {
		t.Fatalf("domain: %s", status.Domain)
	}
	if status.Mode != ModeProxy {
		t.Fatalf("mode: %s", status.Mode)
	}
	if status.UptimeSeconds < 0 {
		t.Fatal("negative uptime")
	}
}

func TestValidateHops(t *testing.T) {
	_, cidr, _ := net.ParseCIDR("127.0.0.0/8")
	srv := testServer(func(c *Config) {
		c.TrustedProxies = []*net.IPNet{cidr}
		c.TrustedHops = 2
	})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	req.Header.Set("X-Forwarded-For", "10.0.0.1, 10.0.0.2")
	if !srv.validateHops(req) {
		t.Fatal("2 hops should be valid with TrustedHops=2")
	}
	req.Header.Set("X-Forwarded-For", "10.0.0.1, 10.0.0.2, 10.0.0.3")
	if srv.validateHops(req) {
		t.Fatal("3 hops should be invalid with TrustedHops=2")
	}
}

func TestTrustedProxyNonLoopback(t *testing.T) {
	_, cidr, _ := net.ParseCIDR("10.0.0.0/8")
	srv := testServer(func(c *Config) {
		c.TrustedProxies = []*net.IPNet{cidr}
	})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.168.1.1:1234"
	if srv.trustedProxy(req) {
		t.Fatal("192.168.1.1 should not be trusted for 10.0.0.0/8")
	}
	req.RemoteAddr = "10.0.0.5:1234"
	if !srv.trustedProxy(req) {
		t.Fatal("10.0.0.5 should be trusted for 10.0.0.0/8")
	}
}

func TestHandlerRoutes(t *testing.T) {
	srv := testServer()
	handler := srv.Handler()
	tests := []struct {
		path   string
		expect int
	}{
		{"/healthz", http.StatusOK},
		{"/status", http.StatusUnauthorized},
		{"/metrics", http.StatusUnauthorized},
	}
	for _, tt := range tests {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, tt.path, nil)
		handler.ServeHTTP(rec, req)
		if rec.Code != tt.expect {
			t.Errorf("%s: expected %d, got %d", tt.path, tt.expect, rec.Code)
		}
	}
}

func TestNewServerDefaults(t *testing.T) {
	srv := New(Config{Domain: "test.site", Logger: zerolog.New(io.Discard)})
	if srv.cfg.ReadTimeout != 30*time.Second {
		t.Fatalf("expected 30s read timeout, got %v", srv.cfg.ReadTimeout)
	}
	if srv.cfg.Mode != ModeProxy {
		t.Fatalf("expected behind-proxy mode, got %s", srv.cfg.Mode)
	}
	if srv.cfg.TrustedHops != 1 {
		t.Fatalf("expected 1 trusted hop, got %d", srv.cfg.TrustedHops)
	}
}

func TestHasTunnel(t *testing.T) {
	srv := testServer()
	if srv.HasTunnel("foo") {
		t.Fatal("should not have tunnel")
	}
	srv.registry.register("foo", &Tunnel{subdomain: "foo"})
	if !srv.HasTunnel("foo") {
		t.Fatal("should have tunnel")
	}
}

func TestCloneHeader(t *testing.T) {
	original := http.Header{"Accept": {"text/html", "application/json"}, "Host": {"example.com"}}
	cloned := cloneHeader(original)
	cloned["Accept"][0] = "modified"
	if original["Accept"][0] != "text/html" {
		t.Fatal("clone modified original")
	}
}

func TestPeerAddress(t *testing.T) {
	srv := testServer()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "1.2.3.4:5678"
	if got := srv.peerAddress(req); got != "1.2.3.4" {
		t.Fatalf("expected 1.2.3.4, got %s", got)
	}
	req.RemoteAddr = "1.2.3.4"
	if got := srv.peerAddress(req); got != "1.2.3.4" {
		t.Fatalf("expected 1.2.3.4 without port, got %s", got)
	}
}

func TestMax64(t *testing.T) {
	if max64(5, 0) != 5 {
		t.Fatal("5 should be > 0")
	}
	if max64(-1, 0) != 0 {
		t.Fatal("-1 should clamp to 0")
	}
	if max64(0, 0) != 0 {
		t.Fatal("0 should return 0")
	}
}

func TestAuthorizerIntegration(t *testing.T) {
	srv := testServer(func(c *Config) {
		c.Authorizer = auth.NewAuthorizedKeys(nil)
	})
	_ = srv
}
