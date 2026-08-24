# remo PLAN.md

## Now

**State:** remo v0.4.9, standalone repo at gleicon/remo. Control plane live on VPS. nano-rs v2.7.0 (binary-download Dockerfile, ~5s build). Template system: `--template NAME` with user dir `~/.remo/templates/<name>/`. Deploy durability: `reload_nano` failure is non-fatal; sync loop re-registers on recovery.

**State:** All phases complete. No unwired code. No silent stubs. See "Future features" section below for decided vs. undecided work.

---

## Roadmap

### Phase 1 — Scaffold + grill
- [x] Design session (`/ds-grill-me`) — full decision log in `GRILL.md`
- [x] `remo/` directory created inside nano-rs, own Cargo workspace
- [x] Architecture documented in `docs/DESIGN.md`
- [x] Setup guide in `docs/SETUP_GUIDE.md`

### Phase 2 — Core control plane
- [x] `src/db.rs` — SQLite schema: apps, users, deployments, env_vars, nodes
- [x] `src/config.rs` — ServerConfig (TOML at `/etc/remo/server.toml`)
- [x] `src/server/auth.rs` — bearer token middleware, SHA-256 O(1) lookup, master token
- [x] `src/server/api.rs` — all CRUD endpoints, error types, ownership check
- [x] `src/server/mod.rs` — axum router, AppState, empty master_token warning
- [x] `src/nano_client.rs` — HTTP client to nano-rs admin API (create/update/delete/reload/scale/drain/set_env)
- [x] `src/deploy/mod.rs` — DeployContext, run(), git archive → tar extract → symlink swap → nano reload
- [x] `src/cli/` — apps, users, env, deploy, logs, login, server, git_hook, agent subcommands
- [x] `src/proxy/` — ProxyBackend trait, Caddy + nginx stubs (not yet called)
- [x] `src/validation.rs` — `is_valid_app_name()` shared by api.rs and git_hook.rs

### Phase 3 — Nano.env wiring (in nano-rs)
- [x] `CURRENT_ENV` thread-local in `src/runtime/vfs_bindings.rs`
- [x] `set_current_env()` called before `bind_all` in handler.rs and context.rs
- [x] `WorkerPool::with_source_backend_and_env()` constructor
- [x] 6 V8 integration tests: `tests/nano_env_test.rs`

### Phase 4 — Hostname scheme + CNAME
- [x] Hostname: `{owner}-{name}.{domain}` — prevents subdomain squatting
- [x] `custom_domain TEXT` column in apps schema
- [x] `app_set_custom_domain()` in db.rs
- [x] `AppResponse.custom_domain` field
- [x] `PUT /api/apps/:name/domain` + `DELETE /api/apps/:name/domain` with `validate_domain()`
- [ ] Proxy serves both canonical + custom domain (nginx/Caddy config generation)

### Phase 5 — Git repo extraction
- [x] Commit all changes in nano-rs parent repo
- [x] Push remo as standalone repo: github.com/gleicon/remo
- [x] SETUP_GUIDE.md updated with correct repo URL

### Phase 6 — Production gaps (complete)
- [x] `remo server install` implementation
- [x] Proxy wired for custom domain TLS (NoopProxy in Docker mode; NginxBackend on bare VPS)
- [x] app_delete deregisters from nano-rs + cleans up custom domain cert
- [x] Deployment rows written on git-push (pending → success/failed + sha)
- [x] `remo logs` and `remo deployments` return real data with sha and timestamps
- [x] Rollback fixed to compare deployment sha (not UUID) against current_sha
- [x] `remo drain myapp` wired (`POST /api/apps/:name/drain` → nano-rs graceful drain)
- [x] Dead code removed: agent.rs, nodes table, CaddyBackend.domain field

---

## Future features (discuss before implementing)

These are real ideas, not deferred work. Nothing is half-built waiting for them.

| Feature | What it needs | Notes |
|---------|--------------|-------|
| **Log streaming** (`remo logs --follow`) | nano-rs to expose SSE or WebSocket log endpoint per app | nano-rs v2.7.0 has no streaming log API; needs design in nano-rs first |
| **`nano.json` override at push time** | git hook reads `nano.json` from archive before calling deploy | Changes entrypoint/app_type per-push without re-creating the app; low priority |
| **Multi-node / horizontal scale** | Control plane distributes deploys to worker nodes; agent receives them | Big design — node discovery, artifact transfer, consensus on current sha |
| **Systemd unit generation** | `remo server install` writes a systemd unit for bare-VPS mode | Moot for Docker deployments; only matters if someone runs remo without Docker |
| **Custom domain TLS in Docker mode** | Host-side certbot called from within the container, or a side-channel mechanism | Currently logs a warning; user runs certbot manually |
