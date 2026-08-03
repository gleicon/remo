# remo

Single-VPS PaaS. `git push` deploys JS/WASM apps to a nano-rs V8 runtime. No Kubernetes, no Docker per app, no build step on the server.

```
git push remo main  →  extract  →  symlink swap  →  nano-rs reload
```

Apps live at `{owner}-{app}.{domain}`. Optional custom domain via `PUT /api/apps/:name/domain`.

---

## Server setup (ops)

**Requirements:** Linux VPS, Docker, nginx/Caddy with TLS, ports 443 and 2222 open.

```bash
git clone https://github.com/gleicon/remo
cd remo

sudo mkdir -p /etc/remo /var/lib/remo
sudo tee /etc/remo/server.toml <<EOF
domain      = "apps.example.com"
data_dir    = "/var/lib/remo"
nano_socket = "http://127.0.0.1:8889"
bind        = "0.0.0.0:7070"
EOF

sudo remo server init    # writes /etc/remo/master_token
docker compose up -d
```

nginx routes: `remo.example.com → :7070`, `*.apps.example.com → :8080`.

---

## Admin first login

Install the CLI on your laptop:

```bash
cargo install --git https://github.com/gleicon/remo
# or download from https://github.com/gleicon/remo/releases
```

```bash
remo setup
# prompts: server URL, master token (from /etc/remo/master_token), username, SSH key
# writes ~/.config/remo/config.toml and ~/.ssh/config entry
```

---

## Invite a user

```bash
remo users invite alice --email alice@example.com
# prints a single-use claim command valid for 1 hour
```

Alice runs on her laptop:

```bash
remo setup --invite <token>
# picks or generates SSH key, claims account, writes config
```

---

## Deploy an app

```bash
remo apps create myapp

git remote add remo git@remo.example.com:myapp
git push remo main
```

App live at `alice-myapp.apps.example.com`.

```bash
remo apps list
remo env set myapp KEY=value
remo env list myapp
remo apps delete myapp
```

---

## API

All endpoints require `Authorization: Bearer <token>`. Admin endpoints require the master token.

| Method | Path | Auth | |
|--------|------|------|---|
| GET | `/health` | — | liveness |
| POST | `/api/users` | admin | create user |
| GET | `/api/users` | admin | list users |
| DELETE | `/api/users/:name` | admin | delete user |
| POST | `/api/admin/invites` | admin | create invite |
| GET | `/api/admin/invites` | admin | list invites |
| POST | `/api/invites/:token/claim` | — | claim invite |
| POST | `/api/apps` | user | create app |
| GET | `/api/apps` | user | list apps |
| GET | `/api/apps/:name` | user | get app |
| DELETE | `/api/apps/:name` | user | delete app |
| PUT | `/api/apps/:name/domain` | user | set custom domain |
| DELETE | `/api/apps/:name/domain` | user | clear custom domain |
| GET | `/api/apps/:name/env` | user | list env vars |
| POST | `/api/apps/:name/env` | user | set env var |
| DELETE | `/api/apps/:name/env/:key` | user | unset env var |

---

## Contributing / releasing

```bash
make test
make tag VERSION=v1.2.3        # create local tag
make tag-push VERSION=v1.2.3   # push → CI builds linux/amd64 binary (~2 min)
make bump VERSION=v1.2.3       # update docker-compose.yml with new SHA256
git add docker-compose.yml && git commit -m "chore: bump to v1.2.3" && git push
make deploy                    # rsync + rebuild containers on VPS
```
