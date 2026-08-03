# now
Building nano-rs v2.2.1 from source on VPS (REDACTED_VPS_IP). Background task b0ad8fcfi running `docker compose build nano-rs` with rust:1.88-slim. Also building remo images (already built with updated NanoClient).

# next
1. Verify nano-rs build succeeded (check b0ad8fcfi output)
2. `docker compose up -d` on VPS with NANO_ADMIN_API_KEY=REDACTED_NANO_KEY
3. Test nano-rs admin API: `curl -H 'X-Admin-Key: ...' http://127.0.0.1:8889/admin/apps`
4. Test remo health: `curl https://cloud.remoapps.site/health`
5. Add operator SSH key to /etc/remo/authorized_keys for git push test
6. Commit all code changes locally
7. Add startup app sync to remo server/mod.rs (reads DB, pushes apps to nano-rs on start)

# settled
- VPS: ubuntu@REDACTED_VPS_IP, SSH key: ~/.ssh/id_rsa_mgc_saas_apps
- Domain: cloud.remoapps.site, *.cloud.remoapps.site → REDACTED_VPS_IP
- Tokens: NANO_ADMIN_KEY=REDACTED_NANO_KEY, MASTER_TOKEN=REDACTED_MASTER_TOKEN
- server.toml at /etc/remo/server.toml has bind_addr="0.0.0.0" for Docker
- nginx routes: *.cloud.remoapps.site → :8080 (nano-rs), cloud.remoapps.site → :7070 (remo)
- Docker compose file: /home/ubuntu/remo/docker-compose.yml
- nano-rs v2.1.0-alpha has NO admin API; v2.2.1 has admin API on :8889 (no prebuilt binary)
- nano-rs runs in no-config mode (nano-rs run) to enable admin API
- axum 0.8 uses {param} not :param in route paths — already fixed
- remo bind_addr must be "0.0.0.0" in Docker (loopback not reachable via port mapping)
- NanoClient updated: uses entrypoint (file path) not script (inline code) — matches v2.2.1 API
- update_app uses PATCH not PUT (v2.2.1 API)

# hazards
- Don't restart minimidia or vectoria containers — they serve production traffic
- /etc/remo is mode 0700 — sudo required to read contents
- NANO_ADMIN_API_KEY must be passed via `sudo -E docker compose up` or docker-compose .env file
- nano-rs v2.2.1 admin API only starts if NANO_ADMIN_API_KEY env var is non-empty
- nano-rs in config mode (--config) does NOT start admin API — must use no-config mode
- Deployment rows NOT yet written to DB on git push (production gap)
- remo does NOT sync existing apps to nano-rs on startup (production gap — add startup sync)
