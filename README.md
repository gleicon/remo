# remo

Single-binary reverse tunnel that exposes local services through public
`*.yourdomain.tld` subdomains. Uses SSH reverse tunnels.

## Quick Start

### Client (laptop)

```bash
curl -sL https://raw.githubusercontent.com/gleicon/remo/main/scripts/remo-setup.sh | bash
```

This installs the binary and generates your identity. The script shows your public key.

### Server (VPS)

```bash
# Full setup (requires root)
sudo curl -sL https://raw.githubusercontent.com/gleicon/remo/main/scripts/remo-setup.sh | bash -s -- server \
  --domain yourdomain.tld

# Behind nginx
sudo curl -sL https://raw.githubusercontent.com/gleicon/remo/main/scripts/remo-setup.sh | bash -s -- server \
  --domain yourdomain.tld \
  --behind-proxy \
  --skip-certs
```

---

## Connecting

After setting up client and server:

```bash
remo connect --server yourserver.com --subdomain myapp --upstream http://127.0.0.1:3000
```

Your service is now available at `https://myapp.yourdomain.tld`.

---

## File Locations

| Path | Purpose |
|------|---------|
| `/etc/remo/server.yaml` | Server configuration |
| `/home/remo/.ssh/authorized_keys` | SSH authorized keys (add client public keys) |
| `/var/lib/remo/state.db` | SQLite state database |
| `~/.remo/identity.json` | Client identity |
| `/etc/systemd/system/remo.service` | Systemd service (if installed) |

---

## Server Management

```bash
sudo systemctl status remo
sudo journalctl -u remo -f
sudo systemctl restart remo
```

---

## Configuration

### Server config (`/etc/remo/server.yaml`)

```yaml
listen: "127.0.0.1:18080"
domain: "yourdomain.tld"
mode: behind-proxy
trusted_proxies:
  - "127.0.0.1/32"
authorized: /etc/remo/authorized.keys
state: /var/lib/remo/state.db
reserve: true
admin_secret: your-secret-here
```

### Client

```bash
remo connect \
  --server yourserver.com \
  --subdomain myapp \
  --upstream http://127.0.0.1:3000 \
  --tui
```

---

## Adding Client Keys

On server, add client public keys to `/home/remo/.ssh/authorized_keys`:

```bash
echo "BASE64_PUBLIC_KEY *" | sudo tee -a /home/remo/.ssh/authorized_keys
sudo chmod 600 /home/remo/.ssh/authorized_keys
```

Format: `<base64_public_key> <subdomain_rule>`
- `*` - allows any subdomain
- `myapp-*` - allows subdomains starting with `myapp-`
- `myapp` - allows only exact subdomain

---

## Admin Endpoints

Requires `Authorization: Bearer <admin-secret>`:

| Endpoint | Description |
|----------|-------------|
| `GET /healthz` | Health check (no auth) |
| `GET /status` | JSON status |
| `GET /metrics` | Prometheus metrics |

```bash
remo status --server http://127.0.0.1:18080 --secret your-secret
```

---

## How It Works

1. Client connects to server via SSH (port 22)
2. Client opens reverse tunnel - listens on a local port on server
3. Client registers subdomain via HTTP through the SSH tunnel
4. Server proxies HTTP requests to the tunnel port
5. SSH tunnel forwards to client's local upstream

---

## Development

```bash
go build ./...
go test ./...
```
