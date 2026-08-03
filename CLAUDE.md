# remo — Claude context

remo is a single-VPS edge PaaS built on nano-rs. Git-push deploy, no Kubernetes, no Docker.

## What it does

`git push remo main` → post-receive hook → git archive → tar extract → atomic symlink swap → nano-rs admin API reload.

Apps live at `{owner}-{name}.{domain}` (e.g. `alice-myapp.apps.yourdomain.tld`). Optional custom domain stored as `custom_domain` on the app row — proxy must serve both.

## Architecture

```
laptop
  git push
    │
    SSH (forced command) → remo git-hook --user alice
      ├─ auth: user owns app?
      ├─ exec git-receive-pack <bare-repo>
      └─ post-receive: remo git-hook --deploy myapp --sha <sha>
            ├─ git archive → tar
            ├─ extract to deploys/<sha>/
            ├─ symlink swap: current/ → deploys/<sha>/
            └─ nano admin API: create/update + reload

VPS
  nginx/Caddy :443  →  nano-rs :8080  (Host-header routing)
  remo control :7070   (localhost only)
  nano-rs admin :9000  (localhost only)
```

## Key files

| File | Role |
|------|------|
| `src/db.rs` | SQLite schema + all queries; `App`, `User`, `Deployment` types |
| `src/config.rs` | `ServerConfig` (TOML at `/etc/remo/server.toml`) |
| `src/server/api.rs` | All HTTP handlers; `AppResponse`; `validate_app_name`, `validate_domain`, `safe_join` |
| `src/server/auth.rs` | Bearer token middleware; `sha256_hex`; master token + user token lookup |
| `src/server/mod.rs` | axum router, `AppState`, `user_routes()`, `admin_routes()` |
| `src/nano_client.rs` | HTTP client to nano-rs admin API; `is_not_found()` |
| `src/deploy/mod.rs` | `DeployContext`, `run()`, prune logic |
| `src/cli/git_hook.rs` | SSH forced-command handler + post-receive deploy |
| `src/validation.rs` | `is_valid_app_name()`, `is_valid_entrypoint()` — shared validators |
| `src/proxy/` | `ProxyBackend` trait + nginx/Caddy stubs (not yet wired) |
| `docs/DESIGN.md` | Architecture, API surface, extension points |
| `.project/PLAN.md` | Task roadmap with phase status |

## Token auth

SHA-256 only (no bcrypt). `sha256_hex(raw_token)` stored in `users.token_hash`. Lookup is O(1) by hash. Master token (admin) is constant-time compared in `auth.rs`. `sha256_hex` is `pub` — call it from api.rs for user creation.

## Master token file

`/etc/remo/master_token` is written atomically with `OpenOptions::mode(0o600)` — the permission is set at `open()` time, so there is never a window where the file exists but is world-readable. Do not simplify this to `fs::write` + `chmod` — that two-step has a TOCTOU race.

`/etc/remo` itself is `0o700` (owner-execute only). This means even before per-file permissions are set, no other account can stat or open files inside the directory. On `--reinit`, `set_permissions` is called unconditionally after `DirBuilder::create` to enforce the mode even if the directory already existed with looser permissions.

## Env pipeline

Vars stored base64-encoded in `env_vars` table. `env_list_decoded()` decodes on read. On every deploy, `run_deploy` in git_hook.rs loads decoded env and passes it in `DeployContext.env_vars` → `reload_nano` sends it to nano-rs. On `env_unset`, remaining decoded vars are pushed immediately to drain the deleted key from the running worker.

## Hostname scheme

`{owner}-{name}.{domain}` — enforced at app create. Prevents two users from squatting the same subdomain. Custom CNAME in `custom_domain` column. `PUT /api/apps/:name/domain` sets it; `DELETE` clears. Proxy layer must serve both (not yet implemented).

## Deploy limits

`MAX_DEPLOY_BYTES` in `src/deploy/mod.rs` caps the tar archive at 10 MB. Checked before SHA hashing or disk I/O — user gets a clear error at `git push`. Recompile to change. nano-rs should carry its own higher ceiling (defense-in-depth) as a separate compile-time constant; remo is the policy layer.

## What's not done yet (production gaps)

- Proxy config generation (nginx vhost / Caddy config written on app create/delete/domain change)
- Deployment rows written to DB on git-push
- `nano.json` parsed in deploy hook for app_type/entrypoint override
- Systemd unit generation
- Log streaming (`remo logs` tails nano-rs JSON filtered by hostname)

## Dev workflow

```bash
# build
cargo build

# run tests
cargo test

# run server (needs nano-rs running + /etc/remo/server.toml)
cargo run -- server start
```

Config at `/etc/remo/server.toml`:
```toml
domain = "apps.yourdomain.tld"
data_dir = "/var/lib/remo"
nano_socket = "http://127.0.0.1:9000"
bind = "127.0.0.1:7070"
```

