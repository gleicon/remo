# nano-paas Design Decisions

Decisions recorded during `/ds-grill-me` session — 2026-07-31.
Feed into `/ds-spec` or implementation planning.

**Q: Server install UX — separate script or smart init?**
**A: `nano-paas server install` for VPS; `nano-paas login` for laptop.** Same binary, explicit subcommand namespaces. `server install` detects nginx/caddy, backs up configs, creates dirs/git-user/systemd/SQLite/token. Idempotent, `--reinit` to force. `login` saves server URL + token to `~/.nano-paas/config`. No ambiguity about which context the command targets.
*Rationale: `init` conflates two unrelated operations; explicit namespaces make intent obvious.*

---

**Q: First-run, install flow, proxy backend, user bootstrap**
**A:** `nano-paas init --domain apps.yourdomain.tld [--proxy nginx|caddy]` — single server setup. Auto-detects proxy: nginx present → nginx backend (default on Ubuntu); else Caddy. nginx path: per-app cert via `certbot --nginx` on `apps create` (HTTP-01, works because wildcard A record already resolves). Caddy path: on-demand TLS, no per-app action needed. Init creates: dir structure, SQLite DB, systemd units, git user, master token (chmod 600). Master token = admin, no separate admin user. `nano-paas users add <name> <pubkey>` creates restricted user with scoped token (own apps only). DNS: `*.apps.yourdomain.tld A VPS_IP` set once manually.
*Rationale: nginx default fits existing Ubuntu servers; Caddy opt-in for fresh installs; proxy backend abstraction keeps both maintainable.*

---

**Q: Git remote + CLI install**
**A: Explicit create required** before first push (`nano-paas apps create myapp`). Remote: `git remote add nano git@server:myapp`. SSH forced command parses app name from `$SSH_ORIGINAL_COMMAND`. Push to non-existent app rejected with error. CLI install: GitHub releases binary + `curl | sh` one-liner. Single binary handles both server-side (hook, control plane) and client-side (CLI).
*Rationale: explicit create prevents silent typo apps; one-liner install is zero-friction for dev onboarding.*

---

**Q: Deploy artifact storage**
**A: Content-addressed with symlink swap.** `/var/lib/nano-paas/apps/{name}/deploys/{sha}/`, `current/` symlink swapped atomically. Rollback = re-point symlink + admin API reload. nano-rs entrypoint points to `current/` — stable path, never changes. Hot reload via `POST /admin/apps/{hostname}/reload`. Keep last 5 deploys.
*Rationale: atomic cutover, free rollback, standard pattern (Capistrano/Dokku).*

---

**Q: Logs**
**A: Structured tail + filter.** `nano-paas logs myapp` tails nano-rs JSON stdout filtered by `hostname`. Deploy logs stored in SQLite (last 10 per app, 10KB cap), accessible via `nano-paas deployments myapp`. No extra processes.
*Rationale: nano-rs already emits structured JSON per-request; filtering is zero infra.*

---

**Q: Env var injection + supply chain risk**
**A: Wire `Nano.env` in nano-rs** (existing `env_vars` AppConfig field, never wired). Read-only in JS. Values never logged in tracing. Admin API requires auth to set/read. Supply chain risk (compromised dep exfiltrates via fetch) is real but identical to Cloudflare Workers/Deno — unavoidable for any runtime with env vars + outbound fetch. Per-isolate isolation prevents cross-app leakage. Outbound domain allowlist deferred to v2.
*Rationale: `env_vars` already in AppConfig — this is completion, not a new feature. Risk acknowledged and scoped.*

---

**Q: Build step**
**A: None.** Users commit built artifacts (compiled JS, WASM binary, static dist/). No server-side toolchain. Model matches Cloudflare Workers (wrangler uploads built output). Build command in nano.json deferred to v1.1.
*Rationale: keeps VPS clean; no Node.js/Rust toolchain dependency on the server.*

---

**Q: Project location and containment**
**A: `nano-paas/` subdirectory**, self-contained. All tests, docs, diagrams, CI, this GRILL.md move inside it. Zero trail at repo root — nothing nano-paas-specific outside the directory. Extractable to own repo by copying the directory. No `use nano::` Rust imports.
*Rationale: clean extraction path; contained artifacts make the boundary explicit.*

