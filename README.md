# Remo

[![Go Version](https://img.shields.io/badge/go-1.21+-00ADD8.svg)](https://golang.org/)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

**Self-hosted reverse tunnel with SSH and full-screen TUI dashboard**

Expose local services through public `*.yourdomain.tld` subdomains using standard SSH. No custom protocols, no complex configuration—just a single binary with htop-style interface.

---

## What is Remo?

Remo is a lightweight reverse tunnel solution that lets you expose local development or production services to the internet through clean, memorable subdomains. It uses standard SSH reverse tunnels under the hood, giving you battle-tested security without the complexity of custom protocols.

### Architecture

```
Internet → Nginx (443/SSL) → Remo Server (18080) → SSH Tunnel → Your Local Service
```

**Key Components:**
- **Client** — Runs on your laptop, creates SSH reverse tunnel
- **Server** — Runs on your VPS, routes by subdomain, tracks tunnel health
- **Nginx** — Optional SSL termination and reverse proxy

---

## Quick Start

### 1. Install Server (on Ubuntu VPS)

```bash
# Run as root
sudo curl -sL https://raw.githubusercontent.com/gleicon/remo/main/install.sh | bash
```

**Interactive prompts:**
- Domain: `yourdomain.com`
- Behind nginx: `Y` (recommended)
- Client SSH keys: paste your `ssh-ed25519 AAAAC3...` key

### 2. Install Client (on your laptop)

```bash
# Run as regular user
curl -sL https://raw.githubusercontent.com/gleicon/remo/main/install.sh | bash
```

**Interactive prompts:**
- Server domain: `yourdomain.com`
- SSH key: `~/.ssh/id_ed25519` (or generate new)

### 3. Connect with TUI

```bash
remo connect --server yourdomain.com --subdomain myapp \
  --upstream http://127.0.0.1:3000 --tui
```

Your service is now live at: **`https://myapp.yourdomain.com`**

---

## TUI Dashboard

The full-screen Terminal User Interface provides real-time monitoring with htop-style layout.

```
┌─ myapp | connected | https://myapp.yourdomain.com ───────────────┐
│ Requests: 42 │ Errors: 1 │ Bytes: 1.2KB/45KB │ Latency: 28ms  │
├──────────────────────────────────────────────────────────────────┤
│ Time     Method  Path              Status  Latency  Remote        │
│ 14:32:05 GET     /                 200     35ms     192.168.1.100│
│ 14:32:04 POST    /api/users        201     120ms    10.0.0.5      │
│ 14:32:01 GET     /docs/README.md   200     28ms     172.16.0.10   │
├──────────────────────────────────────────────────────────────────┤
│ q:quit  p:pause  c:clear  e:errors  /:filter  Tab:connections     │
└──────────────────────────────────────────────────────────────────┘
```

### Views

**Logs View** (default): Shows real-time HTTP requests
- Time, Method, Path, Status (color-coded), Latency, Remote IP
- Status colors: 🟢 2xx, 🔵 3xx, 🟡 4xx, 🔴 5xx

**Connections View** (press `Tab`): Shows your active tunnels
```
┌─ Connections ────────────────────────────────────────────────────┐
│ Subdomain │ Status │ Uptime  │ Port   │ Last Ping              │
│ myapp     │ ● ON   │ 15m 23s │ 38421  │ 3s ago                 │
│ test      │ ◐ STALE│ 5m 7s   │ 34291  │ 2m ago                 │
├──────────────────────────────────────────────────────────────────┤
│ Navigation: ↑/↓  Kill: x  Refresh: r  Back: Tab                 │
└──────────────────────────────────────────────────────────────────┘
```

### Keyboard Shortcuts

| Key | Action |
|-----|--------|
| `Tab` | Switch between Logs and Connections views |
| `↑/↓` | Navigate (in Connections view) |
| `x` | Kill selected connection (Connections view) |
| `r` | Force refresh connections |
| `q` | Quit with export prompt |
| `Ctrl+C` | Quit immediately |
| `c` | Clear logs |
| `e` | Toggle errors-only filter |
| `p` | Pause/resume updates |
| `/` | Enter filter mode |
| `Esc` | Cancel filter |

---

## CLI Commands

### Manage Connections

```bash
# List your active connections
remo connections

# Kill a specific connection
remo kill myapp

# Kill all your connections
remo kill --all
```

### Connect Options

```bash
remo connect [flags]

Flags:
  -s, --server string      SSH server address (user@host:port)
  -d, --subdomain string   Subdomain to register
  -u, --upstream string    Local service URL (default: http://127.0.0.1:8080)
  -i, --identity string    Identity file path (default: ~/.remo/identity.json)
  --tui                    Enable full-screen TUI dashboard
  --refresh-interval int   TUI refresh interval in seconds (default: 5)
  -v, --verbose            Verbose logging
  -h, --help               Help for connect
```

---

## SSH Key Authentication

Remo uses SSH public key authentication. The setup script handles this automatically, but here's the manual process:

### Generate SSH Key (Client)

```bash
ssh-keygen -t ed25519 -C "remo-$(whoami)" -f ~/.ssh/id_ed25519
```

### Add to Server

```bash
# On server as remo user
cat ~/.ssh/id_ed25519.pub >> ~/.ssh/authorized_keys
chmod 600 ~/.ssh/authorized_keys
```

### Subdomain Authorization Rules

In `/home/remo/.ssh/authorized_keys`, each line is: `key comment subdomain-rule`

```
# Allow any subdomain
ssh-ed25519 AAAAC3... user@laptop *

# Allow only specific subdomain
ssh-ed25519 AAAAC3... user@laptop myapp

# Allow pattern
ssh-ed25519 AAAAC3... user@laptop dev-*
```

---

## Production Setup with Nginx

### Why Nginx?
- SSL/TLS termination with Let's Encrypt
- WebSocket support
- Better performance
- Security headers and best practices

### Quick Setup

```bash
# 1. Install nginx and certbot
sudo apt install nginx certbot python3-certbot-nginx

# 2. Get SSL certificate
sudo certbot --nginx -d yourdomain.com -d '*.yourdomain.com'

# 3. Configure nginx site
sudo tee /etc/nginx/sites-available/yourdomain.com << 'EOF'
server {
    listen 443 ssl;
    server_name *.yourdomain.com;

    ssl_certificate /etc/letsencrypt/live/yourdomain.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/yourdomain.com/privkey.pem;

    location / {
        proxy_pass http://127.0.0.1:18080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_http_version 1.1;
    }
}
EOF

# 4. Enable and restart
sudo ln -s /etc/nginx/sites-available/yourdomain.com /etc/nginx/sites-enabled/
sudo systemctl restart nginx
```

---

## Configuration

### Server Config: `/etc/remo/server.yaml`

```yaml
# Listen address (behind nginx)
listen: "127.0.0.1:18080"

# Domain for wildcard routing
domain: "yourdomain.com"

# Mode: "standalone" or "behind-proxy"
mode: behind-proxy

# Trusted proxies for X-Forwarded-For
trusted_proxies:
  - "127.0.0.1/32"

# SSH authorized keys
authorized: /home/remo/.ssh/authorized_keys

# SQLite state database
state: /var/lib/remo/state.db

# Tunnel timeout (auto-cleanup stale tunnels)
tunnel_timeout: 5m

# Reserve subdomains on disconnect
reserve: true

# Admin secret (for /admin/* endpoints)
admin_secret: your-secure-secret-here
```

### Client Config: `~/.remo/config.yaml`

```yaml
server: "yourdomain.com"
refresh_interval: 5s
tui_mode: fullscreen
```

---

## File Locations

| File | Purpose |
|------|---------|
| `/etc/remo/server.yaml` | Server configuration |
| `/home/remo/.ssh/authorized_keys` | SSH authorized keys |
| `/var/lib/remo/state.db` | Server SQLite database |
| `~/.remo/identity.json` | Client SSH identity |
| `~/.remo/config.yaml` | Client configuration |
| `~/.remo/state.json` | Client connection state |
| `/etc/systemd/system/remo.service` | Systemd service |

---

## Security

### SSH Security
- Ed25519 keys (modern, secure)
- No password authentication
- Standard SSH protocol (no custom crypto)

### Tunnel Security
- Reverse tunnel means server initiates no outbound connections
- Your local service is never directly exposed
- All traffic encrypted via SSH tunnel

### Rate Limiting
- Admin endpoints: 5 attempts per minute per IP
- Prevents brute force on management APIs

### Error Handling
- Returns 404 for all errors (prevents subdomain enumeration)
- No information leakage in error messages
- Debug headers only visible with proper authorization

### State Files
- Client state: `0600` permissions (owner only)
- No SSH private keys stored in state
- Only connection metadata: subdomain, pid, port, timestamps

---

## Troubleshooting

### Connection Refused
```bash
# Check server is running
ssh your-server 'sudo systemctl status remo'

# Check SSH connectivity
ssh -v remo@your-server

# Check firewall
ssh your-server 'sudo ufw status'
```

### Subdomain Already Reserved
```bash
# Kill existing connection
remo kill myapp

# Or wait for automatic cleanup (5 minutes)
```

### TUI Not Full-Screen
The TUI uses alternate screen buffer (like vim). If it doesn't clear screen:
1. Check terminal supports escape sequences
2. Try: `export TERM=xterm-256color`
3. Use `--tui` flag (required for full-screen)

### Client IP Shows 127.0.0.1
Ensure nginx adds `X-Forwarded-For` header:
```nginx
proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
```

---

## Building from Source

```bash
# Clone
git clone https://github.com/gleicon/remo.git
cd remo

# Build
go build -o remo ./cmd/remo

# Cross-compile for server
GOOS=linux GOARCH=amd64 go build -o remo-linux ./cmd/remo
```

---

## Documentation

- [SSH Setup Guide](docs/ssh-setup.md) — Detailed SSH key configuration
- [Nginx Setup](docs/nginx.md) — Production nginx configuration
- [API Reference](docs/api.md) — Server endpoints and API

---

## License

MIT License — see [LICENSE](LICENSE) file.
