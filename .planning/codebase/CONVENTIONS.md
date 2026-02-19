# Coding Conventions

**Analysis Date:** 2026-02-18

## Language

**Go:** 1.25.3 (per `go.mod`)

## Naming Patterns

**Files:**
- Go source: lowercase with underscores for multiple words (`store_test.go`, `authorized_test.go`)
- Test files: `*_test.go` suffix, co-located with source

**Packages:**
- Lowercase, no underscores: `client`, `server`, `store`, `auth`, `identity`, `logging`, `tui`
- Test packages: same as source or `_test` suffix for external tests

**Functions:**
- Exported: PascalCase (`New`, `Run`, `HandleRegister`)
- Unexported: camelCase (`routingDomain`, `extractSubdomain`)
- Constructor: `New` prefix for factory functions (`NewServer`, `NewClient`)

**Variables:**
- camelCase: `cfg`, `log`, `srv`, `id`
- Unused variables: `_ = value` or explicitly assigned to avoid errors

**Types:**
- PascalCase: `Server`, `Client`, `Config`, `Identity`, `Entry`
- Struct tags: JSON tags for serialization

**Constants:**
- PascalCase for exported: `ModeStandalone`, `ModeProxy`
- camelCase for unexported: `handshakeSkew`

## Code Style

**Formatting:**
- Tool: `gofmt` (standard Go formatter)
- Command: `make fmt` runs `gofmt -s -w .`

**Linting:**
- Tool: `go vet` (built-in)
- Optional: `staticcheck` (requires separate installation)
- Command: `make lint` runs both

**Indentation:**
- Standard Go tab-based indentation
- No custom indentation rules

**Line Length:**
- No enforced limit; Go typically uses 80-120 characters

## Import Organization

**Order (as seen in `internal/server/server.go`):**
1. Standard library (grouped by category):
   - `context`, `crypto/*`, `encoding/*`, `errors`, `fmt`
   - `net`, `net/http`, `net/http/httputil`
   - `strings`, `sync`, `sync/atomic`, `time`
2. External packages:
   - `github.com/rs/zerolog`
   - `github.com/charmbracelet/*`
   - `golang.org/x/crypto`
3. Internal packages:
   - `github.com/gleicon/remo/internal/*`

**Blank imports:** Grouped at end (e.g., `_ "modernc.org/sqlite"`)

## Error Handling

**Patterns:**
- Early returns on error:
```go
if err != nil {
    return nil, err
}
```
- Wrapped errors with context:
```go
return nil, fmt.Errorf("dial ssh: %w", err)
```
- Sentinel errors: `errors.New()` for known error types
- Nil checks: Explicit nil checks before method calls on potentially nil receivers

**HTTP Handlers:**
- Use `http.Error()` for error responses with appropriate status codes
- Log errors before returning: `s.log.Error().Err(err).Msg("...")`

**Defensive Coding:**
- Nil-safe methods (e.g., `func (a *AuthorizedKeys) Entries() []Entry` checks for nil receiver)
- Nil store handling in `internal/store/store.go` methods

## Logging

**Framework:** `zerolog` (github.com/rs/zerolog)

**Pattern:**
```go
logger := zerolog.New(os.Stdout).Level(logLevel).With().Timestamp().Logger()
```

**Usage:**
- Structured logging with level methods: `.Info()`, `.Warn()`, `.Error()`, `.Debug()`
- Context fields: `.Str("key", value).Msg("message")`
- Error logging: `.Err(err).Msg("...")`

**Configuration:**
- Via `internal/logging/logging.go` package
- Level string parsed from CLI: "debug", "warn", "error", "info"

## Comments

**Convention:**
- Go doc comments for exported functions/types (start with name)
- No required comment style enforcement
- Inline comments for non-obvious logic

**Example (from `internal/logging/logging.go`):**
```go
// New returns a zerolog.Logger configured for CLI tools.
func New(level string) zerolog.Logger {
```

## Function Design

**Constructor Pattern:**
- Functional options for Config structs:
```go
func testServer(opts ...func(*Config)) *Server {
    cfg := Config{...}
    for _, opt := range opts {
        opt(&cfg)
    }
    return New(cfg)
}
```

**Return Values:**
- Multiple returns for error handling: `func() (T, error)`
- Named returns for clarity when helpful

**Parameters:**
- Context as first parameter for operations that may be canceled: `func(ctx context.Context, ...)`

## Configuration

**Pattern:** Config structs with zero-value sensible defaults
- Example: `ReadTimeout` defaults to 30 seconds in `New()` function
- Mode defaults to `ModeProxy`

## HTTP Server Patterns

**Handler Registration:**
```go
func (s *Server) Handler() http.Handler {
    mux := http.NewServeMux()
    mux.HandleFunc("/register", s.handleRegister)
    mux.HandleFunc("/healthz", s.handleHealth)
    // ...
    return mux
}
```

**Graceful Shutdown:**
- Context-based cancellation
- Timeout on shutdown (5 seconds in example)

## Module Design

**Exports:**
- Only exported what needs to be used externally
- Internal packages not exposed

**No Barrel Files:**
- Direct imports: `github.com/gleicon/remo/internal/server`

---

*Convention analysis: 2026-02-18*
