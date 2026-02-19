# Roadmap: Remo Refactoring

**Project:** Remo — Simplified reverse tunnel  
**Created:** 2026-02-18  
**Phases:** 3  
**Requirements:** 13 v1 requirements

---

## Phase 1: Client Simplification

**Goal:** Replace internal SSH dialer with external ssh command

**Requirements:** CLI-01, CLI-02, CLI-03, CLI-04, CLI-05

**Success Criteria:**
1. Client successfully creates tunnel using system ssh command
2. No hangs or connection issues
3. Tunnel registration works through ssh -R forwarded port
4. Old SSH dial code removed
5. Tests updated and passing

**Approach:**
- Rewrite `internal/client/client.go` to exec ssh command
- Use `-R` flag format: `-R 0:localhost:SERVER_PORT` (auto-assign remote port)
- Parse ssh output to detect assigned port
- Register with server through the tunnel

---

## Phase 2: TUI & Proxy Integration

**Goal:** Fix TUI issues and add request logging

**Requirements:** TUI-01, TUI-02, TUI-03, TUI-04, PROXY-01, PROXY-02, PROXY-03

**Success Criteria:**
1. Pressing 'q' quits the TUI gracefully
2. HTTP requests appear in TUI in real-time
3. TUI shows method, path, status code, latency
4. Client shuts down cleanly without hanging

**Approach:**
- Add 'q' key handler to TUI model
- Create event system between server proxy and client
- Proxy emits request events after each handled request
- Client receives events and forwards to TUI
- Clean up TUI goroutine on shutdown

---

## Phase 3: Documentation

**Goal:** Document admin endpoints and usage

**Requirements:** DOCS-01, DOCS-02, DOCS-03

**Success Criteria:**
1. README includes curl examples for /status, /metrics
2. SSH setup requirements clearly documented
3. Client usage examples show new simplified flow

**Approach:**
- Update README.md with admin endpoint examples
- Add TROUBLESHOOTING.md for common issues
- Update --help text with clearer descriptions

---

## Requirements Mapping

| Phase | Requirements | Count |
|-------|--------------|-------|
| 1 | CLI-01 to CLI-05 | 5 |
| 2 | TUI-01 to TUI-04, PROXY-01 to PROXY-03 | 7 |
| 3 | DOCS-01 to DOCS-03 | 3 |
| **Total** | | **15** |

---

## Progress Tracking

Update this section after each phase:

- [ ] Phase 1 complete
- [ ] Phase 2 complete
- [ ] Phase 3 complete

---

## Notes

**Critical path:** Phase 1 (client simplification) unblocks everything else. The SSH hang issue must be resolved first.

**Risk:** External ssh command dependency — need to verify ssh is available and handle gracefully if not.

**Testing:** Each phase needs manual testing with real VPS since integration testing SSH tunnels is difficult.

---
*Last updated: 2026-02-18*
