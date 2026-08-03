# remo Setup Guide

## Overview

remo is a single-VPS PaaS built on nano-rs. One binary handles the control plane, CLI client, and git hook. Apps are deployed by `git push`.

---

## 0. Pre-flight

Run doctor on the VPS before installing to assess the current state:

```bash
remo server doctor
```

Fixes all `FAIL` items before proceeding. Warnings are safe to address after install.

---

## 1. Prerequisites

- VPS running Ubuntu 22.04+ (tested; other distros should work)
- nginx installed (`apt install nginx certbot python3-certbot-nginx`)
- nano-rs running and listening (typically `http://127.0.0.1:8080` data, `http://127.0.0.1:9000` admin)
- DNS: two A records pointing to the VPS IP

```
*.apps.yourdomain.tld  A  <VPS_IP>    # app subdomains
remo.apps.yourdomain.tld  A  <VPS_IP> # control plane + git SSH
```

The wildcard covers `alice-myapp.apps.yourdomain.tld` but not the apex `apps.yourdomain.tld` or `remo.apps.yourdomain.tld` — those need explicit records.

---

## 2. Server Install (on VPS, as root)

Build and install the binary (no published releases yet):

```bash
git clone https://github.com/gleicon/remo
cd remo && cargo build --release
cp target/release/remo /usr/local/bin/remo
```

Run the installer:

```bash
remo server install --domain apps.yourdomain.tld
```

The installer:
- Creates `/var/lib/remo/` (apps, git) and `/etc/remo/`
- Creates a `git` system user and grants it write access to the data dirs
- Creates `/etc/remo/authorized_keys` (owned by `git`)
- Generates a master token → `/etc/remo/master_token` (chmod 600)
- Initializes SQLite at `/var/lib/remo/state.db`
- Writes nginx wildcard config to `/etc/nginx/sites-enabled/remo-wildcard.conf`
- Writes nginx control-plane config to `/etc/nginx/sites-enabled/remo-control.conf`
- Prints the master token once — **save it**

After install:

```bash
# Reload nginx
systemctl reload nginx

# Start the remo control plane
remo server start
```

### Optional: Caddy instead of nginx

```bash
remo server install --domain apps.yourdomain.tld --proxy caddy
```

Caddy uses on-demand TLS — no certbot needed, no per-app cert commands. Requires Caddy 2.x installed.

---

## 3. Admin Laptop — First Login

Build and install the binary on your laptop:

```bash
git clone https://github.com/gleicon/remo
cd remo && cargo build --release
sudo cp target/release/remo /usr/local/bin/remo
```

Run the interactive setup. It will create your user account, register your SSH key server-side, and configure `~/.remo/config.toml` and `~/.ssh/config`:

```bash
remo setup
```

Prompts:
1. Server URL — e.g. `https://cloud.remoapps.site`
2. Setup as: `1` (Admin — master token)
3. Master token — the one printed by `remo server install`
4. Your username
5. SSH key — generate a new one or use an existing key

After this completes, `git push remo main` works.

The master token grants full admin access. After setup, your CLI is configured with a scoped **user token** — you only need the master token again to create or revoke other users.

---

## 4. Inviting Users (Alice)

Create a single-use invite link for Alice (expires in 1 hour by default):

```bash
remo users invite alice --email alice@example.com
```

Output:
```
Invite created for 'alice' (expires 2026-08-02T16:00:00Z)

Send this command to the user (shown once):
  remo setup --invite <token>
```

Send Alice the `remo setup --invite <token>` line. She runs it on her laptop:

```bash
# Install remo binary (same as above)
remo setup --invite <token>
```

This:
1. Validates the invite with the server
2. Generates or picks her SSH key locally
3. Registers her key server-side (written to `/etc/remo/authorized_keys`)
4. Creates her account and saves her user token to `~/.remo/config.toml`

The invite token is single-use and expires after 1 hour. If it expires before Alice uses it, create a new one with `remo users invite alice`.

To extend the expiry window (e.g. 24 hours):

```bash
remo users invite alice --expires 86400
```

List pending invites:

```bash
remo users invites
```

---

## 5. Deploy Your First App

```bash
# Create the app record on the server
remo apps create myapp

# Write a minimal JS app
cat > index.js << 'EOF'
addEventListener("fetch", (e) =>
  e.respondWith(new Response("hello from remo", { status: 200 }))
);
EOF

git init
git add index.js && git commit -m "initial"

# Add the git remote — path is just /appname (server identifies you by SSH key)
git remote add remo ssh://cloud.remoapps.site/myapp

git push remo main
```

Deployed app is live at `https://alice-myapp.cloud.remoapps.site` (hostname is `{owner}-{name}.{domain}`).

---

## 6. nano.json

> **Not yet implemented** — `nano.json` is parsed by the deploy hook but the parsing is not wired up yet. Fields set here have no effect until that gap is closed. Use `remo apps create` flags and `remo env set` for now.

Place `nano.json` at the repo root (optional):

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
1. Admin runs: `remo users invite <name> --email <email>`
2. Admin sends the printed `remo setup --invite <token>` command to the developer
3. Developer runs it on their laptop — account created, SSH key registered, client configured
4. Developer can immediately `git push`

No manual SSH key copying or authorized_keys editing required.

---

## Ports and Firewall

| Port | Service |
|------|---------|
| 80/443 | nginx/Caddy (public) |
| 8080 | nano-rs data plane (localhost only) |
| 9000 | nano-rs admin API (localhost only) |
| 7070 | remo control plane (localhost only) |

Only 80 and 443 should be externally reachable.
