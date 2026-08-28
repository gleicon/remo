# Updating remo

## What persists between updates

Everything important lives on the host, mounted into containers:

| Host path | What's inside |
|---|---|
| `/etc/remo/` | `server.toml`, `master_token`, `authorized_keys` |
| `/var/lib/remo/` | `state.db` (SQLite), `git/`, `apps/`, `deploys/` |
| `remo_sshd_host_keys` (named Docker volume) | SSH host key |

The Docker image is ephemeral — rebuilding it changes nothing about live data.

## Standard update workflow

### 1. Release a new version

```bash
make release VERSION=v0.5.8
```

This bumps `Cargo.toml`, commits, tags, and pushes — CI builds a static `remo-linux-amd64` binary (~2 min).

Monitor: https://github.com/gleicon/remo/actions

### 2. Deploy to VPS

```bash
make deploy
```

Rsyncs source to VPS, rebuilds the remo container from source, and copies the binary to `/usr/local/bin/remo` on the host (needed for the git-hook forced command).

nano-rs is not restarted and keeps serving traffic during the remo restart (~2–5 s downtime).

### When you rebuild nano-rs

nano-rs keeps app registrations in memory. Rebuilding its container clears them. remo re-registers all apps automatically within 30 seconds. If you need immediate recovery:

```bash
cd /path/to/myapp && git commit --allow-empty -m "re-register" && remo push
```

## Schema migrations

remo uses `CREATE TABLE IF NOT EXISTS` in `src/db.rs`. New tables are added automatically on startup with no manual step.

**Safe (additive):**
- Add a new table
- Add a nullable column (`ALTER TABLE ... ADD COLUMN`)
- Add an index

**Requires care:**
- Rename or drop a column — snapshot the DB first:
  ```bash
  sudo cp /var/lib/remo/state.db /var/lib/remo/state.db.$(date +%Y%m%d)
  ```

## Rollback

```bash
# On VPS
cd /home/ubuntu/remo
git log --oneline -5
git checkout <previous-sha>
make deploy
```

## Other operational tasks

| Task | Command |
|---|---|
| Tail remo logs | `make logs` |
| Check container status | `make status` |
| Check HTTPS health | `make health` |
| Reload server config | `ssh vps "sudo docker compose restart remo"` |
| Rebuild nano-rs | `ssh vps "cd /home/ubuntu/remo && sudo docker compose build nano-rs && sudo docker compose up -d"` |

Changes to `/etc/remo/authorized_keys` take effect immediately — sshd reads the file on each connection, no restart needed.

Changes to `/etc/remo/server.toml` require `docker compose restart remo`.

Port mapping or volume mount changes in `docker-compose.yml` require a full `docker compose down && up -d` (brief full-stack restart; running apps drop for ~5 s).
