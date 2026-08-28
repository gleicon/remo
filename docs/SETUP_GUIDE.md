# remo Setup Guide

remo runs JavaScript apps in V8 isolates (via nano-rs) on a VPS you control. `git push` to deploy.

---

## How it works

```
git push remo main
  │
  SSH (host sshd, port 22) → forced command: remo git-hook --user <name>
    ├── verify ownership
    ├── git-receive-pack /var/lib/remo/git/<app>.git
    └── post-receive:
          ├── git archive → tar (10 MB cap)
          ├── extract → /var/lib/remo/apps/<app>/deploys/<sha>/
          ├── atomic symlink: current/ → deploys/<sha>/
          └── nano-rs admin API (localhost:8889): register/reload

nano-rs (Docker, port 8080)
  single process, routes by Host header → V8 isolate per app
  reads /var/lib/remo/apps/<app>/current/ (shared volume)

nginx (host, port 443)
  *.yourdomain.tld  → nano-rs :8080
  yourdomain.tld    → remo control plane :7070
```

---

## Prerequisites

- Ubuntu 22.04+ VPS (1 vCPU / 1 GB RAM minimum)
- Docker + Docker Compose v2
- nginx + certbot
- DNS records pointing to your VPS:

```
*.yourdomain.tld   A   <VPS_IP>
yourdomain.tld     A   <VPS_IP>
```

Install dependencies:

```bash
apt update && apt install -y docker.io docker-compose-v2 nginx certbot python3-certbot-nginx git
systemctl enable --now docker nginx
```

---

## 1. VPS — install remo (as root)

Download the binary and run the installer:

```bash
wget -qO /usr/local/bin/remo \
  https://github.com/gleicon/remo/releases/latest/download/remo-linux-amd64
chmod +x /usr/local/bin/remo

git clone https://github.com/gleicon/remo /home/ubuntu/remo
cd /home/ubuntu/remo

remo server install --docker --domain yourdomain.tld
```

`remo server install` creates:
- `/etc/remo/` (mode 0700) with `server.toml` and `master_token`
- `/var/lib/remo/{apps,git}/` with correct ownership
- `git` system user (shell `/bin/sh`, password disabled)
- `/etc/remo/authorized_keys`
- `.env` in the current directory with `NANO_ADMIN_API_KEY`

**Save the master token** printed at the end.

Configure sshd to use the remo authorized_keys for the git user. Create `/etc/ssh/sshd_config.d/50-remo-git.conf`:

```
Match User git
    AuthorizedKeysFile /var/lib/remo/authorized_keys
    AllowTcpForwarding no
    X11Forwarding no
    PermitTTY no
```

```bash
systemctl reload ssh
```

---

## 2. VPS — start the stack

```bash
cd /home/ubuntu/remo
docker compose up -d
```

Two containers:
- `remo-nano-rs-1` — JS runtime, data plane `:8080`, admin API `:8889`
- `remo-remo-1` — control plane `:7070`

Copy the remo binary from the container (needed for the git-hook forced command):

```bash
docker compose cp remo:/usr/local/bin/remo /usr/local/bin/remo
remo --version
```

---

## 3. VPS — nginx + TLS

`remo server install` writes the nginx configs automatically. Run certbot for TLS:

```bash
certbot --nginx -d yourdomain.tld -d "*.yourdomain.tld"
```

Wildcard certs require a DNS challenge — add the TXT record your DNS provider prompts for.

---

## 4. Laptop — first login

```bash
remo setup
```

Prompts:
1. **Server URL** — `https://yourdomain.tld`
2. **How are you setting up** — `1` (Admin, master token)
3. **Master token** — from `remo server install`
4. **Username** — your name, e.g. `alice`
5. **SSH key** — pick existing or generate new

`remo setup` writes `~/.remo/config.toml` and adds an `~/.ssh/config` entry for git push.

---

## 5. Deploy your first app

```bash
remo apps create myapp
cd myapp
remo push
```

Live at `https://alice-myapp.yourdomain.tld`.

App source is a JavaScript file with a `fetch` handler:

```javascript
export default {
  fetch(request) {
    return new Response("hello from myapp", { status: 200 });
  },
};
```

---

## 6. Invite another user

```bash
# On VPS or from admin laptop
remo users invite alice
```

Send Alice the printed token. She runs:

```bash
remo setup --invite <token>
```

This registers her SSH key and creates her account without needing the master token.

---

## Day-to-day operations

```bash
remo apps list
remo push                          # deploy from inside app directory
remo logs myapp
remo logs myapp --follow           # poll live runtime stats (request count, heap)
remo deployments myapp
remo rollback myapp
remo env set myapp KEY=value
remo env list myapp
remo env unset myapp KEY
remo scale myapp --workers 4
remo drain myapp
remo domain set myapp example.com  # custom domain
```

Custom domain provisioning (run as root on VPS, after DNS A record is set):

```bash
remo server apply-domain myapp
```

---

## App templates

```bash
remo apps create myapp --template js     # default: fetch handler
remo apps create myapp --template html   # returns styled HTML
remo apps create myapp --template kv     # uses nano:kv for persistence
remo apps create myapp --template spa    # localStorage shim over nano:kv
remo apps create myapp --template wasm   # wraps a wasm module
remo apps create myapp --template gas    # Google Apps Script handler (.gs)
```

Custom templates: save files to `~/.remo/templates/<name>/` and use `--template <name>`.

---

## SSH details

Each user's public key is stored in `/var/lib/remo/authorized_keys` with a forced-command prefix:

```
command="remo git-hook --user alice" ssh-ed25519 AAAA... alice@laptop
```

sshd reads this file on every connection — no reload needed after changes. To add a key manually:

```bash
remo server add-key --user alice --key "ssh-ed25519 AAAA..."
```

---

## nano-rs restart recovery

nano-rs keeps app registrations in memory. On container restart, remo re-registers all apps automatically within 30 seconds. For immediate recovery:

```bash
cd /path/to/myapp && git commit --allow-empty -m "re-register" && remo push
```

---

## Ports

| Port    | Service              | Access         |
|---------|----------------------|----------------|
| 22      | SSH (git push)       | public         |
| 80/443  | nginx                | public         |
| 8080    | nano-rs data plane   | localhost only |
| 8889    | nano-rs admin API    | localhost only |
| 7070    | remo control plane   | localhost only |

```bash
ufw allow 22 && ufw allow 80 && ufw allow 443 && ufw enable
```

---

## Troubleshooting

**"NANO Runtime" default response instead of app** — nano-rs lost the registration. Wait 30 seconds or push again.

**"Permission denied (publickey)"** — check `/var/lib/remo/authorized_keys` contains your key with the forced-command prefix. Check `~/.ssh/config` has the right `Host` entry.

**"app not found" or "user not authorized"** — app doesn't exist or you don't own it. Create it first: `remo apps create myapp`.

```bash
docker compose ps
docker compose logs nano-rs --tail=50
docker compose logs remo --tail=50
```
