---
phase: 02-tui-request-logging
plan: 03
type: execute
wave: 2
depends_on:
  - 02-tui-request-logging-01
files_modified:
  - internal/tui/model.go
  - internal/client/client.go
autonomous: true
requirements:
  - TUI-03
  - TUI-04
  - TUI-05

must_haves:
  truths:
    - 'q' key triggers graceful shutdown with export prompt
    - 'c' key clears the request log (already exists, verify)
    - Status codes are color-coded (2xx green, 3xx blue, 4xx yellow, 5xx red)
    - Long paths wrap to multiple lines instead of truncating
    - Export prompt asks to save log before exit
  artifacts:
    - path: internal/tui/model.go
      provides: Enhanced display with colors and quit handling
      contains: "case 'q'", "lipgloss.Color", path wrapping logic
    - path: internal/client/client.go
      provides: Quit signal handling and data export
      contains: "exportLogToFile", "QuitMsg"
  key_links:
    - from: internal/tui/model.go Update
      to: internal/client/client.go
      via: QuitMsg signal
      pattern: "case 'q':.*QuitMsg"
    - from: internal/tui/model.go View
      to: lipgloss styles
      via: Color-coded status rendering
      pattern: "statusColor\|lipgloss.Color"
---

<objective>
Enhance TUI with status code colors, path wrapping, quit behavior with export, and verify clear functionality.

Purpose: Complete the TUI dashboard with visual polish, proper quit handling with data export, and ensure all key bindings work as specified in locked decisions.
Output: TUI has color-coded status codes, multi-line paths, 'q' quits with export prompt, 'c' clears logs.
</objective>

<execution_context>
@/Users/gleicon/.config/opencode/get-shit-done/workflows/execute-plan.md
</execution_context>

<context>
@internal/tui/model.go
@internal/client/client.go

Current state:
- TUI already handles 'c' (clear), 'e' (errors), 'p' (pause), '/' (filter)
- Missing: 'q' (quit with export), status code colors, path wrapping
- RequestLogMsg has all required fields
- Help footer needs to show all keys: "q:quit c:clear e:errors p:pause /:filter"

Locked decisions:
- 'q' for immediate graceful shutdown with "Shutting down..." message
- Export prompt: "Export session log to file? (y/n)" with default filename remo-log-{subdomain}-{timestamp}.json
- Status colors: 2xx green, 3xx blue, 4xx yellow, 5xx red
- Multi-line path display (no truncation with ellipsis)
</context>

<tasks>

<task type="auto">
  <name>Add status code color helper and improve log display</name>
  <files>internal/tui/model.go</files>
  <action>
1. Add status code color helper function:
   ```go
   func statusColor(status int) lipgloss.Color {
       switch {
       case status >= 200 && status < 300:
           return lipgloss.Color("42") // Green
       case status >= 300 && status < 400:
           return lipgloss.Color("33") // Blue
       case status >= 400 && status < 500:
           return lipgloss.Color("220") // Yellow
       case status >= 500:
           return lipgloss.Color("196") // Red
       default:
           return lipgloss.Color("255") // White
       }
   }
   ```

2. Create path wrapping function for multi-line display:
   ```go
   func wrapPath(path string, maxWidth int) []string {
       if len(path) <= maxWidth {
           return []string{path}
       }
       var lines []string
       for len(path) > 0 {
           if len(path) <= maxWidth {
               lines = append(lines, path)
               break
           }
           lines = append(lines, path[:maxWidth])
           path = path[maxWidth:]
       }
       return lines
   }
   ```

3. Modify View() to use colors and wrapping:
   - Replace the simple Sprintf for log entries with styled rendering
   - Use statusColor(entry.Status) for status code styling
   - Use wrapPath() for path display (reserve ~40 chars for path)
   - Format: "HH:MM:SS | METHOD | path... | STATUS | latency"
   
   Example rendering:
   ```go
   statusStyle := lipgloss.NewStyle().Foreground(statusColor(entry.Status)).Bold(true)
   lines := wrapPath(entry.Path, 40)
   for i, line := range lines {
       if i == 0 {
           b.WriteString(fmt.Sprintf("  %s | %-4s %s %s | %s\n",
               entry.Time.Format("15:04:05"),
               entry.Method,
               line,
               statusStyle.Render(fmt.Sprintf("%3d", entry.Status)),
               formatDuration(entry.Latency)))
       } else {
           b.WriteString(fmt.Sprintf("  %s      %s\n", strings.Repeat(" ", 8), line))
       }
   }
   ```
  </action>
  <verify>grep -n "statusColor\|wrapPath" internal/tui/model.go</verify>
  <done>Status codes use color coding, paths wrap to multiple lines</done>
</task>

<task type="auto">
  <name>Add quit handling and export prompt to TUI</name>
  <files>internal/tui/model.go</files>
  <action>
Add quit functionality and export prompt state to the TUI model:

1. Add new message type and model fields:
   ```go
   type QuitMsg struct {
       Export bool
       FilePath string
   }
   
   // Add to Model struct:
   quitting     bool
   exportPrompt bool
   exportAnswer string
   ```

2. Add 'q' key handler in Update() (in the KeyMsg switch, before other keys):
   ```go
   case "q", "Q":
       if !m.quitting {
           m.quitting = true
           m.exportPrompt = true
           return m, nil
       }
   ```

