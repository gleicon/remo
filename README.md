# remo

Single-binary reverse tunnel that exposes local services through public
`*.yourdomain.tld` subdomains. Runs standalone with its own TLS or behind an
existing nginx/reverse-proxy on your VPS.

## Installation

### Quick install

```bash
# Install remo binary
curl -sL https://raw.githubusercontent.com/gleicon/remo/main/scripts/remo-installer.sh | sh

# Or with custom directory
curl -sL https://raw.githubusercontent.com/gleicon/remo/main/scripts/remo-installer.sh | sh -s -- -b /usr/local/bin
```

Or download from [GitHub releases](https://github.com/gleicon/remo/releases).

### Setup

```bash
# Server (VPS)
curl -sL https://raw.githubusercontent.com/gleicon/remo/main/scripts/remo-setup.sh | bash -s -- server \
  --domain yourdomain.tld \
  --email you@example.com

# Client (laptop)
curl -sL https://raw.githubusercontent.com/gleicon/remo/main/scripts/remo-setup.sh | bash -s -- client
```

This creates the required directories, generates SSH host key, identity, config, and systemd unit.

## Quick start

### 1. Server (VPS)

The setup script above handles everything. Or manually:

```bash
# Generate SSH host key
ssh-keygen -t ed25519 -f /etc/remo/host_key -N ""

# Run server
remo server --config /etc/remo/server.yaml
```

### 2. Client (laptop)

```bash
# Generate identity
remo auth init

# Connect
remo connect \
  --server user@yourserver:22 \
  --subdomain myapp \
  --upstream http://127.0.0.1:3000
```

Your local port 3000 is now exposed at `https://myapp.yourdomain.tld`.

---

## Configuration

All configuration is done via YAML config file. The server reads from
`/etc/remo/server.yaml` by default, or specify `--config path/to/config.yaml`.

### Server config (`/etc/remo/server.yaml`)

```yaml
# HTTP server listen address
listen: ":443"

# Your domain
domain: "yourdomain.tld"

# Mode: standalone (TLS terminates here) or behind-proxy
mode: standalone

# TLS certificates (standalone mode only)
tls_cert: /etc/remo/fullchain.pem
tls_key: /etc/remo/privkey.pem

# Authorized client public keys
authorized: /etc/remo/authorized.keys

# SQLite database for state
state: /var/lib/remo/state.db

# Auto-reserve subdomains for authorized keys
reserve: true

# Allow clients to request random subdomains
allow_random: true

# Admin secret for /status and /metrics endpoints
admin_secret: changeme
```

### Client

The client uses command-line flags only (no config file needed):

```bash
remo connect \
  --server user@yourserver:22 \
  --subdomain myapp \
  --upstream http://127.0.0.1:3000 \
  --tui
```

---

## File locations

| Path | Purpose |
|------|---------|
| `/etc/remo/server.yaml` | Server configuration |
| `/etc/remo/authorized.keys` | Allowed client public keys |
| `/etc/remo/fullchain.pem` | TLS certificate |
| `/etc/remo/privkey.pem` | TLS private key |
| `/var/lib/remo/state.db` | SQLite state database |
| `~/.remo/identity.json` | Client ed25519 identity |
| `/etc/systemd/system/remo.service` | Systemd service unit |

---

## Running the server

### With systemd (recommended)

```bash
sudo systemctl enable --now remo
sudo systemctl status remo
sudo journalctl -u remo -f
```

### Manually

```bash
remo server --config /etc/remo/server.yaml
```

### Behind nginx

In `behind-proxy` mode, remo listens on an internal port and expects nginx to
proxy requests:

```yaml
listen: "127.0.0.1:18080"
domain: "yourdomain.tld"
mode: behind-proxy
trusted_proxies:
  - "127.0.0.1/32"
```

Nginx config:

```nginx
server {
    listen 443 ssl;
    server_name *.yourdomain.tld;

    ssl_certificate /etc/remo/fullchain.pem;
    ssl_certificate_key /etc/remo/privkey.pem;

    location / {
        proxy_pass http://127.0.0.1:18080;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

---

## Connecting a client

### Generate identity

```bash
remo auth init -o ~/.remo/identity.json
```

The public key is printed to stdout. Add it to the server's
`/etc/remo/authorized.keys`:

```
<YOUR_PUBLIC_KEY> *
```

### Connect

```bash
remo connect \
  --server user@yourdomain.tld:22 \
  --subdomain myapp \
  --upstream http://127.0.0.1:3000 \
  -i ~/.remo/identity.json
```

The `--server` flag takes a `user@host:port` format (SSH connection string).

### Random subdomains

Omit `--subdomain` to get a random one:

```bash
remo connect --server user@yourdomain.tld:22 --upstream http://127.0.0.1:3000
```

The server assigns a random name (e.g., `a3f9c2b1`) and the client logs it.

---

## TUI

Add `-tui` or `--tui` to see a live request log:

```bash
remo connect --server user@yourdomain.tld:22 --subdomain myapp --upstream http://127.0.0.1:3000 --tui
```

Controls:
- `/` — filter
- `e` — errors only
- `p` — pause
- `c` — clear

---

## Admin endpoints

The server exposes admin endpoints (requires `Authorization: Bearer <admin-secret>`):

| Endpoint | Description |
|---------|-------------|
| `GET /healthz` | Health check (no auth) |
| `GET /status` | JSON: tunnels, keys, reservations |
| `GET /metrics` | Prometheus metrics |

Query via CLI:

```bash
remo status --server http://127.0.0.1:18080 --secret changeme
```

---

## Server management

```bash
# List authorized keys
remo keys list --state /var/lib/remo/state.db

# Add a key
remo keys add --state /var/lib/remo/state.db --pubkey BASE64 --prefix myapp-*

# List reservations
remo reservations list --state /var/lib/remo/state.db
```

---

## How it works

1. Client connects to server via SSH and opens a `remo-proxy` channel
2. Client sends signed `Hello` with subdomain claim
3. Server validates signature against authorized keys, registers tunnel
4. HTTP requests to `subdomain.yourdomain.tld` are proxied over SSH to the client
5. Client forwards to local upstream and returns response

---

## Development

```bash
# Build
make build

# Test
make test

# Cross-compile
make dist
```

Local testing without TLS:

```bash
# Terminal 1: upstream
python3 -m http.server 3000

# Terminal 2: server (behind-proxy mode)
remo server --config /etc/remo/server.yaml

# Terminal 3: client
remo connect --server user@localhost:22 --subdomain test --upstream http://127.0.0.1:3000

# Terminal 4: test
curl -H "Host: test.yourdomain.tld" http://localhost:18080/
```

---

## Security notes

- TLS certificates should be mode 600: `chmod 600 /etc/remo/*.pem`
- Identity file is mode 600: `chmod 600 ~/.remo/identity.json`
- Authorized keys file is mode 600: `chmod 600 /etc/remo/authorized.keys`
- The SSH server needs a host key — generate one with:
  `ssh-keygen -t ed25519 -f /etc/remo/host_key`
