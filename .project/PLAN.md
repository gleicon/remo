# remo PLAN.md

## Current state

remo v0.5.8, nano-rs v2.7.0. Control plane live on VPS at remoapps.site. All phases complete.

---

## What is implemented and wired

- Git push deploy: SSH forced command → git-receive-pack → post-receive → nano-rs admin API
- Deployment rows: pending → success/failed with content-addressed sha on every push
- Rollback: compares deployment sha against current_sha (fixed — was comparing UUID)
- Proxy: `NoopProxy` in Docker mode (bind_addr=0.0.0.0), `NginxBackend` on bare VPS
- Custom domains: `PUT /api/apps/:name/domain` calls `proxy.provision_cert()`, `remo server apply-domain` writes nginx vhost + certbot
- App delete: deregisters from nano-rs + removes custom domain cert
- nano-rs re-registration: on startup and periodic sync loop
- Drain: `POST /api/apps/:name/drain` → nano-rs drain endpoint
- Scale: `POST /api/apps/:name/scale` → nano-rs scale endpoint
- Stats: `GET /api/apps/:name/stats` → filters nano-rs `/admin/isolates` by hostname
- `remo logs --follow`: polls `/api/apps/:name/stats` every 2s, prints on change
- Waitlist: `POST /waitlist` (public), admin list/approve/reject endpoints
- Landing site: `GET /` serves embedded HTML; live at https://remoapps.site
- Systemd unit: `remo server install` writes `/etc/systemd/system/remo.service` (bare-VPS mode)
- `remo server wire-site <domain>`: nginx vhost + certbot for the landing site domain
- Invites: `POST /api/admin/invites` creates a one-time token; `POST /api/invites/:token/claim` registers user + SSH key
- App templates: js, html, kv, spa, wasm, gas (user templates from `~/.remo/templates/`)

## Future features

| Feature | Prerequisite | Notes |
|---------|-------------|-------|
| Log streaming (`remo logs --follow` with real log lines) | nano-rs to expose a per-app log stream endpoint | Current `--follow` shows request count and heap from `/admin/isolates`, not log lines |
| `nano.json` per-app config at push time | git hook reads `nano.json` from archive | Would allow entrypoint override without re-creating the app |
| Multi-node horizontal scale | Agent design, artifact transfer, sha consensus | Big design — not started |
| Custom domain TLS in Docker mode | Host certbot callable from container, or a side-channel | Currently a warning; user runs certbot manually |
| Per-app resource limits via API | DB column + API field + DeployContext plumbing | workers default is 2; memory_mb and timeout_secs are in AppLimits but not user-settable |
