# Codebase Structure

**Analysis Date:** 2026-02-18

## Directory Layout

```
remo/
├── cmd/                    # CLI entry points
│   └── remo/
│       ├── main.go         # Binary entry point
│       └── root/           # Cobra commands
├── internal/               # Private packages
│   ├── auth/               # Authorization
│   ├── client/             # Tunnel client
│   ├── identity/           # Key management
│   ├── integration/        # Integration tests
│   ├── logging/            # Logger setup
│   ├── server/             # HTTP server
│   ├── store/              # SQLite persistence
│   ├── tlsmanager/         # TLS utilities
│   └── tui/                # Terminal UI
├── scripts/                # Build/utility scripts
├── build/                  # Build outputs
├── .planning/              # Planning documents
├── go.mod                 # Go module definition
├── go.sum                 # Go dependencies
├── Makefile               # Build targets
└── .goreleaser.yaml       # Release configuration
```

## Directory Purposes

**cmd/remo/main.go:**
- Purpose: Binary entry point
- Contains: Signal handling, argument normalization
- Key file: `cmd/remo/main.go`

**cmd/remo/root/:**
- Purpose: All CLI commands implemented as Cobra commands
- Contains: Root, server, connect, auth, keys, reservations, status commands
- Key files: `root.go`, `server.go`, `connect.go`, `auth.go`, `keys.go`, `reservations.go`, `status.go`, `config.go`

**internal/server/:**
- Purpose: Core reverse tunnel server implementation
- Contains: Server struct, HTTP handlers, registry, metrics
- Key files: `server.go`, `server_test.go`, `registry.go`

**internal/client/:**
- Purpose: Client that establishes SSH tunnel and registers with server
- Contains: Client struct, SSH dial, reverse tunnel, registration
- Key files: `client.go`, `client_test.go`

**internal/auth/:**
- Purpose: Authorization via authorized keys
- Contains: AuthorizedKeys loading and Allow() logic
- Key files: `authorized.go`, `authorized_test.go`

**internal/store/:**
- Purpose: SQLite persistence layer
- Contains: Store operations, migrations, CRUD for reservations/keys
- Key files: `store.go`, `store_test.go`

**internal/identity/:**
- Purpose: Ed25519 key pair management
- Contains: Identity generation, loading, saving
- Key files: `identity.go`, `identity_test.go`

**internal/tui/:**
- Purpose: Bubble Tea terminal UI
- Contains: Model, view, state messages
- Key files: `model.go`, `model_test.go`

**internal/logging/:**
- Purpose: Logger factory
- Contains: zerolog configuration
- Key files: `logging.go`

## Key File Locations

**Entry Points:**
- `cmd/remo/main.go`: Binary entry, signal handling
- `cmd/remo/root/root.go`: Root command, logger init

**Configuration:**
- `cmd/remo/root/config.go`: YAML config parsing (`serverFileConfig`)
- `cmd/remo/root/server.go`: Server command flags and config assembly

**Core Logic:**
- `internal/server/server.go`: HTTP server with register/proxy handlers
- `internal/client/client.go`: SSH tunnel client
- `internal/auth/authorized.go`: Key authorization

**State:**
- `internal/store/store.go`: SQLite database operations

## Naming Conventions

**Files:**
- lowercase with underscores: `server.go`, `authorized.go`, `client_test.go`
- Command files: `root.go`, `connect.go`, `server.go` (verb-based for commands)

**Directories:**
- lowercase, single word or underscore: `auth/`, `client/`, `tui/`

**Go Types:**
- PascalCase: `Server`, `Client`, `Config`, `AuthorizedKeys`
- Interfaces often end with -er: `tea.Model`

**Functions:**
- PascalCase exported: `New()`, `Run()`, `Load()`, `Generate()`
- camelCase unexported: `handleRegister()`, `dialSSH()`

**Variables:**
- camelCase: `cfg`, `logger`, `ctx`
- Acronyms stay uppercase: `URL`, `HTTP`, `SSH` (except first letter in struct fields)

## Where to Add New Code

**New CLI Command:**
- Primary code: `cmd/remo/root/<command>.go`
- Follow pattern: create `newXxxCommand()` returning `*cobra.Command`, add to root in `root.go`

**New Server Handler:**
- Implementation: Add to `internal/server/server.go` or create new file in `internal/server/`
- Handler registered in `Handler()` method via `mux.HandleFunc()`

**New Client Feature:**
- Implementation: `internal/client/client.go`
- Add to `Client` struct and `New()` / `Run()` methods

**New Storage Entity:**
- Implementation: `internal/store/store.go`
- Add migration in `migrate()`, add CRUD methods
- Schema: SQLite with tables for authorized_keys, reservations, audit_log, settings

**New Authentication Method:**
- Implementation: `internal/auth/`
- Follow `AuthorizedKeys` pattern

**Utilities:**
- Shared helpers: Appropriate package in `internal/`
- Cross-cutting: `internal/logging/` for logging, create new package if needed

## Special Directories

**internal/integration/:**
- Purpose: Integration tests requiring full system
- Generated: No
- Committed: Yes

**build/:**
- Purpose: Build artifacts (gitignored)
- Generated: Yes
- Committed: No

**scripts/:**
- Purpose: Build and utility scripts
- Generated: No
- Committed: Yes

---

*Structure analysis: 2026-02-18*
