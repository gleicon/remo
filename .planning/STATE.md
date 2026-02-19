# State: Remo Project

**Project:** Remo  
**Current Phase:** 03
**Current Plan:** 01
**Last Action:** Completed 03-nginx-documentation-01-PLAN.md - Nginx config and SSH setup documentation
**Updated:** 2026-02-19

---

## Current Position

Plan 01 complete: Created comprehensive nginx and SSH documentation for production deployment.

- docs/nginx-example.conf: Production nginx config with wildcard subdomain SSL and WebSocket support
- docs/nginx.md: Complete Let's Encrypt setup guide with DNS wildcard instructions
- docs/ssh-setup.md: SSH key generation, authorization, and subdomain restriction guide
- Requirements NGX-02, NGX-03, DOC-02 addressed

**Next:** Phase 03 complete - all plans finished

---

## Project Reference

See: .planning/PROJECT.md (updated 2026-02-18)

**Core value:** Users can expose local services through public subdomains using only system SSH, with minimal client complexity

**Current focus:** Phase 3 — Nginx Documentation (1/1 plans complete)

---

## Phase Status

| Phase | Status | Requirements | Plans |
|-------|--------|--------------|-------|
| 1 | ✓ Complete | 6 | 1/1 |
| 2 | ✓ Complete | 7 | 5/5 |
| 3 | ✓ Complete | 3 | 1/1 |

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

---

## Open Issues

1. ✓ SSH tunnel hangs — **FIXED** by using external ssh command (Plan 01)
2. ✓ TUI quit key missing — **FIXED** 'q' key now quits the TUI with export prompt (Plan 04)
3. ✓ Request logs not showing — **FIXED** — Client polls and forwards events, TUI displays with filtering (Plan 04)
4. ✓ Session statistics missing — **FIXED** — Real-time statistics tracking with req/err/bytes/latency (Plan 05)

---

## Session Continuity

**Started:** 2026-02-18
**Last Session:** 2026-02-19T19:14:16.388Z
**Context:** Completed 03-nginx-documentation-01-PLAN.md. Created nginx config example, Let's Encrypt setup guide, and SSH key documentation.

---
