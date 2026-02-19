# Testing Patterns

**Analysis Date:** 2026-02-18

## Test Framework

**Runner:**
- `testing` package (Go standard library)
- Go 1.25.3

**Run Commands:**
```bash
go test ./...                    # Run all tests
go test -v -count=1 ./...       # Verbose, no cache
go test -coverprofile=coverage.out ./... && go tool cover -func=coverage.out  # With coverage
```

**Makefile Targets:**
```bash
make test    # Run all tests
make test-v  # Verbose output
make cover   # Coverage report
```

## Test File Organization

**Location:**
- Co-located with source files: `internal/server/server_test.go` alongside `internal/server/server.go`

**Naming:**
- Pattern: `*_test.go`
- Test package: same as source package or `_test` suffix for external tests

**Structure:**
- All test files in same directory as implementation
- Test helper files can use `_test` package suffix when needed (e.g., `store_test` package)

## Test Structure

**Helper Functions:**
```go
func testServer(opts ...func(*Config)) *Server {
    cfg := Config{Domain: "rempapps.site", Logger: zerolog.New(io.Discard), AdminSecret: "secret"}
    for _, opt := range opts {
        opt(&cfg)
    }
    return New(cfg)
}

func testIdentity(t *testing.T) *identity.Identity {
    t.Helper()
    id, err := identity.Generate()
    if err != nil {
        t.Fatalf("generate: %v", err)
    }
    return id
}
```

**Pattern:** Use `t.Helper()` for helper functions to skip in failure traces

**Test Naming:**
- `Test` prefix + descriptive name: `TestHealthHandler`, `TestExtractSubdomain`
- Use `t.Fatalf` for fatal errors, `t.Errorf` for assertions

## Test Table Patterns

**Table-Driven Tests:**
```go
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
    }
    for _, tt := range tests {
        got := srv.extractSubdomain(tt.host)
        if got != tt.expected {
            t.Errorf("extractSubdomain(%q) = %q, want %q", tt.host, got, tt.expected)
        }
    }
}
```

**Use Cases:**
- Multiple input/output combinations
- Edge cases
- Boundary conditions

## Assertion Patterns

**Standard Library:**
- `if condition { t.Fatal/Error }` pattern
- No assertion library (using plain Go)

**Common Patterns:**
```go
if rec.Code != http.StatusOK {
    t.Fatalf("expected 200, got %d", rec.Code)
}
if rec.Body.String() != "ok" {
    t.Fatalf("expected ok, got %s", rec.Body.String())
}
if !srv.HasTunnel("foo") {
    t.Fatal("should have tunnel")
}
```

## Mocking

**Approach:**
- No external mocking framework
- Manual mocks when needed
- Test interfaces with concrete implementations

**HTTP Handler Testing:**
- Use `net/http/httptest`:
```go
rec := httptest.NewRecorder()
req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
srv.handleHealth(rec, req)
if rec.Code != http.StatusOK {
    t.Fatalf("expected 200, got %d", rec.Code)
}
```

**Database/Store Testing:**
- Use `t.TempDir()` for temporary test databases:
```go
func openTestStore(t *testing.T) *store.Store {
    t.Helper()
    path := filepath.Join(t.TempDir(), "state.db")
    st, err := store.Open(path)
    if err != nil {
        t.Fatalf("open store: %v", err)
    }
    t.Cleanup(func() { st.Close() })
    return st
}
```

**Key Generation for Tests:**
```go
func genKey(t *testing.T) ed25519.PublicKey {
    t.Helper()
    pub, _, err := ed25519.GenerateKey(rand.Reader)
    if err != nil {
        t.Fatalf("generate key: %v", err)
    }
    return pub
}
```

## Fixtures and Test Data

**Location:**
- Generated inline within test functions
- Helper functions for common test data

**Pattern:**
```go
func generateKey(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
    t.Helper()
    pub, priv, err := ed25519.GenerateKey(rand.Reader)
    if err != nil {
        t.Fatalf("generate key: %v", err)
    }
    return pub, priv
}
```

**File Permissions:**
- Use `t.TempDir()` for temporary files
- Explicit permissions when needed: `os.WriteFile(path, data, 0o600)`

## Coverage

**View Coverage:**
```bash
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out
```

**Enforcement:** No coverage requirements enforced

**Typical Coverage:** Tests cover core logic, handlers, store operations

## Test Types

**Unit Tests:**
- Test individual functions and methods
- Mock external dependencies where needed
- Example: `TestExtractSubdomain`, `TestHealthHandler`

**Integration Tests:**
- Test HTTP handlers with httptest
- Test store operations with SQLite
- Example: `TestAuthorizerIntegration`

**Handler Tests:**
```go
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
```

**Nil Safety Tests:**
- Test nil receiver behavior explicitly:
```go
func TestAllowNilAuthorizer(t *testing.T) {
    var ak *AuthorizedKeys
    pub, _ := generateKey(t)
    if !ak.Allow(pub, "anything") {
        t.Fatal("nil authorizer should allow all")
    }
}
```

## Common Patterns

**Default Values Testing:**
```go
func TestNewServerDefaults(t *testing.T) {
    srv := New(Config{Domain: "test.site", Logger: zerolog.New(io.Discard)})
    if srv.cfg.ReadTimeout != 30*time.Second {
        t.Fatalf("expected 30s read timeout, got %v", srv.cfg.ReadTimeout)
    }
    if srv.cfg.Mode != ModeProxy {
        t.Fatalf("expected behind-proxy mode, got %s", srv.cfg.Mode)
    }
}
```

**Error Cases:**
```go
func TestNewClientRequiresIdentity(t *testing.T) {
    _, err := New(Config{
        Server:      "localhost",
        ServerPort:  22,
        Subdomain:   "foo",
        UpstreamURL: "http://localhost:3000",
        Logger:      zerolog.New(io.Discard),
    })
    if err == nil {
        t.Fatal("expected error without identity")
    }
}
```

**Teardown:**
- Use `t.Cleanup()` for resource cleanup:
```go
t.Cleanup(func() { st.Close() })
```

---

*Testing analysis: 2026-02-18*
