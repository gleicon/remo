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
- [ ] 02-tui-request-logging-01-PLAN.md — Server request event capture and /events endpoint
- [ ] 02-tui-request-logging-02-PLAN.md — Client event polling and TUI forwarding
- [ ] 02-tui-request-logging-03-PLAN.md — TUI core display: colors, path wrapping, help footer
- [ ] 02-tui-request-logging-04-PLAN.md — Keyboard controls: quit with export, error filter, pause
- [ ] 02-tui-request-logging-05-PLAN.md — Session statistics tracking and JSON log export

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

**Goal:** Production-ready with nginx, full documentation

**Requirements:** NGX-02, NGX-03, DOC-01, DOC-02

**Success Criteria:**
1. Working nginx config example for wildcard domains
2. Let's Encrypt setup documented
3. README has quick start
4. Admin endpoints documented

**Approach:**
- Create `docs/nginx-example.conf`
- Update README with architecture diagram
- Document admin endpoint authentication
- SSH key setup guide

**Files to create/modify:**
- `README.md` — Full rewrite
- `docs/nginx.md` — Nginx setup
- `docs/ssh-setup.md` — Key management

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
- [ ] Phase 1 complete
- [ ] Phase 2 complete
- [ ] Phase 3 complete

---

## Notes

**Critical path:** Phase 1 unblocks everything. The SSH hang is the primary blocker.

**Testing strategy:**
- Phase 1: Test with real VPS (ssh -R behavior varies)
- Phase 2: Local testing with mock requests
- Phase 3: Production nginx setup test

**Risk mitigation:**
- Keep old code in git history (don't delete, just replace)
- Test ssh command availability on macOS/Linux/Windows
- Fallback behavior if ssh not found

---
*Last updated: 2026-02-19*
