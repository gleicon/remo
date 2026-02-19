# Phase 2: TUI & Request Logging - Context

**Gathered:** 2026-02-19
**Status:** Ready for planning

<domain>
## Phase Boundary

Working TUI dashboard showing live request log. Wire up request events from server proxy through client to TUI, add 'q' quit behavior, and improve display. This phase focuses on the event flow and user experience within the existing TUI framework.

**Scope includes:**
- Event transport from server → client → TUI
- Request log display with full details
- Quit behavior with graceful shutdown
- Layout improvements (status bar, help footer)

**Scope excludes:**
- New TUI features beyond logging (no new panels, tabs, or views)
- Server-side changes beyond event emission
- Authentication or authorization changes
- Performance optimizations outside event flow

</domain>

<decisions>
## Implementation Decisions

### Request Event Transport
- **Mechanism:** HTTP polling through the existing SSH tunnel
  - Client exposes local HTTP endpoint for TUI to poll
  - TUI polls every second for new events
  - Works through existing tunnel, no additional network exposure
- **Reconnection:** Auto-reconnect with exponential backoff on disconnect
  - Silent retry attempts with increasing delays
  - Display connection status in TUI when disconnected
- **Event History:** Last 100 requests by default, configurable to full session
  - Buffer size configurable via flag or config
  - Memory-safe default prevents unbounded growth

### Log Display Format
- **Fields shown:** All information per request
  - Timestamp (HH:MM:SS format)
  - HTTP Method (GET, POST, etc.)
  - Full Path (multi-line display for long paths)
  - Status Code
  - Latency
  - Remote IP address
  - Bytes In / Bytes Out
- **Status code coloring:** Standard HTTP colors
  - 2xx responses: Green
  - 3xx responses: Blue
  - 4xx responses: Yellow
  - 5xx responses: Red
- **Path handling:** Multi-line display for long paths
  - Don't truncate with ellipsis
  - Wrap to show full path when it exceeds terminal width
- **Error view:** 'e' key toggles errors-only filter
  - Show visual indicator in status bar when active (e.g., "| errors only")
  - Filter shows only 4xx and 5xx responses

### Quit Behavior
- **Quit trigger:** Press 'q' for immediate graceful shutdown
  - No confirmation dialog, but require single press
  - Shutdown sequence begins immediately
- **Active requests:** Interrupt immediately
  - Don't wait for in-flight requests to complete
  - Close SSH tunnel, connections drop
- **Shutdown message:** Brief "Shutting down..." displayed
  - Show for 1-2 seconds before exit
  - Clear and professional
- **Data export:** Prompt to save request log before exit
  - "Export session log to file? (y/n)" prompt
  - Default filename: remo-log-{subdomain}-{timestamp}.json
  - Export format: JSON array of RequestLogMsg objects

### TUI Layout & Information Density
- **Help footer:** Always visible at bottom
  - Shows all available key bindings
  - Format: "q:quit c:clear e:errors p:pause /:filter"
  - Compact single-line display
- **Status bar:** Full multi-line status display
  - Line 1: Connection status, subdomain, attempt counter
  - Line 2: Tunnel health (if applicable), last error message
  - Line 3: Current URL (if registered)
- **Statistics:** Compact inline stats in header
  - Format: "req {N} err {N} bytes {in}/{out} avg {N}ms"
  - Real-time counters for current session
- **Log display:** Fill available terminal space
  - Use all remaining height after header and footer
  - No artificial limit on visible lines
  - Scroll naturally with new entries at bottom

### Claude's Discretion
- Exact polling interval (suggested: 1 second, but can adjust based on performance)
- Specific color hex values (use lipgloss defaults for standard colors)
- Export file format details (JSON structure, field names)
- Error message wording and formatting
- Exact exponential backoff timing for reconnection
- Help footer exact formatting and key order

</decisions>

<specifics>
## Specific Ideas

- Event endpoint should be simple: GET /events?since={timestamp} returning JSON array
- TUI model already has `logs []RequestLogMsg` — extend this pattern
- Current key handlers (p, c, e, /) should remain functional
- Status bar should feel like `top` or `htop` — informative but not cluttered
- Export prompt should appear after shutdown initiated but before process exits
- Multi-line path display should indent continuation lines for readability

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope

</deferred>

---

*Phase: 02-tui-request-logging*
*Context gathered: 2026-02-19*
