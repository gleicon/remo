# Bug Fixes and Security Improvements

## Issues Identified

### Bug #1: Connections View Shows Nothing
**Root Cause:** `ConnectionsMsg` is defined in TUI but never sent by the client. There's no data flow from server to TUI for connections list.

**Missing Pieces:**
1. No `/connections` endpoint on server to list user's active tunnels
2. No code in client to fetch connections and send `ConnectionsMsg` to TUI
3. No refresh mechanism for connections view

### Bug #2: Old Tunnels Not Cleared or Shown as Stopped
**Root Cause:** The registry tracks tunnels server-side, but:
1. Client has no visibility into which tunnels are "active" vs "stopped" 
2. TUI shows all connections from state file, but doesn't distinguish status
3. No endpoint to get tunnel health status

### Security Issues Found

#### Issue #1: Admin Endpoint Brute Force Risk
- File: `internal/server/server.go:386-412`
- `/admin/cleanup` checks `X-Admin-Secret` header
- No rate limiting on failed attempts
- Risk: Brute force attack on admin secret
- Fix: Add rate limiting (max 5 attempts per minute per IP)

#### Issue #2: State File Permissions (If Created)
- File: Missing `internal/client/state.go` 
- If state file stores SSH keys or sensitive data, needs 0600 permissions
- Fix: Ensure state file has proper permissions when created

#### Issue #3: Error Information Disclosure  
- File: `internal/server/server.go` (error handling)
- Error headers (`X-Remo-Error: no-tunnel`) reveal infrastructure details
- Fix: Consider removing detailed error headers in production, only use for debugging

#### Issue #4: No Input Validation on Admin Secret
- Admin secret comparison is direct string comparison
- No timing attack protection (but Go's string comparison is constant-time)
- Low risk but worth noting

## Fix Plan

### Phase 1: Add /connections Endpoint (Critical)

**Server Changes:**
```go
// Add to Handler()
mux.HandleFunc("/connections", s.handleConnections)

// New handler
func (s *Server) handleConnections(w http.ResponseWriter, r *http.Request) {
    // Extract public key from header
    // Filter registry entries by pubKey
    // Return: []{subdomain, port, createdAt, lastPing, status}
}
```

**Client Changes:**
```go
// Add to client poll routine or create connections ticker
func (c *Client) pollConnections() {
    // Fetch from /connections endpoint
    // Parse response
    // Send ConnectionsMsg to TUI
}
```

### Phase 2: Show Tunnel Status Correctly

**Server Changes:**
- Add `status` field to connections response:
  - `"active"` - tunnel registered and receiving pings
  - `"stale"` - registered but no ping > timeout  
  - `"disconnected"` - not in registry

**TUI Changes:**
- Show status indicator: ● (green), ◐ (yellow), ● (red)
- Show "last ping" time
- Show connection age/uptime

### Phase 3: Security Improvements

1. **Rate Limit Admin Endpoint:**
   ```go
   // Simple in-memory rate limiter
   type rateLimiter struct {
       attempts map[string][]time.Time // IP -> []attempt timestamps
       mu sync.RWMutex
   }
   // Max 5 attempts per minute per IP
   ```

2. **Review Error Headers:**
   - Keep `X-Remo-Error` for debugging but remove in production
   - Or add config flag: `ExposeDebugHeaders: bool`

3. **State File Security:**
   - When creating state.go, ensure:
     - File permissions 0600
     - No sensitive data (SSH private keys)
     - Only store: subdomain, pid, port, timestamps

## Implementation Priority

**Critical (Fix First):**
1. Add `/connections` endpoint
2. Add client polling for connections
3. Fix TUI to display connections data

**High Priority:**
4. Add rate limiting to admin endpoint
5. Show tunnel status (active/stale) in TUI

**Medium Priority:**
6. Review error headers for info disclosure
7. Add state file with proper permissions

## Testing Checklist

- [ ] Connections view shows current user's connections
- [ ] Tab key switches to connections view
- [ ] Connections refresh every 5 seconds
- [ ] Status shows correctly (active/stale/stopped)
- [ ] Admin endpoint rejects after 5 failed attempts
- [ ] State file has 0600 permissions
- [ ] Ctrl+C exits cleanly with full-screen TUI
