# remo

Single-VPS PaaS. `git push` deploys JS apps to a nano-rs V8 runtime. No Kubernetes, no Docker per app, no build step on the server.

```
remo push  →  git archive  →  extract  →  symlink swap  →  nano-rs reload
```

Apps live at `https://{owner}-{app}.{domain}`. One nano-rs process routes all apps by `Host` header.

---

## How it works

```
laptop
  remo push  (git push ssh://yourdomain.tld/myapp)
    │
    HOST sshd :22 → forced command: remo git-hook --user alice
      ├── verify: alice owns myapp?
      ├── git-receive-pack /var/lib/remo/git/myapp.git
      └── post-receive: extract → symlink → nano-rs reload

VPS
  nginx :443        →  nano-rs :8080  (app traffic, Host-header routing)
  yourdomain.tld    →  remo API :7070 (CLI traffic)
  nano-rs admin        :8889           (localhost only)
```

See [docs/SETUP_GUIDE.md](docs/SETUP_GUIDE.md) for the full install walkthrough.

---

## VPS install (Ubuntu 22.04+, Docker)

```bash
# 1. Dependencies
apt update && apt install -y docker.io docker-compose-v2 nginx certbot python3-certbot-nginx git
systemctl enable --now docker nginx

# 2. git user for SSH deploys
useradd --system --uid 2000 --shell /bin/sh --no-create-home git
usermod -p '*' git

# 3. Data directories
mkdir -p /var/lib/remo/{apps,git}
touch /var/lib/remo/authorized_keys
chown -R git:git /var/lib/remo
chmod 750 /var/lib/remo

# 4. sshd drop-in for git user
cat > /etc/ssh/sshd_config.d/50-remo-git.conf << 'EOF'
Match User git
    AuthorizedKeysFile /var/lib/remo/authorized_keys
    AllowTcpForwarding no
    X11Forwarding no
    PermitTTY no
EOF
systemctl reload ssh

# 5. Clone and run the installer
git clone https://github.com/gleicon/remo /home/ubuntu/remo
cd /home/ubuntu/remo
remo server install --docker --domain yourdomain.tld
# → saves master_token to /etc/remo/master_token (copy it now)

# 6. Start containers
docker compose up -d

# 7. Copy host binary (git-hook runs outside Docker)
docker compose cp remo:/usr/local/bin/remo /usr/local/bin/remo
remo --version

# 8. nginx + TLS — see docs/SETUP_GUIDE.md §3
```

> DNS: add `*.yourdomain.tld A <VPS_IP>` and `yourdomain.tld A <VPS_IP>` before step 8.

---

## Laptop setup

```bash
# Build from source (macOS or Linux)
git clone https://github.com/gleicon/remo && cd remo
make install   # cargo build --release + copy to /usr/local/bin/remo

# First-time login (needs master token from /etc/remo/master_token on VPS)
remo setup
# prompts: server URL · admin or user · token · username · SSH key
```

---

## Deploy your first app

```bash
remo apps create myapp
# → creates myapp/ with starter index.js, git repo, and remo remote

cd myapp
remo push
# → live at https://alice-myapp.yourdomain.tld
```

Starter `index.js` (ES module, same as Cloudflare Workers):

```javascript
export default {
  fetch(request) {
    const url = new URL(request.url);
    if (url.pathname === "/") {
      return new Response("hello from myapp", { status: 200 });
    }
    return new Response("Not Found", { status: 404 });
  },
};
```

The `addEventListener("fetch", ...)` Service Worker pattern is also supported.

---

## Day-to-day

```bash
remo apps list                        # list your apps
remo push                             # deploy (from inside the app dir)
remo logs                             # tail logs (infers app from git remote)
remo env set KEY=value                # set env var
remo env list                         # list env vars (values masked)
remo env unset KEY                    # remove env var
remo apps delete myapp                # delete app
```

Commands that take an app name infer it from the git remote when run inside the app directory.

---

## Invite a user

```bash
remo users invite alice
# prints a link and a one-liner — share either with Alice
```

Alice runs on her laptop:

```bash
remo setup --invite https://yourdomain.tld/invite/<token>
# or
remo setup --invite <token>
```

---

## API

`Authorization: Bearer <token>` on all endpoints. Admin endpoints need the master token.

| Method | Path | Auth |
|--------|------|------|
| GET | `/health` | — |
| POST | `/api/invites/{token}/claim` | — |
| POST | `/api/apps` | user |
| GET | `/api/apps` | user |
| GET | `/api/apps/{name}` | user |
| DELETE | `/api/apps/{name}` | user |
| POST | `/api/apps/{name}/rollback` | user |
| POST | `/api/apps/{name}/scale` | user |
| GET/POST | `/api/apps/{name}/env` | user |
| DELETE | `/api/apps/{name}/env/{key}` | user |
| PUT/DELETE | `/api/apps/{name}/domain` | user |
| GET | `/api/apps/{name}/logs` | user |
| GET/POST/DELETE | `/api/users[/{name}]` | admin |
| POST/GET | `/api/admin/invites` | admin |

---

## Releasing / contributing

```bash
make test                          # run tests
make build                         # cargo build --release
make release VERSION=v0.5.0        # bump Cargo.toml, commit, tag, push → CI builds linux/amd64
make deploy                        # rsync to VPS, rebuild container, update host binary
make logs                          # follow remo logs on VPS
make status                        # docker ps on VPS
```

Override VPS settings: `VPS_HOST`, `VPS_USER`, `VPS_SSH_KEY`, `VPS_DIR`.
