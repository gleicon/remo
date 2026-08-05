# remo Setup Guide

remo is a single-VPS PaaS. One `git push` deploys a JS app to a nano-rs V8 runtime. No Kubernetes, no Docker per app, no build step on the server.

---

## How it works

```
laptop
  git push ssh://cloud.yourdomain.tld/myapp
    │
    HOST sshd (port 22) → forced command: remo git-hook --user alice
      ├── verify: alice owns myapp?
      ├── git-receive-pack /var/lib/remo/git/myapp.git
      └── post-receive:
            ├── git archive → tar (10 MB cap)
            ├── extract → /var/lib/remo/apps/myapp/deploys/<sha>/
            ├── atomic symlink: current/ → deploys/<sha>/
            └── nano-rs admin API (localhost:8889): register/reload app

nano-rs (Docker container, port 8080)
  ├── single process, all apps
  ├── routes by Host header → entrypoint file
  └── reads /var/lib/remo/apps/<app>/current/ (volume mount, read-only)

nginx (host, port 443)
  ├── *.yourdomain.tld  → nano-rs :8080  (app traffic)
  └── yourdomain.tld   → remo API :7070  (CLI traffic)
```

Key point: **nano-rs is one process for all apps**. A single V8 runtime reads each app's entrypoint from disk and routes requests by hostname. There is no per-app container.

---

## Prerequisites

- Ubuntu 22.04+ VPS (1 vCPU / 1 GB RAM minimum)
- Docker + Docker Compose (v2)
- nginx + certbot
- DNS: wildcard A record and control-plane A record pointing to your VPS IP

```
*.yourdomain.tld    A   <VPS_IP>
yourdomain.tld      A   <VPS_IP>
```

The wildcard covers `alice-myapp.yourdomain.tld`. The bare domain covers the control plane and git SSH endpoint.

---

## 1. VPS — one-time host setup (as root)

### 1a. Install dependencies

```bash
apt update && apt install -y docker.io docker-compose-v2 nginx certbot python3-certbot-nginx git
systemctl enable --now docker nginx
```

### 1b. Create the git system user

The `git` user receives SSH connections for `git push`. Its shell must be `/bin/sh` (not `/usr/sbin/nologin`) so the forced command runs. Password disabled (not locked — OpenSSH distinguishes these).

```bash
useradd --system --uid 2000 --shell /bin/sh --no-create-home git
usermod -p '*' git
```

### 1c. Create data directories

```bash
mkdir -p /var/lib/remo/{apps,git}
touch /var/lib/remo/authorized_keys
chown -R git:git /var/lib/remo
chmod 750 /var/lib/remo
```

### 1d. Configure host sshd for git pushes

Create `/etc/ssh/sshd_config.d/50-remo-git.conf`:

```
# remo: git push uses forced command, no interactive shell.
Match User git
    AuthorizedKeysFile /var/lib/remo/authorized_keys
    AllowTcpForwarding no
    X11Forwarding no
    PermitTTY no
```

Reload sshd:

```bash
systemctl reload ssh
```

### 1e. Install the remo binary on the host

The git-hook runs as a host process (not inside Docker). The binary is built from source inside the Docker image and then copied out:

```bash
# After the Docker stack is running (step 2 below):
docker compose cp remo:/usr/local/bin/remo /usr/local/bin/remo
remo --version
```

Alternatively, if you have Rust on the host:

```bash
git clone https://github.com/gleicon/remo /home/ubuntu/remo
cd /home/ubuntu/remo && cargo build --release
cp target/release/remo /usr/local/bin/remo
```

### 1f. Clone remo and run the installer

```bash
cd /home/ubuntu
git clone https://github.com/gleicon/remo
cd remo

remo server install --docker --domain yourdomain.tld
```

This creates:
- `/etc/remo/` (mode 0700) with `server.toml` and `master_token`
- `.env` in the current directory with `NANO_ADMIN_API_KEY=<random>`

**Save the master token** printed at the end — you need it once for `remo setup` on your laptop.

---

## 2. VPS — start the Docker stack

```bash
cd /home/ubuntu/remo
docker compose up -d
```

Two containers start:
- `remo-nano-rs-1` — JS runtime, data plane on `127.0.0.1:8080`, admin API on `127.0.0.1:8889`
- `remo-remo-1` — control plane HTTP API on `127.0.0.1:7070`

Verify they're healthy:

```bash
docker compose ps
```

Both should show `Up (healthy)` or `Up`.

---

## 3. VPS — nginx + TLS

### 3a. Create nginx config

Create `/etc/nginx/sites-available/remo`:

```nginx
# App traffic: *.yourdomain.tld → nano-rs
server {
    listen 80;
    server_name *.yourdomain.tld;
    location / {
        proxy_pass         http://127.0.0.1:8080;
        proxy_set_header   Host              $host;
        proxy_set_header   X-Real-IP         $remote_addr;
        proxy_set_header   X-Forwarded-For   $proxy_add_x_forwarded_for;
        proxy_set_header   X-Forwarded-Proto $scheme;
    }
}

# Control plane: yourdomain.tld → remo API
server {
    listen 80;
    server_name yourdomain.tld;
    location / {
        proxy_pass         http://127.0.0.1:7070;
        proxy_set_header   Host              $host;
        proxy_set_header   X-Real-IP         $remote_addr;
    }
}
```

```bash
ln -s /etc/nginx/sites-available/remo /etc/nginx/sites-enabled/remo
nginx -t && systemctl reload nginx
```

### 3b. Get TLS certificates

```bash
certbot --nginx -d yourdomain.tld -d "*.yourdomain.tld"
```