---

**Q: nano-paas ↔ nano-rs integration**
**A: Admin HTTP API** (`POST /admin/apps`, `PUT`, `DELETE`, `reload`, `scale`, `drain`). Unix socket for local node (no exposed port), HTTP for remote nodes. nano-rs config files never touched by nano-paas; no restart on deploy. nano-rs is unmodified.
*Rationale: admin API already implements everything a PaaS needs; Unix socket keeps local operations off the network.*

---

**Q: Multi-node goal**
**A: Scale-out (more apps/traffic), not HA.** `nodes` table in SQLite from day one (one row). Deploy artifacts stored by content hash. Control plane separate from data plane. Adding a node = `nano-paas node add <ip>` + agent subcommand in same binary. HA deferred.
*Rationale: HA requires Postgres and distributed consensus — too much complexity now. Scale-out just needs a node registry and artifact push.*

**Q: Binary count**
**A: One binary (`nano-paas`) + nano-rs unchanged.** `nano-paas server` runs control plane, `nano-paas git-hook` is the SSH forced command, all CLI subcommands in same binary. Multi-node agent = `nano-paas agent` subcommand, same binary deployed to worker nodes.
*Rationale: single install, single upgrade path; mode by subcommand.*

---

**Q: Auth model and persona flow**
**A: SSH keys for git push** (forced-command per key), **bearer token per persona for CLI**. Two personas: admin (master token from `server install`, full access) and users (scoped token, own apps only). Laptop setup: `curl install.sh | sh` then `nano-paas login` — writes `~/.nano-paas/config`, no other init. Admin adds users with `nano-paas users add <name> --pubkey <key>` — prints scoped token to share out-of-band. Single-owner: use master token directly, no user creation needed.
*Rationale: laptops have no init — login IS setup; only the VPS has an install step.*

---

**Q: State persistence**
**A: SQLite** at `/var/lib/nano-paas/state.db`. Secrets encrypted with server-side master key. Migration to multi-node = swap SQLite driver for Postgres, schema unchanged.
*Rationale: zero infrastructure dependency, trivial backup, schema-compatible with Postgres.*

---

**Q: App config format**
**A: `nano.json` in repo root**, with fallback auto-detect. Contains type, entrypoint, limits only — never secrets. Deploy hook reads it server-side and discards; file never enters VFS. Static handler blocklists `nano.json`, `.env`, `*.key`, `*.pem` — 404 if requested. Env vars/secrets stored server-side only, set via CLI, never committed.
*Rationale: versioned config ships with code; secrets separation is hard requirement.*

---

**Q: Static site serving — Caddy or nano-rs?**
**A: nano-rs** via existing `StaticDir` / `VfsStaticFiles` handlers. No new code needed in core. PaaS detects static type and registers `StaticDir` pointing at deploy root. Use case: HTML/CSS/JS bundles, landing pages — no large assets, no range requests needed.
*Rationale: routing stays unified through nano-rs; `StaticDir` already exists and works.*

---

**Q: Reverse proxy and TLS**
**A: Caddy with on-demand TLS, single static config.** DNS: one `*.apps.yourdomain.tld A VPS_IP` record, set once. Caddy config: one rule — `*.apps.yourdomain.tld` → proxy `localhost:8080`, on-demand TLS. Caddy generates cert per hostname on first hit via HTTP-01 (works because wildcard A record routes to VPS). nano-rs routes by Host header. No DNS API token. Caddy config never changes after init — nano-paas never touches it.
*Rationale: nano-rs already routes by Host; Caddy's only job is TLS termination. On-demand TLS eliminates all per-app Caddy management.*

---

**Q: Deploy model — git push or CLI?**
**A: Both.** Git remote (`git push nano main`) is the primary deploy path. CLI handles app management (create, delete, env, logs). CLI can also wrap `git push` for non-git projects.
*Rationale: git push is zero-new-concept for devs; hooks on the server are trivial; CLI avoids polluting the deploy primitive with admin concerns.*

---

