---
phase: 02-tui-request-logging
plan: 05
type: execute
wave: 3
depends_on:
  - 02-tui-request-logging-04
files_modified:
  - internal/client/client.go
  - internal/tui/model.go
autonomous: true
requirements:
  - TUI-05

must_haves:
  truths:
    - Log export saves to remo-log-{subdomain}-{timestamp}.json format
    - Statistics format: "req {N} err {N} bytes {in}/{out} avg {N}ms" displayed in header
    - Statistics update in real-time as requests arrive
    - Export function available in client to handle QuitMsg from TUI
    - 'c' key clears both logs and statistics
  artifacts:
    - path: internal/client/client.go
      provides: Log export functionality and quit handling
      contains: "exportLogToFile", "requestLogEntry", "exportedLogs"
    - path: internal/tui/model.go
      provides: Session statistics tracking and display
      contains: "SessionStats", "statsLine", "stats.*RequestCount"
    - path: internal/tui/model.go
      provides: Real-time statistics updates
      contains: "m.stats.RequestCount++", "m.stats.ErrorCount++"
  key_links:
    - from: internal/tui/model.go Update
      to: internal/client/client.go
      via: QuitMsg signal triggers export
      pattern: "QuitMsg"
    - from: internal/client/client.go
      to: JSON file
      via: exportLogToFile writes timestamped log file
      pattern: "remo-log-.*json"
---

<objective>
Implement session statistics tracking with real-time display and JSON log export functionality.

Purpose: Add statistics header showing request count, error count, bytes transferred, and average latency. Implement log export to JSON file when user quits with 'y' response.
Output: TUI displays live statistics in header format "req {N} err {N} bytes {in}/{out} avg {N}ms", client can export logs to remo-log-{subdomain}-{timestamp}.json.
</objective>

<execution_context>
@/Users/gleicon/.config/opencode/get-shit-done/workflows/execute-plan.md
</execution_context>

<context>
@internal/tui/model.go
@internal/client/client.go

Current state:
- TUI has logs but no statistics tracking
- Client receives logs but doesn't store them for export
- QuitMsg defined in 02-04 but export not yet implemented
- Need to coordinate between TUI stats and client export

Locked decisions:
- Statistics format: "req {N} err {N} bytes {in}/{out} avg {N}ms"
- Export filename: remo-log-{subdomain}-{timestamp}.json
- Statistics update on every RequestLogMsg
- Clear ('c') resets both logs and statistics
- Export includes all fields: Time, Method, Path, Status, Latency, Remote, BytesIn, BytesOut
</context>

<tasks>

<task type="auto">
  <name>Add session statistics tracking and display</name>
  <files>internal/tui/model.go</files>
  <action>
1. Add statistics struct and fields to Model:
   ```go
   type SessionStats struct {
       RequestCount  int
       ErrorCount    int
       BytesIn       int64
       BytesOut      int64
       TotalLatency  time.Duration
   }
   
   // Add to Model struct:
   stats SessionStats
   ```

2. Update statistics when receiving RequestLogMsg in Update():
   ```go
   case RequestLogMsg:
       m.logs = append(m.logs, msg)
       
       // Update statistics
       m.stats.RequestCount++
       if msg.Status >= 400 {
           m.stats.ErrorCount++
       }
       m.stats.BytesIn += int64(msg.BytesIn)
       m.stats.BytesOut += int64(msg.BytesOut)
       m.stats.TotalLatency += msg.Latency
   ```

3. Create statistics display helper:
   ```go
   func (m Model) statsLine() string {
       avgLatency := time.Duration(0)
       if m.stats.RequestCount > 0 {
           avgLatency = m.stats.TotalLatency / time.Duration(m.stats.RequestCount)
       }
       
       style := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
       statsText := fmt.Sprintf("req %d err %d bytes %d/%d avg %dms",
           m.stats.RequestCount,
           m.stats.ErrorCount,
           m.stats.BytesIn,
           m.stats.BytesOut,
           avgLatency.Milliseconds(),
       )
       return style.Render(statsText)
   }
   ```

4. Add statistics line to View() header:
   Insert after subdomain status line:
   ```go
   b.WriteString(m.statsLine())
   b.WriteString("\n")
   ```

5. Clear stats when 'c' key pressed:
   In the "c" key handler, also reset stats:
   ```go
   case "c", "C":
       m.logs = []RequestLogEntry{}
       m.stats = SessionStats{}  // Reset statistics
       return m, nil
   ```
  </action>
  <verify>
    grep -n "SessionStats\|statsLine\|stats.*RequestCount" internal/tui/model.go
  </verify>
  <done>Statistics displayed in header: req/err/bytes/avg latency, updates in real-time</done>
