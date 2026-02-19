---
phase: 02-tui-request-logging
plan: 01
name: Request Event Tracking
completed: 2026-02-19
---

# Phase 02 Plan 01: Request Event Tracking Summary

Server now captures and exposes request events for TUI polling via HTTP endpoint.

## One-Liner

Added request event capture with circular buffer and `/events` endpoint for real-time request logging through SSH tunnel.

## What Was Built

### RequestEvent Type
- Struct matching TUI's `RequestLogMsg` with fields: Time, Method, Path, Status, Latency, Remote, BytesIn, BytesOut
- JSON-tagged for proper serialization

### Circular Buffer
- Thread-safe circular buffer using `sync.RWMutex`
- Capacity: 100 events (configurable via `maxEvents`)
- Automatic eviction of oldest events when capacity exceeded

### Event Capture
- `recordingResponseWriter` wrapper captures status code and bytes written
- Events emitted after each proxied request in `handleProxy`
- Records: timestamp, HTTP method, request path, response status, latency, remote IP, bytes transferred

### HTTP Endpoint
- `GET /events` returns JSON array of all captured events
- Restricted to localhost access only (ensures tunnel-only access)
- Returns `403 Forbidden` for non-localhost requests
- Returns `405 Method Not Allowed` for non-GET methods

## Key Implementation Details

```go
// Event capture in handleProxy
rw := &recordingResponseWriter{ResponseWriter: w}
proxy.ServeHTTP(rw, r)
latency := time.Since(start)

s.recordEvent(RequestEvent{
    Time:     time.Now(),
    Method:   r.Method,
    Path:     r.URL.RequestURI(),
    Status:   rw.statusCode,
    Latency:  latency,
    Remote:   remoteAddr,
    BytesIn:  int(r.ContentLength),
    BytesOut: rw.bytesWritten,
})
```

## Files Modified

| File | Changes |
|------|---------|
| `internal/server/server.go` | +100 lines: RequestEvent type, circular buffer, recordEvent method, recordingResponseWriter, handleEvents endpoint |

## Commits

| Hash | Message |
|------|---------|
| `1df0d11` | feat(02-tui-request-logging-01): add RequestEvent type and circular buffer to Server |
| `d840778` | feat(02-tui-request-logging-01): add recordEvent method and emit events from handleProxy |
| `a291f90` | feat(02-tui-request-logging-01): add /events HTTP endpoint to Handler |

## Verification

```bash
# Check RequestEvent type exists
grep -n "RequestEvent" internal/server/server.go

# Check recordEvent and recordingResponseWriter exist
grep -n "recordEvent\|recordingResponseWriter" internal/server/server.go

# Check /events endpoint registered
grep -n "handleEvents\|/events" internal/server/server.go
```

## Success Criteria Met

- [x] RequestEvent struct matches tui.RequestLogMsg fields
- [x] Server captures events after each proxied request
- [x] Circular buffer limits to 100 events
- [x] GET /events returns JSON array accessible through tunnel
- [x] Only localhost can access /events endpoint

## Next Steps

Plan 02 will add HTTP client polling from TUI to consume these events.

## Deviations from Plan

None - plan executed exactly as written.

## Self-Check

```bash
# Created files exist
[ -f ".planning/phases/02-tui-request-logging/02-tui-request-logging-01-SUMMARY.md" ] && echo "FOUND: SUMMARY.md"

# Commits exist
git log --oneline --all | grep -q "1df0d11" && echo "FOUND: commit 1df0d11"
git log --oneline --all | grep -q "d840778" && echo "FOUND: commit d840778"
git log --oneline --all | grep -q "a291f90" && echo "FOUND: commit a291f90"
```

## Self-Check: PASSED

All files created and commits verified.
