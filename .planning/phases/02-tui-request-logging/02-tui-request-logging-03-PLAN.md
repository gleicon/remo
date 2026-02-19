---
phase: 02-tui-request-logging
plan: 03
type: execute
wave: 3
depends_on:
  - 02-tui-request-logging-02
files_modified:
  - internal/tui/model.go
autonomous: true
requirements:
  - TUI-03

must_haves:
  truths:
    - Status codes are color-coded (2xx green, 3xx blue, 4xx yellow, 5xx red)
    - Long paths wrap to multiple lines instead of truncating
    - Help footer shows: "q:quit c:clear e:errors p:pause /:filter"
    - Log display uses available terminal height dynamically
  artifacts:
    - path: internal/tui/model.go
      provides: Status code colors, path wrapping, help footer, dynamic height
      contains: "statusColor", "wrapPath", "helpFooter", "availableLines"
    - path: internal/tui/model.go
      provides: Dynamic log display based on terminal height
      contains: "m.height", "headerLines", "footerLines", "availableLines"
  key_links:
    - from: internal/tui/model.go View
      to: lipgloss styles
      via: Color-coded status rendering
      pattern: "statusColor\|lipgloss.Color"
---

<objective>
Enhance TUI with status code colors, multi-line path wrapping, help footer, and dynamic terminal height usage.

Purpose: Add visual polish to the TUI dashboard with color-coded HTTP status codes, proper path display for long URLs, and improve layout to use available terminal space efficiently.
Output: TUI displays color-coded status codes (green 2xx, blue 3xx, yellow 4xx, red 5xx), wraps long paths to multiple lines, shows help footer with all key bindings, and adapts log display to terminal size.
</objective>

<execution_context>
@/Users/gleicon/.config/opencode/get-shit-done/workflows/execute-plan.md
</execution_context>

<context>
@internal/tui/model.go

Current state:
- TUI handles basic log display with timestamp, method, path, status, latency
- Paths are truncated with ellipsis if too long
- Status codes displayed as plain numbers
- Help footer is missing
- Log display limited to hardcoded 10 entries

Locked decisions:
- Status colors: 2xx green, 3xx blue, 4xx yellow, 5xx red
- Multi-line path display (no truncation with ellipsis)
- Help footer: "q:quit c:clear e:errors p:pause /:filter"
- Reserve 3 lines for header (status, URL, "Recent requests")
- Reserve 1 line for help footer
- Use remaining terminal height for log display
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
  <verify>
    grep -n "statusColor\|wrapPath" internal/tui/model.go
  </verify>
  <done>Status codes use color coding, paths wrap to multiple lines</done>
</task>

<task type="auto">
  <name>Add help footer and enhance TUI layout with dynamic height</name>
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
  <verify>
    grep -n "helpFooter\|availableLines" internal/tui/model.go
  </verify>
  <done>Help footer visible, log display uses available terminal space</done>
</task>

</tasks>

<verification>
- [ ] Status codes 200-299 display in green
- [ ] Status codes 300-399 display in blue
- [ ] Status codes 400-499 display in yellow
- [ ] Status codes 500+ display in red
- [ ] Paths longer than 40 characters wrap to multiple lines
- [ ] Wrapped paths align under the original path position
- [ ] Help footer shows: "q:quit c:clear e:errors p:pause /:filter"
- [ ] Help footer uses dim/gray color (lipgloss.Color("241"))
- [ ] Log display adapts to terminal height (resize terminal to test)
- [ ] Minimum 5 log lines always displayed
- [ ] Header (3 lines) + footer (1 line) always visible
</verification>

<success_criteria>
TUI visual enhancements complete:
1. Status codes are color-coded per HTTP conventions (green 2xx, blue 3xx, yellow 4xx, red 5xx)
2. Long paths wrap to show full URL on multiple lines
3. Help footer visible with all key bindings in gray color
4. Log display fills available terminal space dynamically
5. Layout remains functional when terminal is resized
</success_criteria>

<output>
After completion, create `.planning/phases/02-tui-request-logging/02-tui-request-logging-03-SUMMARY.md`
</output>
