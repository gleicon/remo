# Remo - Reverse Tunnel Project

## Summary

**remo** is a self-hosted reverse tunnel service that exposes local services through public subdomains (`*.yourdomain.tld`) using SSH reverse tunnels.

## Architecture

### How It Works

1. **Client connects to server via SSH** (port 22) using ed25519 key authentication
2. **Client opens reverse tunnel** - listens on a local port on the server
3. **Client registers** subdomain via HTTP through the SSH tunnel to `127.0.0.1:18080`
4. **Server proxies HTTP requests** to the tunnel port
5. **SSH tunnel forwards** to client's local upstream service

### Components

| Component | Path | Purpose |
|-----------|------|---------|
| Client | `internal/client/client.go` | SSH connection, reverse tunnel setup, registration |
| Server | `internal/server/server.go` | HTTP server, routing, registry |
| Registry | `internal/server/registry.go` | In-memory tunnel registry |
| Auth | `internal/auth/authorized.go` | Authorized keys management |
| Store | `internal/store/store.go` | SQLite persistence |
| Identity | `internal/identity/identity.go` | Ed25519 key management |

### Key Files

- `cmd/remo/root/root.go` - CLI entry point
- `cmd/remo/root/connect.go` - Client connection command
- `cmd/remo/root/server.go` - Server start command

## Current Status

- **Tests**: All passing (`go test ./...`)
- **Build**: Successful

### Known Issues (from prior session)

The context mentions prior debugging of an issue where `setupReverseTunnel` hangs after SSH connection succeeds. Debug statements were added to `client.go` to trace the issue:
- Line 149: "DEBUG: dialSSH called"
- Line 159: "DEBUG: Using public key: ..."
- Line 180: "DEBUG: About to dial SSH to ..."
- Line 187: "DEBUG: SSH dial succeeded"
- Line 193: "DEBUG: setupReverseTunnel called"

The SSH connection succeeds but `client.Listen("tcp", ...)` in `setupReverseTunnel` doesn't complete.

## Configuration

### Server Config (`/etc/remo/server.yaml`)

```yaml
listen: "127.0.0.1:18080"
domain: "yourdomain.tld"
mode: "behind-proxy"
authorized: /etc/remo/authorized.keys
state: /var/lib/remo/state.db
admin_secret: your-secret
```

### Client

```bash
remo connect --server yourserver.com --subdomain myapp --upstream http://127.0.0.1:3000
```

## Paths

| Path | Purpose |
|------|---------|
| `/etc/remo/server.yaml` | Server config |
| `/home/remo/.ssh/authorized_keys` | SSH authorized keys |
| `/var/lib/remo/state.db` | SQLite state |
| `~/.remo/identity.json` | Client identity |

## Commands

- `remo server` - Start the server
- `remo connect` - Connect to server with tunnel
- `remo auth` - Manage authorized keys
- `remo status` - Show server status
- `remo reservations` - List reservations

## Dependencies

- `golang.org/x/crypto/ssh` - SSH client
- `modernc.org/sqlite` - SQLite driver
- `github.com/rs/zerolog` - Logging
- `github.com/spf13/cobra` - CLI
- `github.com/charmbracelet/bubbletea` - TUI
