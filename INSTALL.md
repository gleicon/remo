# Installation Guide

Quick installation instructions for Remo.

---

## Quick Install (Recommended)

### Step 1: Server (VPS)

```bash
# SSH to your VPS as root
ssh root@your-vps-ip

# Run installer
curl -sL https://raw.githubusercontent.com/gleicon/remo/main/install.sh | bash
```

**You'll be prompted for:**
- Domain name (e.g., `yourdomain.com`)
- Deployment mode (standalone or behind nginx)
- Email (for Let's Encrypt if standalone)
- Client SSH keys (paste your public key)

### Step 2: Client (Your Laptop)

```bash
# Run installer
curl -sL https://raw.githubusercontent.com/gleicon/remo/main/install.sh | bash
```

**You'll be prompted for:**
- Server domain
- SSH key (creates new if needed)
- Test connection

### Step 3: Connect

```bash
remo connect --server yourdomain.com --subdomain myapp \
  --upstream http://127.0.0.1:3000 --tui
```

Done! Visit: `https://myapp.yourdomain.com`

---

## Manual Install

### Server

**1. Download binary:**
```bash
# Linux AMD64
curl -L https://github.com/gleicon/remo/releases/download/v0.1.4/remo_linux_x86_64.tar.gz | tar xz
sudo mv remo /usr/local/bin/
```

**2. Create user:**
```bash
sudo useradd --system --create-home --shell /bin/bash remo
```

**3. Create config `/etc/remo/server.yaml`:**
```yaml
listen: "127.0.0.1:18080"
domain: "yourdomain.com"
mode: behind-proxy
trusted_proxies:
  - "127.0.0.1/32"
authorized: /etc/remo/authorized.keys
state: /var/lib/remo/state.db
tunnel_timeout: 5m
reserve: true
admin_secret: your-secure-secret
```

**4. Setup authorized keys:**
```bash
echo "PASTE_CLIENT_PUBLIC_KEY_HERE *" | sudo tee /etc/remo/authorized.keys
sudo chmod 600 /etc/remo/authorized.keys
```

**5. Create systemd service `/etc/systemd/system/remo.service`:**
```ini
[Unit]
Description=Remo Server
After=network-online.target

[Service]
Type=simple
User=remo
ExecStart=/usr/local/bin/remo server --config /etc/remo/server.yaml
Restart=on-failure

[Install]
WantedBy=multi-user.target
```

**6. Start:**
```bash
sudo systemctl daemon-reload
sudo systemctl enable remo
sudo systemctl start remo
```

### Client

**1. Download binary:**
```bash
# macOS ARM64 (Apple Silicon)
curl -L https://github.com/gleicon/remo/releases/download/v0.1.4/remo_darwin_arm64.tar.gz | tar xz

# macOS Intel
curl -L https://github.com/gleicon/remo/releases/download/v0.1.4/remo_darwin_x86_64.tar.gz | tar xz

# Linux
curl -L https://github.com/gleicon/remo/releases/download/v0.1.4/remo_linux_x86_64.tar.gz | tar xz

sudo mv remo /usr/local/bin/
```

**2. Generate identity:**
```bash
remo auth init -out ~/.remo/identity.json
remo auth inspect -f ~/.remo/identity.json
# Copy the public key and give to server admin
```

**3. Create config `~/.remo/config.yaml`:**
```yaml
server: "yourdomain.com"
```

**4. Connect:**
```bash
remo connect --subdomain myapp --upstream http://127.0.0.1:3000 --tui
```

---

## Build from Source

**Requirements:**
- Go 1.21+
- Make (optional)

**Build:**
```bash
git clone https://github.com/gleicon/remo.git
cd remo
go build -o remo ./cmd/remo

# Cross-compile for server
GOOS=linux GOARCH=amd64 go build -o remo-linux ./cmd/remo
```

---

## Verify Installation

**Server:**
```bash
# Check service
sudo systemctl status remo

# Check logs
sudo journalctl -u remo -f

# Test health endpoint
curl http://127.0.0.1:18080/healthz
```

**Client:**
```bash
# Check version
remo --version

# Test SSH connection
ssh -v remo@yourdomain.com "echo OK"

# Connect with verbose logging
remo connect --server yourdomain.com --subdomain test \
  --upstream http://127.0.0.1:3000 -v
```

---

## Troubleshooting

### "Connection refused"
```bash
# Check server is listening
ssh yourserver 'sudo ss -tlnp | grep 18080'

# Check firewall
ssh yourserver 'sudo ufw status'
```

### "Permission denied"
```bash
# Verify SSH key is in authorized_keys
ssh yourserver 'sudo cat /home/remo/.ssh/authorized_keys'

# Check permissions
ssh yourserver 'ls -la /home/remo/.ssh/'
```

### "Subdomain already reserved"
```bash
# Kill existing connection
remo kill myapp

# Or wait 5 minutes for automatic cleanup
```

---

## Next Steps

- [SSH Setup Guide](../docs/ssh-setup.md) — Detailed key management
- [Nginx Setup](../docs/nginx.md) — Production SSL setup
- [API Reference](../docs/api.md) — Server endpoints