</task>

<task type="auto">
  <name>Implement log export and quit handling in client</name>
  <files>internal/client/client.go</files>
  <action>
Add log export functionality and quit handling to the client:

1. Add exportedLogs slice to Client struct to track logs for export:
   ```go
   exportedLogs []requestLogEntry
   logsMu       sync.RWMutex
   ```

2. Create requestLogEntry struct:
   ```go
   type requestLogEntry struct {
       Time     time.Time     `json:"time"`
       Method   string        `json:"method"`
       Path     string        `json:"path"`
       Status   int           `json:"status"`
       Latency  time.Duration `json:"latency"`
       Remote   string        `json:"remote"`
       BytesIn  int           `json:"bytes_in"`
       BytesOut int           `json:"bytes_out"`
   }
   ```

3. Modify pollAndForwardEvents to also store logs locally:
   When forwarding to TUI, also append to exportedLogs (respecting max 100 limit).

4. Create exportLogToFile function:
   ```go
   func (c *Client) exportLogToFile() (string, error) {
       timestamp := time.Now().Format("20060102-150405")
       filename := fmt.Sprintf("remo-log-%s-%s.json", c.cfg.Subdomain, timestamp)
       
       c.logsMu.RLock()
       logs := make([]requestLogEntry, len(c.exportedLogs))
       copy(logs, c.exportedLogs)
       c.logsMu.RUnlock()
       
       data, err := json.MarshalIndent(logs, "", "  ")
       if err != nil {
           return "", fmt.Errorf("marshal logs: %w", err)
       }
       
       if err := os.WriteFile(filename, data, 0644); err != nil {
           return "", fmt.Errorf("write file: %w", err)
       }
       
       return filename, nil
   }
   ```

5. Wire up export when TUI sends QuitMsg:
   The client needs to handle QuitMsg from the TUI program. When TUI exits with Export: true, call exportLogToFile().
   
   In the client's UI management code (where tea.Program runs), after the program exits:
   ```go
   // After c.uiProgram.Run() returns
   if quitMsg.Export {
       filename, err := c.exportLogToFile()
       if err != nil {
           c.log.Error().Err(err).Msg("Failed to export logs")
       } else {
           c.log.Info().Str("file", filename).Msg("Session log exported")
       }
   }
   ```
   
   Note: This requires passing the quit result from the model back to the client. Implement via a channel or by accessing the model's final state.
  </action>
  <verify>
    grep -n "exportLogToFile\|requestLogEntry" internal/client/client.go
  </verify>
  <done>Log export function implemented, client can save logs to JSON file with timestamp</done>
</task>

</tasks>

<verification>
- [ ] SessionStats struct defined with RequestCount, ErrorCount, BytesIn, BytesOut, TotalLatency
- [ ] Statistics update on each RequestLogMsg (count, errors 400+, bytes, latency)
- [ ] Statistics line shows "req {N} err {N} bytes {in}/{out} avg {N}ms"
- [ ] Statistics display in gray color (lipgloss.Color("241"))
- [ ] 'c' key clears both logs AND statistics (resets to zero)
- [ ] requestLogEntry struct defined with all required JSON fields
- [ ] exportedLogs slice stores up to 100 entries in client
- [ ] exportLogToFile creates filename with format: remo-log-{subdomain}-{timestamp}.json
- [ ] Export file contains valid JSON array with all log entries
- [ ] Export triggered when QuitMsg has Export: true
- [ ] Export skipped when QuitMsg has Export: false
- [ ] Bytes displayed as integers (not scientific notation)
- [ ] Average latency displayed in milliseconds
</verification>

<success_criteria>
Statistics and export fully functional:
1. Statistics header displays real-time req/err/bytes/avg latency
2. Statistics update in real-time as requests arrive
3. Error count increments for status >= 400
4. Byte counters accumulate from BytesIn/BytesOut
5. Average latency calculated correctly from TotalLatency
6. 'c' clears both logs and statistics
7. Export creates valid JSON file with timestamped filename
8. Export includes all request fields (Time, Method, Path, Status, Latency, Remote, BytesIn, BytesOut)
9. Export only happens when user answers 'y' to prompt
10. Export filename format: remo-log-{subdomain}-{timestamp}.json
</success_criteria>

<output>
After completion, create `.planning/phases/02-tui-request-logging/02-tui-request-logging-05-SUMMARY.md`
</output>
