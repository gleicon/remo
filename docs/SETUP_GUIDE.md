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

On the **operator's laptop**, log in with the master token printed by the installer:

```bash
remo login --server https://remo.apps.yourdomain.tld --token <master-token>
```

Config saved to `~/.remo/config.toml`. The master token grants full admin access — do not share it with developers.

Create a developer account:

```bash
remo users add alice --pubkey "ssh-rsa AAAA...alice@laptop"
```

Output: a scoped token for Alice. Share it out-of-band (Slack, 1Password, etc.).

Create a `git` user on the VPS (no shell, no home login):

```bash
adduser --system --shell /usr/sbin/nologin --no-create-home git
```

Add to `/etc/remo/authorized_keys` (the file was created by the installer, owned by `git`):

```
command="/usr/local/bin/remo git-hook --user alice" ssh-rsa AAAA...alice@laptop
```

Point sshd at it **only for the git user** in `/etc/ssh/sshd_config`. A global `AuthorizedKeysFile` directive replaces `~/.ssh/authorized_keys` for all users and will lock out admin SSH access — use a `Match` block:

```
Match User git
    AuthorizedKeysFile /etc/remo/authorized_keys
```

Then `systemctl reload sshd`.

---

## 4. Developer Laptop (Alice's machine)

Build and install the binary (no published releases yet):

```bash
git clone https://github.com/gleicon/remo
cd remo && cargo build --release
cp target/release/remo /usr/local/bin/remo
```

Log in with the scoped token the admin gave you:

```bash
remo login --server https://remo.apps.yourdomain.tld --token <alice-token>
```

Config saved to `~/.remo/config.toml`.

---

## 5. Deploy Your First App

```bash
# Create the app on the server
remo apps create myapp --type js --entrypoint index.js

# In your project directory
git init
git remote add remo git@remo.apps.yourdomain.tld:myapp
git add -A && git commit -m "initial"
git push remo main
```

Deployed app is live at `https://alice-myapp.apps.yourdomain.tld` (hostname is `{owner}-{name}.{domain}`).

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
