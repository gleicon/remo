# remo Design

Extracted from `/ds-grill-me` design session. See `GRILL.md` for full decision log.

## What It Is

Single-VPS edge PaaS for JS, WASM, and static HTML apps built on nano-rs.

- Deploy: `git push remo main`
- Live: `https://myapp.apps.yourdomain.tld`
- No Kubernetes, no Docker, no build server

## Architecture

```
laptop
  git push
    │
    ▼ SSH (forced command)
  remo git-hook --user alice
    │
    ├─ auth (user owns app?)
    ├─ exec git-receive-pack <bare-repo>
    └─ post-receive: remo git-hook --deploy myapp --sha <sha>
                          │
                          ├─ git archive → tar bytes
                          ├─ extract to deploys/<sha>/
                          ├─ symlink swap: current/ → deploys/<sha>/
                          └─ nano admin API: create/update + reload

VPS
  nginx / Caddy  :443  ──▶  nano-rs  :8080  (Host header routing)
  remo control   :7070      (localhost only)
  nano-rs admin  :9000      (localhost only)
```

## Key Decisions

| Decision | Choice | Reason |
|----------|--------|--------|
| Binary count | 1 (`remo`) | Single install, single upgrade |
| App routing | nano-rs by Host header | Already works; proxy is TLS-only |
| Proxy | nginx default, Caddy opt-in | nginx standard on Ubuntu |
| TLS | certbot per-app (nginx) / on-demand (Caddy) | No DNS API token needed |
| State | SQLite `/var/lib/remo/state.db` | Zero infra, Postgres-compatible schema |
| Deploy artifact | content-addressed tar, symlink swap | Atomic, free rollback |
| Env vars | encrypted at rest, Nano.env in V8 | Completion of existing AppConfig field |
| Build step | none | Users commit built output |
| Static sites | nano-rs StaticDir handler | No new infra |
| Multi-node | SQLite `nodes` table now, swap driver later | Schema stable |
| Auth (git) | SSH forced command per key | No extra server |
| Auth (CLI) | Bearer token, admin=master, users=scoped | Simple, auditable |

## nano-rs Change Required

One addition: wire `Nano.env` in the V8 runtime.

`env_vars: HashMap<String, String>` already exists in `AppConfig` but is never injected into the JS context. Adding ~20 lines to `src/runtime/web_apis.rs` would expose `globalThis.Nano.env` as a read-only frozen object.

This is the only nano-rs source change remo requires. Everything else goes through the existing admin HTTP API.

## Directory Layout

```
/var/lib/remo/
├── state.db
├── apps/
│   └── myapp/
│       ├── git/           (bare repo — managed by remo, not nano-rs)
│       ├── deploys/
│       │   ├── abc123/    (extracted tar)
│       │   └── def456/
│       └── current -> deploys/abc123/
/etc/remo/
├── server.toml
└── master_token           (chmod 600)
```

## URL / Hostname Scheme

Canonical hostname: `{owner}-{name}.{domain}`  
Example: `alice-myapp.apps.yourdomain.tld`

Owner prefix prevents subdomain squatting — two users can each have an app named `api` without conflict.

Custom CNAME: store `custom_domain` on the app row. The proxy serves both the canonical hostname and the custom domain. Users set it via `PUT /api/apps/:name/domain`; point their DNS CNAME at the canonical hostname.

## Control Plane API

```
GET    /health                           public
GET    /api/apps                         list apps (own, or all if admin)
POST   /api/apps                         create app
GET    /api/apps/:name                   app detail
DEL    /api/apps/:name                   delete app
GET    /api/apps/:name/deployments       deployment history
POST   /api/apps/:name/rollback          roll back to previous/named sha
POST   /api/apps/:name/scale             set worker count
GET    /api/apps/:name/env               list env keys (values masked)
POST   /api/apps/:name/env               set env vars (body: {vars:{k:v}})
DEL    /api/apps/:name/env/:key          unset env var
PUT    /api/apps/:name/domain            set custom domain (body: {domain:"..."})
DEL    /api/apps/:name/domain            clear custom domain
GET    /api/apps/:name/logs              deployment logs
GET    /api/users                        (admin only)
POST   /api/users                        (admin only)
DEL    /api/users/:name                  (admin only)
```

### AppResponse shape

```json
{
  "name":         "myapp",
  "hostname":     "alice-myapp.apps.yourdomain.tld",
  "owner":        "alice",
  "app_type":     "js",
  "entrypoint":   "index.js",
  "current_sha":  "abc123...",
  "custom_domain": "myapp.example.com",
  "created_at":   "2026-07-31T00:00:00Z"
}
```