> **Note:** Wildcard certs (`*.yourdomain.tld`) require a DNS challenge. certbot will prompt for a TXT record — add it to your DNS provider, then press Enter.

---

## 4. Laptop — first login (admin)

Build the remo CLI (or download the same release binary):

```bash
git clone https://github.com/gleicon/remo
cd remo && cargo build --release
sudo cp target/release/remo /usr/local/bin/remo
```

Run interactive setup:

```bash
remo setup
```

Prompts:
1. **Server URL** — `https://yourdomain.tld`
2. **Setup as** — `1` (Admin — master token)
3. **Master token** — the one printed by `remo server install`
4. **Username** — your name (e.g. `alice`)
5. **SSH key** — generate a new key or enter path to existing `~/.ssh/id_ed25519.pub`

`remo setup` creates:
- `~/.remo/config.toml` — server URL + scoped user token
- Registers your SSH key server-side (written to `/var/lib/remo/authorized_keys`)
- Adds a `Host yourdomain.tld` block to `~/.ssh/config` so git push works

After setup, the master token is no longer needed day-to-day. Your user token in `~/.remo/config.toml` is scoped to your account.

---

## 5. Deploy your first app

`remo apps create` scaffolds everything — creates the directory, writes a starter `index.js`, initialises a git repo, and adds the remo remote:

```bash
remo apps create myapp
cd myapp
remo push
```

Live at `https://alice-myapp.yourdomain.tld`.

### Writing app code

Apps use the ES module pattern (same as Cloudflare Workers):

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

The Service Worker `addEventListener` pattern is also supported:

```javascript
addEventListener("fetch", (event) => {
  event.respondWith(new Response("hello", { status: 200 }));
});
```

Both patterns implement the WinterTC fetch interface. `request` is a standard [Request](https://developer.mozilla.org/en-US/docs/Web/API/Request) object; return a [Response](https://developer.mozilla.org/en-US/docs/Web/API/Response).

---

## 6. Invite another user (Alice)

```bash
remo users invite alice
```

Prints a shareable link and a CLI command (valid 1 hour). Send either to Alice:

```
Link:    https://yourdomain.tld/invite/<token>
Command: remo setup --invite <token>
```

Alice runs on her laptop (both forms accepted):

```bash
remo setup --invite https://yourdomain.tld/invite/<token>
```

This registers her SSH key and creates her account. No manual authorized_keys editing.

---

## 7. Day-to-day operations

```bash
# List your apps
remo apps list

# Deploy (run from inside the app directory)
remo push

# Environment variables (app name inferred from git remote when run inside the app dir)
remo env set DATABASE_URL=postgres://...
remo env list
remo env unset DATABASE_URL

# Or specify the app name explicitly
remo env set myapp DATABASE_URL=postgres://...

# Logs (also infers app name from git remote)
remo logs --lines 200
remo logs myapp --lines 200

# Deployment history
remo deployments myapp
```

---

## 8. SSH details

SSH for git push uses the **host sshd** (port 22), not a Docker container.

Each user's public key is stored in `/var/lib/remo/authorized_keys` with a forced command prefix:

```
command="remo git-hook --user alice" ssh-ed25519 AAAA... alice@laptop
```

When Alice does `git push`, sshd runs `remo git-hook --user alice` instead of giving a shell. The forced command authenticates ownership, runs `git-receive-pack`, and deploys on post-receive.

The git remote URL is `ssh://yourdomain.tld/myapp` (no explicit user — `~/.ssh/config` sets `User git`). The server-side path is just the app name; remo resolves it to the bare repo at `/var/lib/remo/git/myapp.git`.

To add a key manually (e.g. for CI):

```bash
remo server add-key --user alice --key "ssh-ed25519 AAAA..."
```

This appends the forced-command line to `/var/lib/remo/authorized_keys`. sshd reads the file on every connection — no reload needed.

---

## 9. nano-rs restart and app registration

nano-rs keeps app registrations in memory. If the container restarts (upgrade, crash), registrations are lost. **remo detects this automatically**: on startup and every 30 seconds it checks nano-rs health, and re-registers all apps if nano-rs came back up.

Manual re-register (if you need immediate recovery without waiting):

```bash
cd /path/to/myapp && git commit --allow-empty -m "re-register" && remo push
```

---

## Ports and firewall

| Port | Service | Access |
|------|---------|--------|
| 22 | SSH (git push) | public |
| 80/443 | nginx | public |
| 8080 | nano-rs data plane | localhost only |
| 8889 | nano-rs admin API | localhost only |
| 7070 | remo control plane | localhost only |

Firewall: only 22, 80, and 443 should be reachable from outside.

```bash
ufw allow 22 && ufw allow 80 && ufw allow 443 && ufw enable
```

---

## Troubleshooting

### "NANO Runtime" instead of app response

nano-rs lost the app registration (container restart). remo re-registers automatically within 30 seconds. If you need it immediately:

```bash
cd /path/to/myapp && git commit --allow-empty -m "re-register" && remo push
```

### git push: "Permission denied (publickey)"

Check `/var/lib/remo/authorized_keys` contains your key with the correct forced-command prefix. Verify `~/.ssh/config` has the right identity:

```
Host yourdomain.tld
    User git
    IdentityFile ~/.ssh/id_ed25519_remo
    IdentitiesOnly yes
```

### git push: "Error: app not found" or "user not authorized"

Your SSH key is recognized but you don't own that app. Create it first:

```bash
remo apps create myapp
```

### Check container health

```bash
# On VPS
docker compose ps
docker compose logs nano-rs --tail=50
docker compose logs remo --tail=50
```
