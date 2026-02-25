# Roadmap: Remo MVP

**Project:** Remo — Self-hosted reverse tunnel  
**Created:** 2026-02-18  
**Phases:** 3  
**Requirements:** 22 v1 (7 already working, 15 to implement)

---

## Phase 1: SSH Client Rewrite

**Plans:** 1 plan in 1 wave

Plans:
- [x] 01-01-PLAN.md — Rewrite client to use external ssh command (complete 2026-02-19)


**Goal:** Replace internal SSH dialer with external `ssh -R` command

**Requirements:** CLI-01, CLI-02, CLI-03, CLI-04, CLI-05, CLI-06

**Success Criteria:**
1. `remo connect` launches `ssh -R 0:localhost:18080` successfully
2. Client parses SSH's allocated port from verbose output
3. Client registers subdomain through the tunnel
4. Old `ssh.Dial` code removed from `internal/client/client.go`
5. No hangs, reliable reconnection

**Approach:**
- Rewrite client to use `exec.Command("ssh", ...)`
- Use `-v` flag to capture port assignment
- Parse output for "Allocated port" message
- Register with server via HTTP over tunnel
- Keep identity/Ed25519 for auth header

**Files to modify:**
- `internal/client/client.go` — Replace dialSSH, setupReverseTunnel
- `cmd/remo/root/connect.go` — May need flag adjustments

---

## Phase 2: TUI & Request Logging

**Plans:** 5 plans in 3 waves

Plans:
- [x] 02-tui-request-logging-01-PLAN.md — Server request event capture and /events endpoint (complete 2026-02-19)
- [x] 02-tui-request-logging-02-PLAN.md — Client event polling and TUI forwarding (complete 2026-02-19)
- [x] 02-tui-request-logging-03-PLAN.md — TUI visual enhancements: colors, path wrapping, help footer (complete 2026-02-19)
- [x] 02-tui-request-logging-04-PLAN.md — Keyboard controls: quit with export, error filter, pause (complete 2026-02-19)
- [x] 02-tui-request-logging-05-PLAN.md — Session statistics tracking and JSON log export (complete 2026-02-19)

**Goal:** Working TUI dashboard showing live request log

**Requirements:** TUI-01, TUI-02, TUI-03, TUI-04, TUI-05, SRV-03

**Success Criteria:**
1. TUI shows connection status and URL
2. HTTP requests appear in real-time
3. Each log line: method, path, status, latency
4. 'q' quits gracefully
5. 'c' clears the log

**Approach:**
- Add request event channel system
- Server proxy emits events after each request
- Client receives events and forwards to TUI
- TUI Update() handles RequestLogMsg
- Add 'q' and 'c' key handlers

**Files to modify:**
- `internal/server/server.go` — Emit request events
- `internal/tui/model.go` — Add key handlers, improve display
- `internal/client/client.go` — Wire events to TUI

---

## Phase 3: Nginx & Documentation

**Plans:** 2 plans in 2 waves

Plans:
- [x] 03-nginx-documentation-01-PLAN.md — Create nginx config and SSH setup documentation (complete 2026-02-19)
- [x] 03-nginx-documentation-02-PLAN.md — Rewrite README with architecture and quick start (complete 2026-02-19)

**Status:** ✅ COMPLETE

**Goal:** Production-ready with nginx, full documentation

**Requirements:** NGX-02, NGX-03, DOC-01, DOC-02

**Success Criteria:**
1. ✅ Working nginx config example for wildcard domains
2. ✅ Let's Encrypt setup documented
3. ✅ README has quick start
4. ✅ Admin endpoints documented

**Deliverables:**
- `docs/nginx-example.conf` — Production nginx configuration (98 lines)
- `docs/nginx.md` — Complete Let's Encrypt setup guide (356 lines)
- `docs/ssh-setup.md` — SSH key management documentation (224 lines)
- `README.md` — Comprehensive project documentation (378 lines)
- `docs/api.md` — API reference with authentication (453 lines)

**Approach:**
- Create `docs/nginx-example.conf`
- Update README with architecture diagram
- Document admin endpoint authentication
- SSH key setup guide

**Files created/modified:**
- `README.md` — Full rewrite with architecture diagram
- `docs/nginx.md` — Nginx + Let's Encrypt setup
- `docs/ssh-setup.md` — Key management
- `docs/api.md` — Complete API reference

---

## Requirements Mapping

| Phase | Requirements | New | Existing | Total |
|-------|--------------|-----|----------|-------|
| 1 | CLI-01 to CLI-06 | 6 | 0 | 6 |
| 2 | TUI-01 to TUI-05, SRV-03 | 6 | 0 | 6 |
| 3 | NGX-02, NGX-03, DOC-01, DOC-02 | 4 | 0 | 4 |
| — | SRV-01, SRV-02, SRV-04, SRV-05, NGX-01, ADM-01-03 | 0 | 7 | 7 |
| **Total** | | **16** | **7** | **23** |

---

## Progress Tracking

- [x] Phase 1 Plan 01 complete (2026-02-19) — SSH client rewritten
- [x] Phase 1 complete
- [x] Phase 2 Plan 01 complete (2026-02-19) — Server request event capture
- [x] Phase 2 Plan 02 complete (2026-02-19) — Client event polling
- [x] Phase 2 Plan 03 complete (2026-02-19) — TUI visual enhancements
- [x] Phase 2 Plan 04 complete (2026-02-19) — Keyboard controls for quit, filter, pause
- [x] Phase 2 Plan 05 complete (2026-02-19) — Session statistics and JSON export
- [x] Phase 2 complete
- [x] Phase 3 Plan 01 complete (2026-02-19) — Nginx config and SSH docs
- [x] Phase 3 Plan 02 complete (2026-02-19) — README rewrite
- [x] Phase 3 complete
- [x] Bugfix/Security complete (2026-02-22) — Rate limiting, connections view, state files
- [x] **PROJECT COMPLETE** (2026-02-24) — All phases finished, production ready

---

## Final Status

✅ **Remo v1.0 is complete and production-ready**

**Total Plans Executed:** 14 plans across 3 phases + 1 enhancement wave  
**Total Requirements:** 23/23 implemented  
**Total Lines of Documentation:** ~1,500 lines  
**Total Commits:** 40+ feature commits

---

## Notes

**Critical path resolved:** Phase 1 successfully replaced internal SSH dialer with external `ssh -R` command, eliminating hangs.

**Testing completed:**
- Phase 1: Tested with real VPS (ssh -R behavior verified)
- Phase 2: Local testing with mock requests and real TUI interaction
- Phase 3: Documentation validated for accuracy

**Key architectural decisions:**
1. External SSH command instead of Go SSH library (reliability)
2. TUI uses Bubble Tea framework with AltScreen mode
3. Rate limiting on admin endpoints (security)
4. 404 for all errors (prevents subdomain enumeration)
5. Localhost-only /events access (security through network isolation)

---
*Last updated: 2026-02-24 — PROJECT COMPLETE*
