# remo

Single-VPS PaaS. `git push` deploys JS/WASM apps to a nano-rs edge runtime. No Kubernetes, no Docker per-app, no build step on the server.

```
git push remo main  →  extract  →  symlink swap  →  nano-rs reload
```

Apps live at `{owner}-{app}.{domain}`. Optional custom domain via `PUT /api/apps/:name/domain`.

## Stack

- **remo** — control plane (Rust/axum), runs on `:7070`
- **nano-rs** — V8-based JS runtime, runs on `:8080` (proxy) + `:8889` (admin API)
- **remo-sshd** — dedicated SSH container for git push, port `2222`
- **SQLite** — state: apps, users, deployments, env vars, invites
- **nginx/Caddy** — TLS termination, routes by `Host:` header to nano-rs

## Server setup

**Prerequisites:** VPS with Docker, nginx/Caddy with TLS, ports 443 and 2222 open.

```bash
git clone https://github.com/gleicon/remo
cd remo

# Write server config
sudo mkdir -p /etc/remo /var/lib/remo
sudo tee /etc/remo/server.toml <<EOF
domain      = "apps.example.com"
data_dir    = "/var/lib/remo"
nano_socket = "http://127.0.0.1:8889"
bind        = "0.0.0.0:7070"
EOF

# Initialize master token
sudo remo server init

# Start stack
docker compose up -d
```

nginx proxies `cloud.example.com → 127.0.0.1:7070` and `*.apps.example.com → 127.0.0.1:8080`.

## Admin first login (on your laptop)

Install the remo CLI:

```bash
cargo install --git https://github.com/gleicon/remo
```

Or download a pre-built binary from [releases](https://github.com/gleicon/remo/releases).

Run setup:

```bash
remo setup
# Server URL: https://cloud.example.com
# Choice: 1 (Admin)
# Master token: <from /etc/remo/master_token on VPS>
# Username: alice
# SSH key: pick existing or generate
```

This creates your user account and writes `~/.ssh/config` with the correct port and identity file.

## Invite a user

```bash
# Admin creates invite (single-use, 1h expiry)
remo users invite bob --email bob@example.com

# Output:
#   Claim command:  remo setup --invite <token>
#   Expires:        2026-01-01T13:00:00Z
```

Send the claim command to the new user. They run it on their laptop:

```bash
remo setup --invite <token>
# Picks or generates SSH key, claims account, writes ~/.ssh/config
```

## Deploy an app

```bash
# Create app (admin or app owner)
remo apps create myapp

# Add remote and push
git remote add remo ssh://cloud.example.com/myapp
git push remo main
```

App is live at `alice-myapp.apps.example.com`.

## Environment variables

```bash
remo env set myapp KEY=value
remo env list myapp
remo env unset myapp KEY
```

Vars are pushed to the running worker on every set/unset.

## Release workflow (contributors)

```bash
make test                      # run tests
make release VERSION=v1.2.3    # tag + push → GitHub Actions builds linux/amd64 binary
make update-sha VERSION=v1.2.3 # fetch SHA256, patch docker-compose.yml
git add docker-compose.yml && git commit -m "chore: bump to v1.2.3"
make deploy                    # rsync + docker compose build + up -d on VPS
```

## API

All endpoints require `Authorization: Bearer <token>`. Admin endpoints require the master token.

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/health` | none | Liveness check |
| POST | `/api/users` | admin | Create user |
| GET | `/api/users` | admin | List users |
| POST | `/api/admin/invites` | admin | Create invite |
| GET | `/api/admin/invites` | admin | List invites |
| POST | `/api/invites/:token/claim` | none | Claim invite |
| POST | `/api/apps` | user | Create app |
| GET | `/api/apps` | user | List apps |
| GET | `/api/apps/:name` | user | Get app |
| DELETE | `/api/apps/:name` | user | Delete app |
| PUT | `/api/apps/:name/domain` | user | Set custom domain |
| DELETE | `/api/apps/:name/domain` | user | Clear custom domain |
| GET | `/api/apps/:name/env` | user | List env vars |
| POST | `/api/apps/:name/env` | user | Set env var |
| DELETE | `/api/apps/:name/env/:key` | user | Unset env var |
