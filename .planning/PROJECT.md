# Remo — Self-Hosted Reverse Tunnel

## What This Is

**Remo** is a self-hosted reverse tunnel service that exposes local development services through public subdomains (`*.yourdomain.tld`). 

Unlike ngrok or similar services, remo:
- Uses **your own VPS** and domain
- Uses **system SSH** for tunneling (no custom protocols)
- Can run **behind nginx** (reuse existing VPS setups)
- Has a **TUI dashboard** showing real-time request logs (like `top`)

Inspired by: SirTunnel, tunnelto, pyjam.as/tunnel

## Core Value

Developers can expose local services through public subdomains using only system SSH, with a live dashboard showing traffic — no complex setup, no paid services.

## Architecture (MVP)

```
┌─────────────────┐         ┌──────────────────┐         ┌─────────────┐
│   Local Dev     │◄──SSH──►│   Your VPS       │◄──HTTP──►│   Public    │
│   Service       │  -R     │   (nginx+remo)   │         │   Internet  │
│   localhost:3000│         │   *.domain.com   │         │             │
└─────────────────┘         └──────────────────┘         └─────────────┘
                                    │
                                    ▼
                            ┌───────────────┐
                            │   TUI View    │
                            │   Request Log │
                            └───────────────┘
```

**Flow:**
1. User runs `remo connect --server vps.domain.com --subdomain myapp`
2. Client launches `ssh -R` to create reverse tunnel
3. Client registers subdomain with remo server via tunnel
4. Server adds route: `myapp.domain.com` → tunnel port
5. Nginx proxies `*.domain.com` → remo server
6. TUI shows real-time request log

## Validated (Existing Working Code)

- ✓ HTTP proxy routing with subdomain extraction — `internal/server/server.go`
- ✓ Registry with thread-safe operations — `internal/server/registry.go`
- ✓ Ed25519 identity management — `internal/identity/identity.go`
- ✓ SQLite persistence layer — `internal/store/store.go`
- ✓ Authorization with public keys — `internal/auth/authorized.go`
- ✓ Behind-proxy mode for nginx — `ModeProxy` in server config

## Active (To Fix/Build)

### Critical (Blocking Usage)
- [ ] Replace internal SSH dialer with external `ssh -R` command
- [ ] Fix TUI quit key ('q') and graceful shutdown
- [ ] Wire up request logs from proxy to TUI

### MVP Polish
- [ ] Nginx configuration examples
- [ ] Admin endpoint documentation
- [ ] Setup script improvements

## Out of Scope

| Feature | Reason |
|---------|--------|
| Built-in SSH server | Use system sshd — simpler, more secure |
| WireGuard support | SSH -R is sufficient for MVP |
| Web UI | TUI is enough for developers |
| OAuth/SSO | Ed25519 keys + authorized_keys is simpler |
| Auto HTTPS | Let nginx/traefik handle TLS |
| Multi-region | Single VPS deployment for MVP |

## Context

**Design Philosophy (from research):**
- SirTunnel: "Zero-configuration" — just SSH
- pyjam.as/tunnel: "Bring your own server"
- tunnelto.dev: "Expose local web servers"

**Current Stack:** Go 1.25, Cobra, Bubble Tea, SQLite

**Key Insight:** The internal SSH dialer (`golang.org/x/crypto/ssh`) is the source of hangs. System `ssh` command handles edge cases, reconnection, and host key management better.

## Constraints

- **SSH:** Must work with stock sshd + `authorized_keys`
- **Nginx:** Must support running behind existing nginx setups
- **Binary:** Single static binary (CGO disabled)
- **TUI:** Top-like request log, quit with 'q'

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| Use external `ssh -R` | Proven, handles reconnection, no hangs | — Pending |
| Nginx-friendly | Users have existing VPS setups | — Partial (mode exists) |
| Bubble Tea TUI | Go-native, handles terminal UI well | — Partial (needs wiring) |
| SQLite state | Simple, embedded, no external deps | ✓ Working |
| Ed25519 auth | Modern, fast, simple key format | ✓ Working |

## Research References

- [awesome-tunneling](https://github.com/anderspitman/awesome-tunneling) — Tunneling tools comparison
- [SirTunnel](https://github.com/anderspitman/SirTunnel) — Minimal SSH tunnel approach
- [tunnelto.dev](https://tunnelto.dev/) — Developer-focused tunneling
- [pyjam.as/tunnel](https://tunnel.pyjam.as/) — Bring-your-own-server tunnel
- [Self-hosted ngrok with Nginx](https://jerrington.me/posts/2019-01-29-self-hosted-ngrok.html) — Nginx + SSH setup
- [Go SSH reverse tunnel gist](https://gist.github.com/codref/473351a24a3ef90162cf10857fac0ff3) — Reference implementation

---
*Last updated: 2026-02-18 with full spec context*
