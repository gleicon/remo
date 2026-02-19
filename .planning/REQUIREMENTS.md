# Requirements: Remo MVP

**Defined:** 2026-02-18
**Core Value:** Developers can expose local services through public subdomains using system SSH, with a live TUI dashboard

## v1 Requirements (MVP)

### Client (SSH + Registration)

- [x] **CLI-01**: Client launches `ssh -R 0:localhost:SERVER_PORT` to auto-assign remote port
- [x] **CLI-02**: Client parses SSH output to detect assigned remote port
- [x] **CLI-03**: Client registers subdomain with server through tunnel
- [x] **CLI-04**: Client monitors SSH process and reconnects on failure
- [x] **CLI-05**: Client identity loaded from `~/.remo/identity.json`
- [x] **CLI-06**: Remove internal SSH dialer code

### TUI (Request Log Dashboard)

- [ ] **TUI-01**: TUI displays connection status and assigned URL
- [ ] **TUI-02**: TUI shows scrolling request log (method, path, status, latency)
- [ ] **TUI-03**: Pressing 'q' quits gracefully
- [ ] **TUI-04**: Pressing 'c' clears log
- [ ] **TUI-05**: TUI updates in real-time as requests flow

### Server (Proxy + Registry)

- [ ] **SRV-01**: Server accepts registration via HTTP through tunnel
- [ ] **SRV-02**: Server routes requests by subdomain to tunnel port
- [x] **SRV-03**: Server emits request events for TUI consumption
- [ ] **SRV-04**: Server works behind nginx (X-Forwarded-For handling)
- [ ] **SRV-05**: Server validates client public key on registration

### Nginx Integration

- [ ] **NGX-01**: Server runs in "behind-proxy" mode (no TLS)
- [ ] **NGX-02**: Nginx config example for `*.domain.com` wildcard
- [ ] **NGX-03**: Documentation for nginx + let's encrypt setup

### Admin & Docs

- [ ] **ADM-01**: `/healthz` endpoint returns 200 OK
- [ ] **ADM-02**: `/status` endpoint returns tunnel list (with auth)
- [ ] **ADM-03**: `/metrics` endpoint returns Prometheus format (with auth)
- [ ] **DOC-01**: README with quick start and nginx setup
- [ ] **DOC-02**: SSH key setup documentation

## v2 Requirements

### Client Enhancements

- **CLI-07**: Config file support (`~/.remo/config.yaml`)
- **CLI-08**: Multiple subdomain reservations
- **CLI-09**: HTTP basic auth for tunnel endpoints

### Server Enhancements

- **SRV-06**: Per-client bandwidth limits
- **SRV-07**: Request rate limiting
- **SRV-08**: WebSocket support through tunnel

### Management

- **ADM-04**: Web admin dashboard (simple HTML)
- **ADM-05**: Reservation management API

## Out of Scope

| Feature | Reason |
|---------|--------|
| Built-in SSH server | Use system sshd |
| WireGuard support | SSH -R is sufficient |
| Built-in TLS/HTTPS | Nginx handles this better |
| OAuth/SSO | Key auth is simpler |
| Multi-server federation | Single VPS for MVP |
| Complex TUI (graphs, filters) | Keep it top-like simple |
| Real-time collaboration | Not a core need |

## Traceability

| Requirement | Phase | Status |
|-------------|-------|--------|
| CLI-01 | Phase 1 | ✓ Complete (01-01) |
| CLI-02 | Phase 1 | ✓ Complete (01-01) |
| CLI-03 | Phase 1 | ✓ Complete (01-01) |
| CLI-04 | Phase 1 | ✓ Complete (01-01) |
| CLI-05 | Phase 1 | ✓ Complete (01-01) |
| CLI-06 | Phase 1 | ✓ Complete (01-01) |
| TUI-01 | Phase 2 | Pending |
| TUI-02 | Phase 2 | Pending |
| TUI-03 | Phase 2 | Pending |
| TUI-04 | Phase 2 | Pending |
| TUI-05 | Phase 2 | Pending |
| SRV-01 | Phase 1 | ✓ Exists |
| SRV-02 | Phase 1 | ✓ Exists |
| SRV-03 | Phase 2 | Complete |
| SRV-04 | Phase 1 | ✓ Exists |
| SRV-05 | Phase 1 | ✓ Exists |
| NGX-01 | Phase 3 | ✓ Exists |
| NGX-02 | Phase 3 | Pending |
| NGX-03 | Phase 3 | Pending |
| ADM-01 | Phase 3 | ✓ Exists |
| ADM-02 | Phase 3 | ✓ Exists |
| ADM-03 | Phase 3 | ✓ Exists |
| DOC-01 | Phase 3 | Pending |
| DOC-02 | Phase 3 | Pending |

**Coverage:**
- v1 requirements: 22 total
- Already working: 7
- To implement: 15
- Unmapped: 0 ✓

---
*Requirements defined: 2026-02-18*
*Last updated: 2026-02-18 after spec review*
