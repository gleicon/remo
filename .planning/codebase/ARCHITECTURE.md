# Architecture

**Analysis Date:** 2026-02-18

## Pattern Overview

**Overall:** Command-Line Interface (CLI) with client-server reverse tunnel architecture

**Key Characteristics:**
- Go-based CLI tool using Cobra framework for command structure
- Client connects via SSH to establish reverse tunnel
- Server handles HTTP proxy routing based on subdomains
- Authentication via Ed25519 public keys
- Optional SQLite persistence for state management

## Layers

**CLI Commands Layer:**
- Purpose: Entry point for user interactions
- Location: `cmd/remo/root/`
- Contains: Cobra commands (server, connect, auth, keys, reservations, status)
- Depends on: internal packages (client, server, store, auth, identity)
- Used by: End users via CLI

**Client Layer:**
- Purpose: Manages SSH tunnel connection and registration
- Location: `internal/client/client.go`
- Contains: Client struct, SSH dial, reverse tunnel setup, server registration
- Depends on: identity, tui, zerolog
- Used by: CLI connect command

**Server Layer:**
- Purpose: HTTP server that routes requests to registered tunnels
- Location: `internal/server/server.go`
- Contains: Server struct, HTTP handlers (register, proxy, health, status, metrics)
- Depends on: auth, store, zerolog
- Used by: CLI server command

**Authentication Layer:**
- Purpose: Manages authorized keys and access control
- Location: `internal/auth/authorized.go`
- Contains: AuthorizedKeys struct, key loading, Allow() authorization logic
- Used by: Server, CLI auth commands

**Identity Layer:**
- Purpose: Ed25519 key pair management for clients
- Location: `internal/identity/identity.go`
- Contains: Identity struct, Generate, Load, Save functions
- Used by: Client, CLI keys commands

**Storage Layer:**
- Purpose: SQLite persistence for reservations, authorized keys, audit logs
- Location: `internal/store/store.go`
- Contains: Store struct, database operations, migrations
- Used by: Server, CLI reservations/auth commands

**TUI Layer:**
- Purpose: Terminal UI for connection monitoring
- Location: `internal/tui/model.go`
- Contains: Bubble Tea model for rendering connection state and request logs
- Used by: Client (optional)

**Logging Layer:**
- Purpose: Centralized logging configuration
- Location: `internal/logging/logging.go`
- Contains: zerolog logger factory
- Used by: All layers

## Data Flow

**Client Connection Flow:**

1. User runs `remo connect --server host --subdomain myapp`
2. Client loads identity from `~/.remo/identity.json`
3. Client dials SSH server using Ed25519 key authentication
4. Client sets up reverse SSH tunnel on random port (8000-9000)
5. Client registers with server via HTTP through tunnel
6. Server adds subdomain to registry mapping to remote port
7. Incoming HTTP requests to `myapp.domain.com` are proxied to tunnel

**Server Request Flow:**

1. HTTP request arrives at server for `subdomain.domain.com`
2. Server extracts subdomain from Host header
3. Server looks up subdomain in registry
4. If found, server creates reverse proxy to tunnel port
5. Response flows back through proxy to client

## Key Abstractions

**Server Registry:**
- Purpose: In-memory mapping of subdomains to tunnel ports
- Examples: `internal/server/registry.go`
- Pattern: Thread-safe sync.Map with mutex protection

**Metrics:**
- Purpose: Atomic counters for requests, errors, bytes, latency
- Examples: `internal/server/server.go` (metrics struct)
- Pattern: sync/atomic for lock-free counters

**Config Struct:**
- Purpose: Dependency injection for all configurable options
- Examples: `server.Config`, `client.Config`
- Pattern: Functional options style with validation in New()

## Entry Points

**CLI Entry Point:**
- Location: `cmd/remo/main.go`
- Triggers: Running `remo` command
- Responsibilities: Signal handling, argument normalization, command execution

**Root Command:**
- Location: `cmd/remo/root/root.go`
- Triggers: Any `remo` subcommand
- Responsibilities: Logger initialization, version, persistent flags

**Server Command:**
- Location: `cmd/remo/root/server.go`
- Triggers: `remo server`
- Responsibilities: Config parsing, store opening, server instantiation, Run()

**Connect Command:**
- Location: `cmd/remo/root/connect.go`
- Triggers: `remo connect`
- Responsibilities: Identity loading, client instantiation, Run()

## Error Handling

**Strategy:** Functional error wrapping with context

**Patterns:**
- `fmt.Errorf("action: %w", err)` for error propagation
- Context-aware errors via `errors.Is()`, `errors.As()`
- Structured logging of errors with zerolog

## Cross-Cutting Concerns

**Logging:** zerolog with RFC3339Nano timestamps, configurable levels via `--log` flag or `REMO_LOG` env var

**Validation:** Input validation in command handlers and server registration (subdomain format, port ranges)

**Authentication:** Ed25519 public key authentication for SSH; optional authorized keys file or database-backed keys

---

*Architecture analysis: 2026-02-18*
