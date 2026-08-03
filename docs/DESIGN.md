# remo Architecture

## Data flow

```
git push remo main
  │
  SSH (forced command: remo git-hook --user <name>)
    ├── auth: does this user own the app?
    ├── exec git-receive-pack <bare-repo at /var/lib/remo/git/<app>.git>
    └── post-receive trigger
          ├── git archive → tar bytes
          ├── size check (MAX_DEPLOY_BYTES = 10 MB)
          ├── content-addressed extract → /var/lib/remo/apps/<app>/deploys/<sha>/
          ├── atomic symlink: current/ → deploys/<sha>/
          └── nano-rs admin API: update (or create on first deploy) + reload

HTTP request
  │
  nginx/Caddy :443 (Host header routing)
    └── nano-rs :8080 (data plane, routes by hostname)

remo CLI
  └── HTTP → remo control plane :7070 (Bearer token auth)
        └── SQLite /var/lib/remo/state.db
```

## Ports

| Port   | Service               | Exposure        |
|--------|-----------------------|-----------------|
| 80/443 | nginx / Caddy         | public          |
| 8080   | nano-rs data plane    | localhost only  |
| 9000   | nano-rs admin API     | localhost only  |
| 7070   | remo control plane    | localhost only  |

## Directory layout

```
/var/lib/remo/
├── state.db
├── git/
│   └── myapp.git/          (bare repo, managed by remo)
└── apps/
    └── myapp/
        ├── deploys/
        │   ├── abc123ef/   (extracted tar, content-addressed SHA prefix)
        │   └── def456ab/
        └── current -> deploys/abc123ef/   (atomic symlink)

/etc/remo/
├── server.toml
├── master_token            (0o600, written atomically — never chmod after write)
└── authorized_keys         (SSH forced-command entries)
```

## Hostname scheme

Canonical: `{owner}-{name}.{domain}` (e.g. `alice-myapp.apps.yourdomain.tld`)

Owner prefix is enforced at app create — two users can each have an app named `api` without conflict. Optional custom CNAME stored in `custom_domain` column; proxy must serve both. Set via `PUT /api/apps/:name/domain`.

## Control Plane API

All endpoints require `Authorization: Bearer <token>`. Admin endpoints require the master token or an `is_admin` user.

```
GET    /health                            public
GET    /api/apps                          list apps (own only, or all if admin)
POST   /api/apps                          create app  {name, type, entrypoint}
GET    /api/apps/:name                    app detail
DELETE /api/apps/:name                    delete app
GET    /api/apps/:name/deployments        deployment history
POST   /api/apps/:name/rollback           roll back  {sha?}
POST   /api/apps/:name/scale              set workers  {workers}
GET    /api/apps/:name/env                list env keys (values masked)
POST   /api/apps/:name/env                set env vars  {vars: {k: v}}
DELETE /api/apps/:name/env/:key           unset env var
PUT    /api/apps/:name/domain             set custom domain  {domain}
DELETE /api/apps/:name/domain             clear custom domain
GET    /api/apps/:name/logs               deployment log output
GET    /api/users                         (admin)
POST   /api/users                         (admin)  {name, ssh_pubkey?}
DELETE /api/users/:name                   (admin)
```

`AppResponse` shape:

```json
{
  "name":         "myapp",
  "hostname":     "alice-myapp.apps.yourdomain.tld",
  "owner":        "alice",
  "app_type":     "js",
  "entrypoint":   "index.js",
  "current_sha":  "abc123ef",
  "custom_domain": null,
  "created_at":   "2026-08-01T00:00:00Z"
}
```

## Extending remo

### Add a new proxy backend

1. Add a variant to `ProxyBackend` enum in `src/config.rs`.
2. Create `src/proxy/<name>.rs` implementing the `ProxyBackend` trait (`src/proxy/mod.rs`).
3. Wire the variant in `src/cli/server.rs` → `write_proxy_config()`.
4. Call `write_vhost()` / `delete_vhost()` from `apps_create` / `apps_delete` in `src/server/api.rs` (not yet wired for any backend).

### Add a new CLI subcommand

1. Add a file `src/cli/<name>.rs` with a `pub async fn run(...)`.
2. Add a variant to `Commands` in `src/main.rs` and dispatch it.
3. For server-side operations, add an HTTP handler in `src/server/api.rs` and register the route in `src/server/mod.rs`.

### Add a new nano-rs admin API call

Add a method to `NanoClient` in `src/nano_client.rs`. The client talks to `http://127.0.0.1:9000` (configurable via `server.toml`).

### Change deploy behavior

`src/deploy/mod.rs` → `run()` is the entry point. Called from `src/cli/git_hook.rs` after `git archive` completes. `DeployContext` carries app metadata and env vars. `MAX_DEPLOY_BYTES` (compile-time) is the size gate before any disk I/O.

### Add per-app resource limits

`CreateAppRequest` in `src/nano_client.rs` has `cpu_time_ms` and `memory_mb` fields already sent to nano-rs. Store limits in a new DB column, populate from `nano.json` (parsed in `git_hook.rs` — not yet implemented), and pass through `DeployContext`.
