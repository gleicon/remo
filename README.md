# Remo

[![Go Version](https://img.shields.io/badge/go-1.21+-00ADD8.svg)](https://golang.org/)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

**Self-hosted reverse tunnel with SSH and TUI dashboard**

Expose local services through public `*.yourdomain.tld` subdomains using standard SSH. No custom protocols, no complex configuration—just a single binary.

---

## What is Remo?

Remo is a lightweight reverse tunnel solution that lets you expose local development or production services to the internet through clean, memorable subdomains. It uses standard SSH reverse tunnels under the hood, giving you battle-tested security without the complexity of custom protocols.

### Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              INTERNET                                       │
│                                                                             │
│   User Request: https://myapp.yourdomain.tld/api/users                      │
│           │                                                                 │
│           ▼                                                                 │
│   ┌───────────────┐                                                         │
│   │   DNS         │                                                         │
│   │   (A record:  │                                                         │
│   │   *.domain →  │                                                         │
│   │   server IP)  │                                                         │
│   └───────┬───────┘                                                         │
│           │                                                                 │
│           ▼                                                                 │
│   ┌───────────────┐     ┌───────────────┐     ┌─────────────────────────┐   │
│   │   Nginx       │────▶│   Remo        │────▶│   SSH Tunnel            │   │
│   │   (SSL/       │     │   Server      │     │   (Port Forward)        │   │
│   │   Reverse     │     │   (Port 18080)│     │                         │   │
│   │   Proxy)      │     │               │     │   ┌─────────────────┐   │   │
│   │               │     │   Routes by   │     │   │   Local Service │   │   │
│   │   Port 443    │     │   subdomain   │     │   │   (your app)    │   │   │
│   └───────────────┘     └───────────────┘     │   │   Port 3000     │   │   │
│                                               │   └─────────────────┘   │   │
│                                               │          ▲              │   │
│                                               │          │              │   │
│                                               │   ┌──────┴──────┐       │   │
│                                               │   │  Remo       │       │   │
│                                               │   │  Client     │       │   │
│                                               │   │  (laptop)   │       │   │
│                                               │   └─────────────┘       │   │
│                                               └─────────────────────────┘   │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘

Data Flow:
  HTTP Request → Nginx (SSL) → Remo Server → SSH Tunnel → Your Local Service
  Response     ← Nginx ← Remo Server ← SSH Tunnel ← Your Local Service
```

### Key Features

- **Wildcard Subdomain Routing** — Automatically route `*.yourdomain.tld` to different local services
- **SSH-Based Security** — Uses standard SSH key authentication (Ed25519/RSA), no custom protocols
- **Real-Time TUI Dashboard** — Watch requests flow through your tunnels with live filtering and statistics
- **Single Binary** — One executable, no dependencies, minimal resource usage
- **Production Ready** — Nginx SSL termination, systemd service, Prometheus metrics

---

## Quick Start

### Prerequisites

- **Go 1.21+** (for building from source) or use pre-built binaries
- **SSH client** (OpenSSH) on your local machine
- **Domain name** with DNS access to configure wildcard records
- **Server** with public IP (VPS, cloud instance, etc.)

### Step 1: Install the Client

```bash
# Using the install script
curl -sL https://raw.githubusercontent.com/gleicon/remo/main/scripts/remo-setup.sh | bash

# Or download latest release manually
# See: https://github.com/gleicon/remo/releases
```

This installs the `remo` binary and generates your SSH identity at `~/.remo/identity.json`.

**Show your public key** (to share with server admin):
```bash
cat ~/.remo/identity.json | jq -r '.public_key'
```

### Step 2: Set Up the Server

**Option A: Full Setup (standalone with Let's Encrypt)**

```bash
sudo curl -sL https://raw.githubusercontent.com/gleicon/remo/main/scripts/remo-setup.sh | bash -s -- server \
  --domain yourdomain.tld
```

**Option B: Behind Nginx (recommended for production)**

```bash
sudo curl -sL https://raw.githubusercontent.com/gleicon/remo/main/scripts/remo-setup.sh | bash -s -- server \
  --domain yourdomain.tld \
  --behind-proxy \
  --skip-certs
```

For detailed nginx setup with SSL, see [docs/nginx.md](docs/nginx.md).

### Step 3: Connect and Expose a Service

```bash
# Expose your local service on a public subdomain
remo connect \
  --server yourserver.com \
  --subdomain myapp \
  --upstream http://127.0.0.1:3000 \
  --tui
```

Your service is now available at: **`https://myapp.yourdomain.tld`**

**Expected output:**
```
✓ Connected to yourserver.com:22
✓ Registered subdomain: myapp.yourdomain.tld
✓ Tunnel active: myapp.yourdomain.tld → http://127.0.0.1:3000
✓ TUI dashboard started

Press 'q' to quit, 'h' for help
```

---

## How It Works

1. **SSH Connection** — Client connects to server via SSH (port 22) using your SSH key
2. **Reverse Tunnel** — Client opens a reverse tunnel, listening on a local port on the server
3. **Subdomain Registration** — Client registers a subdomain via HTTP through the SSH tunnel
4. **Request Routing** — Server proxies incoming HTTP requests to the appropriate tunnel port
5. **Local Forwarding** — SSH tunnel forwards requests to your local upstream service

### TUI Dashboard Features

The TUI provides real-time visibility into your tunnels:

- **Live Request Log** — See every HTTP request as it happens
- **Session Statistics** — Track requests, errors, bytes transferred, and latency
- **Filtering** — Filter by text or show only error responses
- **JSON Export** — Export request logs when quitting for later analysis

---

## Production Setup with Nginx

For production deployments, we recommend using nginx as a reverse proxy with SSL termination:

**Why nginx?**
- SSL/TLS termination with Let's Encrypt
- WebSocket support for real-time features
- Better performance and connection handling
- Standard security headers and best practices

**Quick certbot example:**
```bash
# Obtain wildcard certificate
sudo certbot certonly --manual --preferred-challenges dns \
  -d "*.yourdomain.tld" -d "yourdomain.tld"

# Auto-renewal test
sudo certbot renew --dry-run
```

**Documentation:**
- [docs/nginx.md](docs/nginx.md) — Complete nginx and Let's Encrypt setup guide
- [docs/nginx-example.conf](docs/nginx-example.conf) — Production-ready nginx configuration

---

## SSH Key Authentication

Remo uses SSH public key authentication for secure, passwordless connections.

### Quick Setup

**Generate Ed25519 key pair (client):**
```bash
ssh-keygen -t ed25519 -C "remo-$(whoami)@$(hostname)" -f ~/.ssh/remo_ed25519
```

**Add to authorized keys (server):**
```bash
# As remo user on server
echo "ssh-ed25519 AAAAC3NzaC1... your-comment *" >> ~/.ssh/authorized_keys
chmod 600 ~/.ssh/authorized_keys
```

**Subdomain rules:**
- `*` — Allow any subdomain
- `dev-*` — Allow subdomains starting with "dev-"
- `staging` — Allow only exact subdomain "staging"

**Full guide:** [docs/ssh-setup.md](docs/ssh-setup.md)

---

## TUI Dashboard

The Terminal User Interface provides real-time monitoring of your tunnels.

```
┌─────────────────────────────────────────────────────────────────┐
│ Remo TUI Dashboard                                    [RUNNING] │
├─────────────────────────────────────────────────────────────────┤
│ Requests: 1,234  │  Errors: 12  │  Bytes: 45.2 MB  │  Avg: 23ms│
├─────────────────────────────────────────────────────────────────┤
│ Time     Status  Method  Path                        Duration   │
│ 14:32:01 200     GET     /api/users                   15ms      │
│ 14:32:05 201     POST    /api/users                   45ms      │
│ 14:32:08 404     GET     /api/unknown                 5ms       │
│ 14:32:12 500     GET     /api/error                   120ms     │
├─────────────────────────────────────────────────────────────────┤
│ [q]uit  [c]lear  [e]rrors-only  [p]ause  [f]ilter               │
└─────────────────────────────────────────────────────────────────┘
```

### Key Bindings

| Key | Action |
|-----|--------|
| `q` | Quit (with export prompt) |
| `c` | Clear log display |
| `e` | Toggle error-only filter |
| `p` | Pause/resume log updates |
| `f` | Enter filter mode (type to filter) |
| `Esc` | Clear filter / exit filter mode |

### Statistics Display

The header shows real-time session statistics:
- **Requests** — Total HTTP requests processed
- **Errors** — Responses with 4xx/5xx status codes
- **Bytes** — Total data transferred
- **Avg** — Average response latency

### JSON Log Export

When you quit the TUI (`q`), you'll be prompted to export logs:
```
Export request logs to file? (path or 'no'): ./myapp-logs.json
```

Exported logs include full request details:
```json
{
  "timestamp": "2026-02-19T14:32:01Z",
  "method": "GET",
  "path": "/api/users",
  "status": 200,
  "duration_ms": 15,
  "bytes": 2048,
  "client_ip": "203.0.113.42"
}
```

---

## Configuration

### Server Configuration

**Location:** `/etc/remo/server.yaml`

```yaml
# Server listen address (behind nginx)
listen: "127.0.0.1:18080"

# Your domain for subdomain routing
domain: "yourdomain.tld"

# Mode: "standalone" or "behind-proxy"
mode: behind-proxy

# Trusted proxy IPs (for X-Forwarded-For)
trusted_proxies:
  - "127.0.0.1/32"
  - "10.0.0.0/8"

# SSH authorized keys file
authorized: /home/remo/.ssh/authorized_keys

# SQLite state database
state: /var/lib/remo/state.db

# Reserve subdomains on disconnect (allow reconnect)
reserve: true

# Admin API secret (for /status, /metrics endpoints)
admin_secret: your-secure-secret-here
```

### Client Flags Reference

```bash
remo connect [flags]

Flags:
  -s, --server string      SSH server address (required)
  -d, --subdomain string   Subdomain to register (required)
  -u, --upstream string    Local service URL (default: http://127.0.0.1:8080)
  -p, --ssh-port int       SSH server port (default: 22)
  -i, --identity string    Identity file path (default: ~/.remo/identity.json)
      --tui                Enable TUI dashboard
  -v, --verbose            Verbose logging
  -h, --help               Help for connect
```

### File Locations

| Path | Purpose |
|------|---------|
| `/etc/remo/server.yaml` | Server configuration |
| `/home/remo/.ssh/authorized_keys` | SSH authorized keys |
| `/var/lib/remo/state.db` | SQLite state database |
| `~/.remo/identity.json` | Client identity (SSH key) |
| `/etc/systemd/system/remo.service` | Systemd service definition |
| `/var/log/nginx/remo-*.log` | Nginx access/error logs |

---

## Admin Endpoints

Remo exposes HTTP endpoints for monitoring and management.

### Endpoint Reference

| Endpoint | Auth | Description |
|----------|------|-------------|
| `GET /healthz` | None | Health check endpoint |
| `GET /status` | Bearer | JSON status and tunnel information |
| `GET /metrics` | Bearer | Prometheus-compatible metrics |
| `GET /events` | None | Server-sent events stream (localhost only) |

### Authentication

Admin endpoints require the `admin_secret` from server configuration:

```bash
# Using curl with Bearer token
curl -H "Authorization: Bearer your-admin-secret" \
  https://yourserver.com/status

# Using the remo CLI
remo status --server https://yourserver.com --secret your-admin-secret
```

**Security Note:** The `/events` endpoint is restricted to localhost-only access. This ensures event streams are only accessible through the authenticated SSH tunnel, not directly from the internet.

### Example Responses

**Health Check:**
```bash
curl https://yourserver.com/healthz
# Output: OK
```

**Status Endpoint:**
```bash
curl -H "Authorization: Bearer secret" https://yourserver.com/status
```

```json
{
  "version": "0.1.0",
  "uptime": "72h15m30s",
  "tunnels": {
    "active": 5,
    "total": 12
  },
  "subdomains": [
    {"name": "myapp", "client": "alice", "uptime": "2h30m"},
    {"name": "api", "client": "bob", "uptime": "45m"}
  ]
}
```

**Metrics Endpoint:**
```bash
curl -H "Authorization: Bearer secret" https://yourserver.com/metrics
```

```
# HELP remo_requests_total Total HTTP requests
# TYPE remo_requests_total counter
remo_requests_total 15234

# HELP remo_request_duration_seconds Request duration
# TYPE remo_request_duration_seconds histogram
remo_request_duration_seconds_bucket{le="0.1"} 5234
remo_request_duration_seconds_bucket{le="0.5"} 12456
remo_request_duration_seconds_bucket{le="1.0"} 14890
```

---

## Troubleshooting

### Connection Refused

**Problem:** Cannot connect to server

**Solutions:**
```bash
# Check if Remo server is running
sudo systemctl status remo

# Check server logs
sudo journalctl -u remo -f

# Verify SSH port is accessible
nc -zv yourserver.com 22

# Test SSH connection directly
ssh -i ~/.ssh/remo_ed25519 -p 2222 remo@yourserver.com
```

### 502 Bad Gateway

**Problem:** Nginx shows 502 error

**Solutions:**
```bash
# Check if Remo is listening on correct port
sudo ss -tlnp | grep 18080

# Verify nginx upstream configuration
grep proxy_pass /etc/nginx/sites-available/remo

# Check nginx error logs
sudo tail -f /var/log/nginx/remo-error.log

# Test Remo directly
curl http://127.0.0.1:18080/healthz
```

### Permission Denied (SSH)

**Problem:** SSH key authentication fails

**Solutions:**
```bash
# Check key file permissions (client)
ls -la ~/.ssh/remo_ed25519
# Should be: -rw------- (600)
chmod 600 ~/.ssh/remo_ed25519

# Check authorized_keys permissions (server)
sudo su - remo -c "ls -la ~/.ssh/"
# Should be: -rw------- (600) for authorized_keys
chmod 600 ~/.ssh/authorized_keys
chmod 700 ~/.ssh/

# Verify key is in authorized_keys
grep "$(cat ~/.ssh/remo_ed25519.pub)" /home/remo/.ssh/authorized_keys
```

### Subdomain Not Found

**Problem:** Subdomain returns "not found" or 404

**Solutions:**
```bash
# Check if client is still connected
remo status --server yourserver.com --secret your-secret

# Verify subdomain registration
# Look for "Registered subdomain" in client output

# Check DNS resolution
dig +short myapp.yourdomain.tld

# Test without nginx
curl -H "Host: myapp.yourdomain.tld" http://127.0.0.1:18080
```

### TUI Not Showing Logs

**Problem:** TUI dashboard appears but no request logs

**Solutions:**
- Ensure you started with `--tui` flag
- Check that requests are actually hitting your subdomain
- Verify the TUI is not paused (press `p` to toggle)
- Check if filters are active (press `Esc` to clear)

### Server Management Commands

```bash
# View service status
sudo systemctl status remo

# View logs in real-time
sudo journalctl -u remo -f

# Restart service
sudo systemctl restart remo

# Check nginx configuration
sudo nginx -t

# Reload nginx
sudo systemctl reload nginx
```

---

## Development

### Building from Source

```bash
# Clone repository
git clone https://github.com/gleicon/remo.git
cd remo

# Build all binaries
go build ./...

# Run tests
go test ./...

# Build specific binary
go build -o remo-server ./cmd/server
go build -o remo-client ./cmd/client
```

### Project Structure

```
remo/
├── cmd/
│   ├── server/        # Server binary
│   └── client/        # Client binary
├── internal/
│   ├── server/        # Server implementation
│   ├── client/        # Client implementation
│   ├── tui/           # Terminal UI
│   └── ssh/           # SSH tunnel handling
├── docs/              # Documentation
├── scripts/           # Setup scripts
└── README.md          # This file
```

---

## Documentation

- **[docs/nginx.md](docs/nginx.md)** — Production nginx setup with Let's Encrypt SSL
- **[docs/ssh-setup.md](docs/ssh-setup.md)** — SSH key generation and authorization
- **[docs/nginx-example.conf](docs/nginx-example.conf)** — Example production nginx configuration

---

## License

MIT License — see [LICENSE](LICENSE) for details.

---

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

---

## Support

- **Issues:** [GitHub Issues](https://github.com/gleicon/remo/issues)
- **Discussions:** [GitHub Discussions](https://github.com/gleicon/remo/discussions)

---

*Built with Go. Powered by SSH. Made with ♥ for developers who need simple, secure tunnels.*
