# remo Setup Guide

## Overview

remo is a single-VPS PaaS built on nano-rs. One binary handles the control plane, CLI client, and git hook. Apps are deployed by `git push`.

---

## 1. Prerequisites

- VPS running Ubuntu 22.04+ (tested; other distros should work)
- nginx installed (`apt install nginx certbot python3-certbot-nginx`)
- nano-rs running and listening (typically `http://127.0.0.1:8080` data, `http://127.0.0.1:9000` admin)
- DNS: one wildcard A record pointing to the VPS IP

```
*.apps.yourdomain.tld  A  <VPS_IP>
```

---

## 2. Server Install (on VPS, as root)

Download the binary:

```bash
curl -fsSL https://github.com/gleicon/remo/releases/latest/download/remo-linux-amd64 -o /usr/local/bin/remo
chmod +x /usr/local/bin/remo
```

Run the installer:

```bash
remo server install --domain apps.yourdomain.tld
```

The installer:
- Creates `/var/lib/remo/` (apps, git) and `/etc/remo/`
- Generates a master token → `/etc/remo/master_token` (chmod 600)
- Initializes SQLite at `/var/lib/remo/state.db`
- Writes nginx wildcard config to `/etc/nginx/sites-enabled/remo-wildcard.conf`
- Prints the master token once — **save it**

After install:

```bash
# Reload nginx
systemctl reload nginx

# Start the remo control plane (systemd unit auto-created)
remo server start
# or: systemctl start remo
```

### Optional: Caddy instead of nginx

```bash
remo server install --domain apps.yourdomain.tld --proxy caddy
```

Caddy uses on-demand TLS — no certbot needed, no per-app cert commands. Requires Caddy 2.x installed.

---

## 3. Create the Admin User (Alice)

Alice is the first human user. The master token is admin — share it only with the server operator. For a developer account:

```bash
remo users add alice --pubkey "ssh-rsa AAAA...alice@laptop"
```

Output: a scoped token for Alice. Share it out-of-band (Slack, 1Password, etc.).

Alice adds to her `~/.ssh/authorized_keys` on the VPS:

```
command="remo git-hook --user alice" ssh-rsa AAAA...alice@laptop
```

Or use `/etc/remo/authorized_keys` (pointed to by `/etc/ssh/sshd_config AuthorizedKeysFile`).

---

## 4. Laptop Setup (Alice's machine)

Install the binary:

```bash
curl -fsSL https://github.com/gleicon/remo/releases/latest/download/remo-darwin-arm64 -o /usr/local/bin/remo
chmod +x /usr/local/bin/remo
```

Log in:

```bash
remo login --server https://remo.yourdomain.tld --token <alice-token>
```

Config saved to `~/.remo/config.toml`. No other init needed.

---

## 5. Deploy Your First App

```bash
# Create the app on the server
remo apps create myapp --type js --entrypoint index.js

# In your project directory
git init
git remote add remo git@apps.yourdomain.tld:myapp
git add -A && git commit -m "initial"
git push remo main
```

Deployed app is live at `https://myapp.apps.yourdomain.tld`.

---

## 6. nano.json

Place `nano.json` at the repo root (optional but recommended):

```json
{
  "type": "js",
  "entrypoint": "index.js",
  "cpu_time_ms": 500,
  "memory_mb": 64,
  "workers": 2
}
```

Fields:
- `type`: `js` | `wasm` | `static`
- `entrypoint`: main file path
- `cpu_time_ms`, `memory_mb`, `workers`: resource limits

Secrets never go in `nano.json`. Use `remo env set myapp KEY=VALUE`.

---

## 7. Common Operations

```bash
# Environment variables
remo env set myapp DATABASE_URL=postgres://...
remo env list myapp
remo env unset myapp DATABASE_URL

# Deployment history
remo deployments myapp

# Roll back
remo rollback myapp             # previous deploy
remo rollback myapp <sha>       # specific deploy

# Scale
remo scale myapp 4

# Logs
remo logs myapp --lines 200
```

---

## 8. Static Sites

```json
// nano.json
{ "type": "static", "entrypoint": "dist/" }
```

Commit your `dist/` build output. remo maps the directory to nano-rs `StaticDir` handler — no nginx/Caddy routing changes needed.

---

## 9. Multi-User Setup

Each developer:
1. Gets a user account: `remo users add <name> --pubkey <key>`
2. Receives a scoped token (owns their own apps only)
3. Runs `remo login` on their laptop
4. Gets a forced-command SSH entry for git push

---

## Ports and Firewall

| Port | Service |
|------|---------|
| 80/443 | nginx/Caddy (public) |
| 8080 | nano-rs data plane (localhost only) |
| 9000 | nano-rs admin API (localhost only) |
| 7070 | remo control plane (localhost only) |

Only 80 and 443 should be externally reachable.
