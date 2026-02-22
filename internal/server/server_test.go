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
	reg := newRegistry(5 * time.Minute)
	if !reg.register("foo", 8080, "testkey") {
		t.Fatal("register should succeed")
	}
	if reg.register("foo", 8081, "testkey2") {
		t.Fatal("duplicate register should fail")
	}
	if !reg.has("foo") {
		t.Fatal("should have foo")
	}
	if reg.has("bar") {
		t.Fatal("should not have bar")
	}
	port, pubKey, ok := reg.get("foo")
	if !ok || port != 8080 || pubKey != "testkey" {
		t.Fatalf("get returned wrong values: port=%d pubKey=%s", port, pubKey)
	}
	_, _, ok = reg.get("bar")
	if ok {
		t.Fatal("get should return false for unregistered")
	}
	list := reg.list()
	if len(list) != 1 || list[0] != "foo" {
		t.Fatalf("list mismatch: %v", list)
	}
	reg.unregister("foo")
	if reg.has("foo") {
		t.Fatal("should not have foo after unregister")
	}
}

func TestRegistryListSorted(t *testing.T) {
	reg := newRegistry(5 * time.Minute)
	reg.register("zebra", 8080, "key1")
	reg.register("alpha", 8081, "key2")
	reg.register("mid", 8082, "key3")
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
	snap := m.snapshot()
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
}

func TestStatusEndpointContent(t *testing.T) {
	srv := testServer()
	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	srv.handleStatus(rec, req)
	var status statusSnapshot
	if err := json.NewDecoder(rec.Body).Decode(&status); err != nil {
		t.Fatalf("decode: %v", err)
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
		{"/register", http.StatusMethodNotAllowed},
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
	srv.registry.register("foo", 8080, "testkey")
	if !srv.HasTunnel("foo") {
		t.Fatal("should have tunnel")
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

func TestAuthorizerIntegration(t *testing.T) {
	srv := testServer(func(c *Config) {
		c.Authorizer = auth.NewAuthorizedKeys(nil)
	})
	_ = srv
}

func TestExtractSubdomainWithPrefix(t *testing.T) {
	srv := testServer(func(c *Config) {
		c.SubdomainPrefix = "apps"
	})
	tests := []struct {
		host     string
		expected string
	}{
		{"foo.apps.rempapps.site", "foo"},
		{"foo.apps.rempapps.site:443", "foo"},
		{"bar.apps.rempapps.site", "bar"},
		{"apps.rempapps.site", ""},
		{"foo.rempapps.site", ""},
		{"rempapps.site", ""},
		{"other.example.com", ""},
		{"", ""},
	}
	for _, tt := range tests {
		got := srv.extractSubdomain(tt.host)
		if got != tt.expected {
			t.Errorf("extractSubdomain(%q) = %q, want %q", tt.host, got, tt.expected)
		}
	}
}

func TestRoutingDomain(t *testing.T) {
	srv := testServer()
	if got := srv.routingDomain(); got != "rempapps.site" {
		t.Fatalf("expected rempapps.site, got %s", got)
	}
	srv2 := testServer(func(c *Config) {
		c.SubdomainPrefix = "apps"
	})
	if got := srv2.routingDomain(); got != "apps.rempapps.site" {
		t.Fatalf("expected apps.rempapps.site, got %s", got)
	}
}

func TestGenerateRandomSubdomain(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		name, err := generateRandomSubdomain()
		if err != nil {
			t.Fatalf("generate: %v", err)
		}
		if len(name) != 8 {
			t.Fatalf("expected 8-char hex, got %q (len %d)", name, len(name))
		}
		if seen[name] {
			t.Fatalf("duplicate random subdomain: %s", name)
		}
		seen[name] = true
	}
}

func TestProxyWithSubdomainPrefix(t *testing.T) {
	srv := testServer(func(c *Config) {
		c.SubdomainPrefix = "apps"
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "foo.rempapps.site"
	srv.handleProxy(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for non-prefix host, got %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "foo.apps.rempapps.site"
	srv.handleProxy(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502 (no tunnel), got %d", rec.Code)
	}
}
