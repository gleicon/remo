# State: Remo Project

**Project:** Remo
**Current Phase:** TUI Enhancement
**Current Plan:** Complete
**Last Action:** Completed TUI Enhancement Plan - All 5 phases implemented
**Updated:** 2026-02-22

---

## Current Position

TUI Enhancement Plan complete: All 5 phases implemented, tested, and committed.

**Phase 1: Full-Screen TUI** - Added tea.WithAltScreen() for alternate screen buffer mode
**Phase 2: Connections View** - Tab-based view switching between Logs and Connections views
**Phase 3: CLI Commands** - Added `remo connections` and `remo kill` commands with state management
**Phase 4: Error Handling** - Changed 502 to 404 with X-Remo-Error headers for security
**Phase 5: Inline Errors** - Added dismissible error banner in TUI

**Files Created:**
- internal/state/state.go - Connection state management
- cmd/remo/root/connections.go - Connections CLI command
- cmd/remo/root/kill.go - Kill CLI command

**Files Modified:**
- internal/client/client.go - Alt screen mode
- internal/server/server.go - 404 error responses
- internal/tui/model.go - Tab views, error banner
- cmd/remo/root/root.go - New commands

**Summary:** .opencode/plans/TUI_ENHANCEMENT_SUMMARY.md

**Next:** All phases complete - TUI enhancements ready for use

---

## Project Reference

See: .planning/PROJECT.md (updated 2026-02-18)

**Core value:** Users can expose local services through public subdomains using only system SSH, with minimal client complexity

**Current focus:** Phase 3 — Nginx Documentation (2/2 plans complete)

---

## Phase Status

| Phase | Status | Requirements | Plans |
|-------|--------|--------------|-------|
| 1 | ✓ Complete | 6 | 1/1 |
| 2 | ✓ Complete | 7 | 5/5 |
| 3 | ✓ Complete | 3 | 2/2 |
| TUI Enhancement | ✓ Complete | 5 | 5/5 |

---

## Recent Decisions

1. **Nginx upstream address** — Used 127.0.0.1:18080 as upstream (Remo's default behind-proxy mode)
2. **SSH key algorithm** — Documented Ed25519 as preferred over RSA for better security and performance
3. **Documentation structure** — Included comprehensive comments in nginx config explaining each directive
4. **Subdomain rules format** — Documented `*` (any), `prefix-*` (wildcard), and `exact-name` (specific) patterns
5. **Request event capture approach** — Circular buffer with RWMutex for thread-safe access
6. **Localhost-only /events access** — Ensures events only accessible through SSH tunnel
7. **Event structure matching** — RequestEvent mirrors tui.RequestLogMsg for consistency
8. **Recording response writer** — Wraps http.ResponseWriter to capture status and bytes
9. **Polling interval** — 1 second polling balances real-time feel with resource usage
10. **Event deduplication** — lastEventIndex prevents duplicate forwarding to TUI
11. **Status code color scheme** — Green 2xx, Blue 3xx, Yellow 4xx, Red 5xx per HTTP conventions
12. **Path wrapping** — Multi-line display for paths >40 chars instead of truncation
13. **Terminal height adaptation** — Reserve 3 header + 1 footer lines, use remaining space for logs
14. **Export prompt state machine** — exportPrompt flag intercepts all key input until resolved
15. **Error filter predicate** — shouldShowEntry() combines text filter and error-only filter
16. **ASCII architecture diagram** — Portable, renders everywhere without external dependencies
17. **README structure** — Header → What is → Quick Start → How It Works → Configuration → Troubleshooting
18. **Security documentation** — Explicit localhost-only /events access documented
19. **AltScreen for TUI** — Using tea.WithAltScreen() for full-screen terminal mode
20. **Tab view switching** — Tab/Shift+Tab cycles between Logs and Connections views
21. **Connection state storage** — ~/.remo/state.json tracks active connections
22. **404 vs 502 errors** — Both tunnel and upstream errors return 404 with X-Remo-Error header
23. **Inline error display** — Dismissible error banner in TUI with type, message, and timestamp
24. **Process termination** — Kill command uses os.FindProcess() and Kill() for safe termination
25. **CLI confirmations** — Both single and bulk kill operations require user confirmation

---

## Open Issues

1. ✓ SSH tunnel hangs — **FIXED** by using external ssh command (Plan 01)
2. ✓ TUI quit key missing — **FIXED** 'q' key now quits the TUI with export prompt (Plan 04)
3. ✓ Request logs not showing — **FIXED** — Client polls and forwards events, TUI displays with filtering (Plan 04)
4. ✓ Session statistics missing — **FIXED** — Real-time statistics tracking with req/err/bytes/latency (Plan 05)

---

## Session Continuity

**Started:** 2026-02-18
**Last Session:** 2026-02-22
**Context:** Completed TUI Enhancement Plan with all 5 phases: full-screen TUI, tab-based view switching, CLI commands (connections/kill), 404 error handling, and inline error banner.

---