3. Handle export prompt state (after the filtering check, before other key handling):
   ```go
   if m.exportPrompt {
       switch msg.String() {
       case "y", "Y":
           m.exportAnswer = "y"
           m.exportPrompt = false
           return m, func() tea.Msg { return QuitMsg{Export: true} }
       case "n", "N", "esc":
           m.exportPrompt = false
           return m, func() tea.Msg { return QuitMsg{Export: false} }
       }
       return m, nil
   }
   ```

4. Handle QuitMsg:
   ```go
   case QuitMsg:
       // Client will handle the actual shutdown
       m.quitting = true
       return m, tea.Quit
   ```

5. Update View() to show export prompt:
   Add at the beginning of View() before status line:
   ```go
   if m.quitting {
       if m.exportPrompt {
           return "Export session log to file? (y/n)\n"
       }
       return "Shutting down...\n"
   }
   ```
  </action>
  <verify>grep -n "QuitMsg\|exportPrompt\|quitting" internal/tui/model.go</verify>
  <done>TUI handles 'q' key, shows export prompt, and emits QuitMsg</done>
</task>

<task type="auto">
  <name>Add help footer and enhance TUI layout</name>
  <files>internal/tui/model.go</files>
  <action>
1. Add help footer at the bottom of View():
   ```go
   func (m Model) helpFooter() string {
       style := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
       return style.Render("q:quit c:clear e:errors p:pause /:filter")
   }
   ```

2. Modify View() to include help footer and use available terminal space:
   - Reserve 3 lines at top (status, URL, "Recent requests")
   - Reserve 1 line at bottom for help footer
   - Use remaining height for log display
   - Remove the artificial limit of 10 entries, use terminal height instead
   
   Calculate available lines:
   ```go
   headerLines := 3
   footerLines := 1
   filterLine := 0
   if m.filter != "" {
       filterLine = 1
   }
   availableLines := m.height - headerLines - footerLines - filterLine
   if availableLines < 5 {
       availableLines = 5 // Minimum
   }
   ```

3. Update the log rendering loop to use availableLines instead of hardcoded 10:
   ```go
   count := 0
   for i := len(m.logs) - 1; i >= 0 && count < availableLines; i-- {
       // ... existing filtering logic ...
       // Render entry (accounting for wrapped lines)
   }
   ```

4. Add help footer at the end:
   ```go
   b.WriteString("\n")
   b.WriteString(m.helpFooter())
   ```
  </action>
  <verify>grep -n "helpFooter\|availableLines" internal/tui/model.go</verify>
  <done>Help footer visible, log display uses available terminal space</done>
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

5. Handle QuitMsg from TUI in the client's event loop or add a channel to receive quit signals.
   The simplest approach: TUI sends QuitMsg and calls tea.Quit. Client should check ctx.Done() after tea.Quit.
   
   Actually, since tea.Quit stops the TUI program, we need to coordinate shutdown.
   Add a quitChan to Client:
   ```go
   quitChan chan struct{}
   ```
   
   Initialize in New().
   
   In Run(), after starting UI, wait for quit signal:
   ```go
   select {
   case <-ctx.Done():
       return ctx.Err()
   case <-c.quitChan:
       // Handle quit with optional export
   }
   ```
   
   Or simpler: Client's Run() returns when ctx is cancelled. The TUI will call Close() on the client when quitting.
   
   Actually, the cleanest approach: When TUI sends QuitMsg, it also sets a flag. The client should check if the TUI program has exited and handle export then.
   
   For now, implement export functionality and wire it up. The coordination can be:
   - TUI sends QuitMsg -> triggers tea.Quit
   - Client's uiProgram.Start() returns
   - Client then checks if export is needed
   
   Modify the UI goroutine in startUI():
   ```go
   go func() {
       if _, err := c.uiProgram.Run(); err != nil {
           c.log.Error().Err(err).Msg("tui exited")
       }
       // TUI exited, check if we need to export
       // This requires accessing model state... need to refactor
   }()
   ```
   
   Better approach: Add a callback or channel. For simplicity in this plan:
   - TUI's QuitMsg includes Export bool
   - Client creates a channel to receive quit signals
   - TUI sends to this channel (requires passing channel to model)
   
   Simpler: Just implement the export function now. The quit coordination can be handled in the model by the execute-plan agent based on context.
  </action>
  <verify>grep -n "exportLogToFile\|requestLogEntry" internal/client/client.go</verify>
  <done>Log export function implemented, client can save logs to JSON file</done>
</task>

</tasks>

<verification>
- [ ] 'q' key shows "Export session log to file? (y/n)" prompt
- [ ] 'y' exports to remo-log-{subdomain}-{timestamp}.json
- [ ] 'n' or 'esc' skips export
- [ ] After export decision, "Shutting down..." shown for 1-2 seconds
- [ ] Status codes have colors (2xx green, 3xx blue, 4xx yellow, 5xx red)
- [ ] Long paths wrap instead of truncate with ellipsis
- [ ] Help footer shows: "q:quit c:clear e:errors p:pause /:filter"
- [ ] Log display uses available terminal height
- [ ] 'c' key clears the log (verify existing functionality)
</verification>

<success_criteria>
TUI provides complete dashboard experience:
1. 'q' triggers graceful shutdown with export prompt
2. Status codes are color-coded per HTTP conventions
3. Long paths wrap to show full URL
4. Help footer visible with all key bindings
5. Log display fills available terminal space
6. Export creates valid JSON file with all request data
</success_criteria>

<output>
After completion, create `.planning/phases/02-tui-request-logging/02-tui-request-logging-03-SUMMARY.md`
</output>
