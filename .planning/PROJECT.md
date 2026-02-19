# Remo — Code Review & Refactoring Project

## What This Is

**Remo** is a reverse tunnel service that exposes local services through public subdomains. Currently it's overly complex — it implements its own SSH client and tries to manage SSH connections directly, which causes the hanging issues you're experiencing.

The goal is to simplify: use the system's `ssh` command for tunneling, and focus remo on being a lightweight registration client and HTTP proxy server.

## Core Value

Users can expose local services through public subdomains using only system SSH, with minimal client complexity and reliable server-side HTTP proxying.

## Current Problems

### 1. SSH Connection Hangs (Critical)
**Location:** `internal/client/client.go:203`

The client calls `client.Listen("tcp", ...)` which creates a reverse SSH tunnel. This requires:
- sshd configured with `GatewayPorts yes` (rarely enabled by default)
- Or `AllowTcpForwarding remote` 
- The SSH user must be able to bind to the requested port

**Result:** Connection succeeds but `setupReverseTunnel` hangs indefinitely.

### 2. Wrong Architecture Approach
The client implements its own SSH dialer (`ssh.Dial`). This is:
- Complex (handling auth, host keys, reconnection)
- Fragile (depends on sshd config)
- Hard to debug

**Should be:** Use the system's `ssh` command with `-R` flag for reverse tunnels.

### 3. TUI Issues
**Location:** `internal/tui/model.go`

- No quit key ('q' missing from key handlers)
- Request logs are received but **never sent** — the proxy code doesn't emit `RequestLogMsg`
- No graceful shutdown coordination between TUI and client

### 4. Admin Endpoints (Actually Complete)
**Location:** `internal/server/server.go`

The admin endpoints (`/status`, `/metrics`, `/healthz`) are implemented. They require `AdminSecret` config which may not be obvious.

## Validated (Existing Working Code)

- ✓ HTTP proxy routing with subdomain extraction — `internal/server/server.go`
- ✓ Registry with thread-safe operations — `internal/server/registry.go`
- ✓ Ed25519 identity management — `internal/identity/identity.go`
- ✓ SQLite persistence layer — `internal/store/store.go`
- ✓ Authorization with public keys — `internal/auth/authorized.go`
- ✓ Configuration loading — `cmd/remo/root/server.go`

## Active (To Fix/Build)

- [ ] Simplify client to use external `ssh` command
- [ ] Fix TUI quit key and request logging
- [ ] Add request logging to proxy handler
- [ ] Improve documentation for admin endpoints
- [ ] Clean up dead code (internal SSH dialer)

## Out of Scope

- Built-in SSH server — Use system sshd
- Complex TUI features — Keep it simple (logs + status)
- Authentication beyond Ed25519 keys — Not needed for v1

## Context

**Current stack:** Go, Cobra, Bubble Tea, SQLite, golang.org/x/crypto/ssh

**Architecture:** Client-server with client managing its own SSH connections

**Problem:** The SSH connection management is the source of hangs and complexity

## Constraints

- **Tech:** Go 1.25, keep using Cobra for CLI, Bubble Tea for TUI
- **SSH:** Must work with stock sshd (no special config required beyond normal key auth)
- **Deployment:** Single binary, minimal dependencies
- **Simplicity:** Less code is better — remove the SSH dialer

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| Use external ssh command | System SSH is reliable, well-configured, handles edge cases | — Pending |
| Remove internal SSH client | Reduces complexity, eliminates hang issues | — Pending |
| Keep TUI minimal | Logs + status only, no complex features | — Pending |

---
*Last updated: 2026-02-18 after code review*
