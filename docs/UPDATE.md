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

### 1. Commit and tag a new version

```bash
git add -p
git commit -m "feat: ..."
git push

make release VERSION=v0.2.1
```

This pushes the tag. GitHub Actions (`.github/workflows/release.yml`) picks it up,
builds `remo-linux-amd64` for linux/amd64, and publishes it as a release asset
along with a `remo-linux-amd64.sha256` file.

Monitor: https://github.com/gleicon/remo/actions (~2 minutes)

### 2. Update the pinned version in docker-compose.yml

Once the CI release is done:

```bash
make update-sha VERSION=v0.2.1
# patches REMO_VERSION and REMO_SHA256 in docker-compose.yml

git add docker-compose.yml
git commit -m "chore: bump remo to v0.2.1"
git push
```

### 3. Deploy to VPS

```bash
make deploy
```

This rsyncs the repo to the VPS and runs:
```bash
docker compose build --pull remo remo-sshd   # downloads new binary (~10 s)
docker compose up -d                          # restarts remo + remo-sshd
```

nano-rs is not restarted and keeps serving traffic during the remo restart (~2–5 s downtime).

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

Or revert `docker-compose.yml` to the previous `REMO_VERSION` and redeploy.

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
