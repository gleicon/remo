# remo — project map

## Overview

remo is a single-VPS edge PaaS built on top of nano-rs (a V8-based JavaScript runtime). It provides git-push deploys without Kubernetes or Docker: `git push remo main` triggers a post-receive hook that archives the repo, extracts it, swaps a symlink, and reloads the app in nano-rs via its admin API. Apps are addressed as `{owner}-{name}.{domain}` subdomains with optional custom CNAME.

## Stack

- **Language**: Rust 2021 edition
- **Async runtime**: Tokio (full features)
- **HTTP server**: axum 0.8
- **Database**: SQLite via sqlx 0.8 (runtime-tokio, macros)
- **HTTP client**: reqwest 0.12 (rustls-tls + json)
- **Auth**: SHA-256 (sha2 + hex); tokens stored as hash in DB
- **Crypto**: aes-gcm 0.10 (available, usage TBD)
- **Config**: TOML via `toml` crate; config at `/etc/remo/server.toml`
- **CLI**: clap 4.5 (derive)
- **Build**: `cargo build` / `cargo test`

## Repo map

| Path | Role |
|------|------|
| `src/main.rs` | Binary entry point; dispatches CLI subcommands |
| `src/config.rs` | `ServerConfig` struct; reads TOML + master token file |
| `src/db.rs` | SQLite schema (`apps`, `users`, `deployments`, `env_vars`, `nodes`); all queries |
| `src/validation.rs` | `is_valid_app_name()` — shared validator |
| `src/server/mod.rs` | axum router; `AppState`; `user_routes()` + `admin_routes()` |
| `src/server/api.rs` | All HTTP handlers; `AppResponse`; `validate_domain()`; `safe_join()` |
| `src/server/auth.rs` | Bearer token middleware; `sha256_hex`; master + user token lookup |
| `src/server/proxy/` | Empty subdir (reserved) |
| `src/nano_client.rs` | HTTP client to nano-rs admin API (create/update/delete/reload/scale/drain/set_env) |
| `src/deploy/mod.rs` | `DeployContext`; `run()` — git archive → tar extract → symlink swap → nano reload |
| `src/cli/mod.rs` | CLI command dispatch |
| `src/cli/apps.rs` | `remo apps` subcommand |
| `src/cli/users.rs` | `remo users` subcommand |
| `src/cli/env.rs` | `remo env` subcommand |
| `src/cli/deploy.rs` | `remo deploy` subcommand |
| `src/cli/logs.rs` | `remo logs` subcommand (stub) |
| `src/cli/login.rs` | `remo login` subcommand |
| `src/cli/server.rs` | `remo server run` / `remo server install` (install unimplemented) |
| `src/cli/git_hook.rs` | SSH forced-command handler + post-receive deploy trigger |
| `src/cli/agent.rs` | `remo agent` subcommand |
| `src/proxy/mod.rs` | `ProxyBackend` trait |
| `src/proxy/nginx.rs` | nginx vhost stub (not called) |
| `src/proxy/caddy.rs` | Caddy config stub (not called) |
| `docs/DESIGN.md` | Architecture decisions + full API surface |
| `docs/SETUP_GUIDE.md` | VPS setup walkthrough |
| `GRILL.md` | `/ds-grill-me` decision log |
| `.project/PLAN.md` | Task roadmap with phase status |
| `tests/validation_tests.rs` | 12 validation unit tests |
