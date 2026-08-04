# remo

Single-VPS PaaS. `git push` deploys JS/WASM apps to a nano-rs V8 runtime. No Kubernetes, no Docker per app, no build step on the server.

```
git push remo main  →  extract  →  symlink swap  →  nano-rs reload
```

Apps live at `{owner}-{app}.{domain}`. One nano-rs process routes all apps by `Host` header.

---

## Architecture

```
laptop: git push ssh://cloud.yourdomain.tld/myapp
  │
  HOST sshd :22 → forced command → remo git-hook --user alice
    ├── verify ownership
    ├── git-receive-pack /var/lib/remo/git/myapp.git
    └── post-receive: extract → symlink → nano-rs admin API

HOST: nano-rs (Docker) :8080   all apps, one process
HOST: remo API (Docker) :7070  control plane
HOST: nginx :443               *.domain → :8080, domain → :7070
```

See [docs/SETUP_GUIDE.md](docs/SETUP_GUIDE.md) for the full install walkthrough.

---

## Quick install (Docker, Ubuntu 22.04+)

```bash
# 1. Clone and install host binary
git clone https://github.com/gleicon/remo && cd remo
wget -qO /usr/local/bin/remo \
  https://github.com/gleicon/remo/releases/download/v0.2.5/remo-linux-amd64
chmod +x /usr/local/bin/remo

# 2. One-time host prep
useradd --system --uid 2000 --shell /bin/sh --no-create-home git && usermod -p '*' git
mkdir -p /var/lib/remo/{apps,git} && touch /var/lib/remo/authorized_keys
chown -R git:git /var/lib/remo

# 3. Run installer (generates master_token + .env)
remo server install --docker --domain yourdomain.tld

# 4. Configure sshd for git user (see SETUP_GUIDE.md §1d)
# 5. Start Docker stack
docker compose up -d

# 6. Set up nginx + TLS (see SETUP_GUIDE.md §3)
```

---

## Admin first login

On your laptop:

```bash
remo setup
# prompts: server URL, master token (from /etc/remo/master_token), username, SSH key
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
# registers SSH key, creates account, configures ~/.remo/config.toml
```

---

## Deploy an app

```javascript
// index.js — WinterTC Service Worker pattern
addEventListener("fetch", (event) =>
  event.respondWith(new Response("hello", { status: 200 }))
);
```

```bash
remo apps create myapp
git init && git add index.js && git commit -m "init"
git remote add remo ssh://yourdomain.tld/myapp
git push remo main
# live at https://alice-myapp.yourdomain.tld
```

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
