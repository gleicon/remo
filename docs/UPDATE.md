# Non-destructive remo updates

## What persists between updates

Everything important lives on the host, mounted into containers:

| Host path | What's inside | Mount |
|---|---|---|
| `/etc/remo/` | `server.toml`, `master_token`, `authorized_keys` | rw in `remo` container, ro in `remo-sshd` |
| `/var/lib/remo/` | `state.db` (SQLite), `git/`, `apps/`, `deploys/` | rw in both |
| `remo_sshd_host_keys` (named volume) | SSH host key | rw in `remo-sshd` |

The Docker image is ephemeral. Rebuilding it changes nothing about live data.

## Standard update (code change only)

On your laptop:

```bash
git pull                    # fetch latest
cargo test                  # verify nothing broken
```

On the VPS:

```bash
ssh ubuntu@REDACTED_VPS_IP

cd /home/ubuntu/remo
git pull                    # pull the same changes
sudo docker compose build remo remo-sshd   # rebuild images
sudo docker compose up -d   # restart with new image (nano-rs unchanged)
```

Downtime: ~2–5 seconds while remo restarts. nano-rs keeps serving traffic during this window.

## Schema migrations

remo uses `CREATE TABLE IF NOT EXISTS` in `src/db.rs`. Adding a new table is automatically safe — it runs on startup and skips tables that already exist.

**Safe operations** (additive, no restart risk):
- Add a new table
- Add a nullable column (via a separate `ALTER TABLE ... ADD COLUMN`)
- Add an index

**Unsafe operations** (require care):
- Rename a column — write new column, migrate data, remove old
- Drop a column — back up DB first
- Change column type — SQLite requires rebuilding the table

For any non-additive migration, snapshot the DB first:
```bash
sudo cp /var/lib/remo/state.db /var/lib/remo/state.db.$(date +%Y%m%d)
```

## Rollback

```bash
cd /home/ubuntu/remo
git log --oneline -5        # find the previous commit
git checkout <sha>          # or: git stash + git pull for newer version
sudo docker compose build remo remo-sshd
sudo docker compose up -d
```

## What requires manual intervention

- Changes to `/etc/remo/server.toml`: edit on host, then `sudo docker compose restart remo`
- Changes to `/etc/remo/authorized_keys`: edit on host — sshd reads it on each connection, no restart needed
- Changes to nano-rs: rebuild `docker/nano-rs/` and `sudo docker compose build nano-rs && sudo docker compose up -d`
- Changes to `docker-compose.yml` port mappings or volume mounts: `sudo docker compose down && sudo docker compose up -d` (brief full-stack restart; running apps drop connections for ~5s)
