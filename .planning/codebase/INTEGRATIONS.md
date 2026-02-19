# External Integrations

**Analysis Date:** 2026-02-18

## Overview

Remo is a self-hosted reverse tunnel service (similar to ngrok). It creates SSH-based tunnels from client machines to a server, exposing local services via subdomains.

## Core Integration: SSH Tunneling

**Protocol:** SSH (Secure Shell)
- Implementation: golang.org/x/crypto/ssh
- Client connects to server via SSH
- Creates reverse tunnel on remote port (8000-9000 range)
- Server proxies HTTP traffic through tunnel

**Connection Details:**
- Server: configurable (default port 22)
- User: `remo`
- Authentication: SSH public key (Ed25519)
- Tunnel registration: HTTP POST to server's `/register` endpoint

## Data Storage

**Database:**
- SQLite (modernc.org/sqlite - pure Go, no C dependencies)
- Location: configurable path (default: `./remo.db`)
- Tables:
  - `authorized_keys` - Public keys permitted to connect
  - `reservations` - Reserved subdomains
  - `audit_log` - Connection events
  - `settings` - Key-value configuration

**Connection:**
- File-based SQLite at configured path
- No external database services required

## Authentication & Identity

**SSH Key Authentication:**
- Ed25519 key pairs (primary)
- Keys generated client-side via `remo keys` command
- Public key authorization via:
  - File-based authorized keys (`authorized_keys` file)
  - SQLite database (`authorized_keys` table)
- Authorization rules support wildcards (`*`) and prefix matching

**Admin Authorization:**
- Bearer token via `Authorization: Bearer <secret>` header
- Configured via `--admin-secret` flag on server

## Server Endpoints

**HTTP API (port 18080 default):**
- `POST /register` - Register tunnel (requires Ed25519 public key in header)
- `GET /healthz` - Health check
- `GET /status` - Server status (admin protected)
- `GET /metrics` - Prometheus metrics (admin protected)
- `GET /` - Proxy to tunneled service (catch-all)

**Headers:**
- `X-Remo-Publickey` - Client's public key (base64)
- `X-Remo-Subdomain` - Tunnel subdomain
- `X-Forwarded-For` - Original client IP (if behind proxy)

## Configuration Files

**YAML Configuration:**
- Parsed via gopkg.in/yaml.v3
- Server, client, and auth configuration
- No external config management

## Deployment

**Build/Release:**
- GoReleaser for cross-platform builds
- GitHub Releases (implied by GoReleaser)

**Hosting:**
- Self-hosted (no external service dependency)
- Can run behind reverse proxy (Nginx, Caddy, etc.)
- Supports TLS in standalone mode

## Monitoring

**Logging:**
- rs/zerolog structured logging
- Log levels: debug, info, warn, error
- Configurable via `--log` flag or `REMO_LOG` env var

**Metrics:**
- Prometheus-compatible metrics at `/metrics`
- Exported metrics:
  - `remo_active_tunnels` - Current tunnel count
  - `remo_authorized_keys` - Authorized key count
  - `remo_reservations` - Reserved subdomain count

## Environment Variables

**Supported:**
- `REMO_LOG` - Log level (debug, info, warn)

**No external API keys required:**
- Self-contained system
- No third-party service dependencies

---

*Integration audit: 2026-02-18*
