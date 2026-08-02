# remo PLAN.md

## Now

**State:** Core control plane done. Builds clean. All API endpoints implemented. Hostname scheme: `{owner}-{name}.{domain}` with optional `custom_domain` CNAME. Env pipeline complete: base64 in DB, decoded on deploy, pushed to nano-rs via admin API. Token auth: SHA-256 O(1) lookup (bcrypt removed). Validation shared via `src/validation.rs`. 12 validation tests pass.

**Next:** Commit all changes, then extract remo into its own git repo via `git subtree split --prefix=remo -b remo-split` from the parent nano-rs repo.

**Open questions:**
- Proxy backends (nginx/Caddy) not yet wired — `ProxyBackend` trait exists in `src/proxy/` but neither `write_vhost` nor on-demand TLS is called during deploy. Needed before production.
- `deployment_create` / `deployment_update_status` in db.rs exist but are never called — deploy flow doesn't write deployment rows yet.
- Custom domain: proxy must serve both canonical hostname and `custom_domain` — not yet implemented in proxy layer.
- `remo server install` subcommand not yet implemented (CLI scaffolded, implementation missing).
- nano-rs admin socket path configured via `ServerConfig.nano_socket` — must match nano-rs deploy.

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
- [ ] Commit all changes in nano-rs parent repo
- [ ] `git subtree split --prefix=remo -b remo-split`
- [ ] Push remo-split to standalone repo (URL TBD)
- [ ] Update remote in SETUP_GUIDE.md

### Phase 6 — Production gaps (post-split)
- [ ] `remo server install` implementation
- [ ] Proxy config generation called on app create/delete/domain-change
- [ ] Write deployment rows on git-push deploy
- [ ] `nano.json` parsing in deploy hook (app_type, entrypoint override)
- [ ] Systemd unit file generation
- [ ] `remo logs` tail (stream nano-rs structured JSON filtered by hostname)
