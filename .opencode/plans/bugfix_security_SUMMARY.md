# Bug Fixes and Security Improvements - Execution Summary

**Plan:** bugfix-security
**Executed:** 2026-02-22
**Status:** ✅ COMPLETE

---

## Tasks Completed

### 1. Add /connections Endpoint ✅
**Files Modified:** `internal/server/server.go`, `internal/server/registry.go`

Added `/connections` HTTP endpoint that:
- Extracts public key from `X-Remo-Publickey` header
- Filters registry entries by public key to get user's tunnels
- Returns JSON array with: subdomain, port, createdAt, lastPing, status (active/stale)
- Restricts access to localhost-only (through SSH tunnel)

**Key Changes:**
- Exported `TunnelEntry` fields for cross-package access
- Added `listByPubKey()` method to registry
- Added `handleConnections()` handler with proper status calculation

**Commit:** `33ac457`

---

### 2. Add Client Connections Polling ✅
**Files Modified:** `internal/client/client.go`, `internal/tui/model.go`

Implemented client-side polling for connection data:
- `startConnectionsPolling()` runs every 5 seconds
- `pollConnections()` fetches from `/connections` endpoint
- Sends `ConnectionsMsg` to TUI with tunnel data
- Extended `ConnectionEntry` struct with `CreatedAt` and `LastPing` fields

**Key Changes:**
- Added `connectionsTicker` and `publicKeyBase64` to Client struct
- Polls independent of event polling (different intervals)
- Graceful error handling with debug logging

**Commit:** `95cead9`

---

### 3. Fix TUI Connections View ✅
**Files Modified:** `internal/tui/model.go`

Enhanced connections view with visual status indicators:
- Status indicators: ● green (active), ◐ yellow (stale), ● red (stopped)
- Added `stale` style for yellow color coding
- Shows last ping time (e.g., "5s ago", "2m ago", "1h ago")
- Calculates uptime from `CreatedAt` timestamp
- Added "Last Ping" column to connections table
- Updated column widths for new layout

**Key Changes:**
- Added `stale` lipgloss style (yellow color 220)
- Extended column layout with 6 columns
- Smart time formatting for ping recency

**Commit:** `7c9703c`

---

### 4. Rate Limit Admin Endpoint ✅
**Files Modified:** `internal/server/server.go`

Added IP-based rate limiting for `/admin/cleanup`:
- Simple in-memory rate limiter with IP tracking
- Max 5 attempts per minute per IP
- Returns 429 Too Many Requests with `Retry-After: 60` header when exceeded
- Rate limit checked before secret validation (prevents brute force)

**Key Changes:**
- Added `rateLimiter` struct with `attempts` map and time window
- Added `newRateLimiter()` constructor
- Added `check()` method with sliding window algorithm
- Integrated into Server struct and `handleAdminCleanup()`

**Commit:** `07990a0`

---

### 5. Review Error Headers ✅
**Files Modified:** `internal/server/server.go`

Verified error header security:
- `X-Remo-Error` headers are for debugging only
- Values are generic: `no-tunnel`, `no-upstream`
- No sensitive data exposed (no stack traces, paths, or internal details)
- Added comments documenting debug-only purpose

**Key Changes:**
- Added clarifying comments above `X-Remo-Error` header sets
- Verified values are safe to expose

**Commit:** `645958c`

---

### 6. Create State File with Permissions ✅
**Files Created:** `internal/client/state.go` (NEW FILE)

Created client-side state management with secure permissions:
- Stores: subdomain, pid, port, startTime
- **NO SSH keys or sensitive data stored**
- File permissions: `0600` (owner read/write only)
- Directory permissions: `0750` (parent `.remo` folder)
- Thread-safe with `RWMutex` protection

**Key Changes:**
- `ClientState` struct for persistent client data
- `StateManager` with `Load()`, `Save()`, `Get()`, `Clear()` methods
- Secure by default with minimal permissions

**Commit:** `82e31af`

---

## Deviation from Plan

### Auto-fixed: Test Updates (Rule 1)

**Found during:** Task 4 verification

**Issue:** Two server tests (`TestProxyNoTunnel`, `TestProxyWithSubdomainPrefix`) expected HTTP 502 status but server returns 404 for missing tunnels (changed in previous TUI Enhancement plan for security).

**Fix:** Updated test assertions to expect 404 instead of 502.

**Files Modified:** `internal/server/server_test.go`

**Commit:** `3b68ceb`

---

## Security Improvements Summary

| Feature | Before | After |
|---------|--------|-------|
| Connections visibility | Not visible | Full list with status |
| Admin endpoint | No rate limit | 5 attempts/min/IP |
| Error headers | Generic values | Documented as debug-only |
| State file | N/A | Created with 0600 permissions |
| Tunnel status | Unknown | Active/stale distinction |

---

## Commits

```
33ac457 feat(bugfix-security): add /connections endpoint to list user's tunnels
95cead9 feat(bugfix-security): add client connections polling
7c9703c feat(bugfix-security): fix TUI connections view with status indicators
07990a0 feat(bugfix-security): add rate limiting to admin endpoint
645958c docs(bugfix-security): document error headers as debug-only
82e31af feat(bugfix-security): create client state file with secure permissions
3b68ceb fix(bugfix-security): update tests for 404 status code
```

---

## Verification

✅ All tests pass (`go test ./...`)
✅ Code builds successfully (`go build ./...`)
✅ No breaking changes to existing functionality
✅ Security improvements implemented as specified

---

## Files Created

- `internal/client/state.go` - Client state management with secure permissions

## Files Modified

- `internal/server/server.go` - /connections endpoint, rate limiting, error docs
- `internal/server/registry.go` - Exported TunnelEntry, added listByPubKey
- `internal/client/client.go` - Connections polling
- `internal/tui/model.go` - Connections view with status indicators
- `internal/server/server_test.go` - Updated test expectations

---

## Self-Check: PASSED

- [x] All created files exist
- [x] All commits verified in git log
- [x] All tests passing
- [x] Build successful
- [x] No sensitive data exposed
- [x] Security requirements met
