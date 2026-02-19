# Requirements: Remo Refactoring

**Defined:** 2026-02-18
**Core Value:** Users can expose local services through public subdomains using only system SSH, with minimal client complexity

## v1 Requirements

### Client Simplification

- [ ] **CLI-01**: Client uses external `ssh` command instead of internal SSH dialer
- [ ] **CLI-02**: Client launches ssh with `-R` flag for reverse tunnel
- [ ] **CLI-03**: Client detects when ssh tunnel is ready before registering
- [ ] **CLI-04**: Client handles ssh process lifecycle (start, monitor, restart)
- [ ] **CLI-05**: Remove `internal/client/client.go` SSH dial code

### TUI Fixes

- [ ] **TUI-01**: TUI responds to 'q' key for quit
- [ ] **TUI-02**: TUI shows request logs from proxy
- [ ] **TUI-03**: TUI handles graceful shutdown on exit
- [ ] **TUI-04**: Remove unused filter/pause complexity if not working

### Server Proxy

- [ ] **PROXY-01**: Proxy handler emits request log events for TUI
- [ ] **PROXY-02**: Proxy logs include method, path, status, latency
- [ ] **PROXY-03**: Proxy logs are sent to connected clients

### Documentation

- [ ] **DOCS-01**: Admin endpoints documented with curl examples
- [ ] **DOCS-02**: SSH setup requirements documented
- [ ] **DOCS-03**: Client usage examples updated

## v2 Requirements

### Client Enhancements

- **CLI-06**: Multiple tunnel support
- **CLI-07**: Config file for default settings
- **CLI-08**: Auto-retry with exponential backoff

### Server Enhancements

- **SRV-01**: WebSocket support
- **SRV-02**: Rate limiting per subdomain
- **SRV-03**: Bandwidth limits

## Out of Scope

| Feature | Reason |
|---------|--------|
| Built-in SSH server | Use system sshd, don't reinvent |
| Complex TUI (graphs, charts) | Keep minimal, logs are enough |
| OAuth/SSO auth | Ed25519 keys are sufficient |
| HTTPS auto-cert | Use behind nginx/traefik |
| Load balancing | Single server design for now |

## Traceability

| Requirement | Phase | Status |
|-------------|-------|--------|
| CLI-01 | Phase 1 | Pending |
| CLI-02 | Phase 1 | Pending |
| CLI-03 | Phase 1 | Pending |
| CLI-04 | Phase 1 | Pending |
| CLI-05 | Phase 1 | Pending |
| TUI-01 | Phase 2 | Pending |
| TUI-02 | Phase 2 | Pending |
| TUI-03 | Phase 2 | Pending |
| TUI-04 | Phase 2 | Pending |
| PROXY-01 | Phase 2 | Pending |
| PROXY-02 | Phase 2 | Pending |
| PROXY-03 | Phase 2 | Pending |
| DOCS-01 | Phase 3 | Pending |
| DOCS-02 | Phase 3 | Pending |
| DOCS-03 | Phase 3 | Pending |

**Coverage:**
- v1 requirements: 13 total
- Mapped to phases: 13
- Unmapped: 0 ✓

---
*Requirements defined: 2026-02-18*
*Last updated: 2026-02-18 after code review*
