---
phase: 02-tui-request-logging
plan: 04
type: execute
wave: 3
depends_on:
  - 02-tui-request-logging-02
files_modified:
  - internal/tui/model.go
autonomous: true
requirements:
  - TUI-04
  - TUI-05

must_haves:
  truths:
    - 'q' key triggers graceful shutdown with export prompt
    - 'e' key toggles errors-only filter (4xx/5xx) with visual indicator in status bar showing "| errors only"
    - 'p' key pauses/resumes event polling with PAUSED indicator visible as red "[PAUSED]" text
    - Export prompt asks "Export session log to file? (y/n)" before shutdown
  artifacts:
    - path: internal/tui/model.go
      provides: Quit handling, error filtering, pause functionality
      contains: "case 'q'", "case 'e'", "case 'p'", "showErrorsOnly", "paused", "QuitMsg"
    - path: internal/tui/model.go
      provides: Visual indicators for filter and pause states
      contains: "| errors only", "[PAUSED]"
  key_links:
    - from: internal/tui/model.go Update
      to: internal/client/client.go
      via: QuitMsg signal
      pattern: "case 'q':.*QuitMsg"
    - from: internal/tui/model.go View
      to: status bar display
      via: Error filter indicator
      pattern: "errors only"
    - from: internal/tui/model.go View
      to: status bar display
      via: Pause indicator
      pattern: "\[PAUSED\]"
---

<objective>
Implement keyboard controls for quit with export prompt, error-only filtering, and pause functionality with clear visual indicators.

Purpose: Add interactive keyboard controls to the TUI allowing users to gracefully quit with optional data export, filter to show only error responses, and pause/resume event polling.
Output: TUI responds to 'q' (quit with export prompt), 'e' (toggle error filter with "| errors only" indicator), and 'p' (pause/resume with "[PAUSED]" indicator).
</objective>

<execution_context>
@/Users/gleicon/.config/opencode/get-shit-done/workflows/execute-plan.md
</execution_context>

<context>
@internal/tui/model.go

Current state:
- TUI already handles 'c' (clear), '/' (filter)
- Missing: 'q' (quit with export), 'e' (errors-only filter), 'p' (pause)
- Need visual indicators in status bar for active filters and pause state

Locked decisions:
- 'q' for immediate graceful shutdown with "Export session log to file? (y/n)" prompt
- 'e' toggles errors-only filter showing 4xx/5xx with "| errors only" indicator in status bar
- 'p' pauses/resumes event polling with red "[PAUSED]" indicator visible in status bar
- 'y' exports to remo-log-{subdomain}-{timestamp}.json
- 'n' or 'esc' skips export
- QuitMsg signal sent to client for coordination
</context>

<tasks>

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
  <verify>
    grep -n "QuitMsg\|exportPrompt\|quitting" internal/tui/model.go
  </verify>
  <done>TUI handles 'q' key, shows export prompt, and emits QuitMsg</done>
</task>

<task type="auto">
  <name>Add 'e' key error filtering functionality</name>
  <files>internal/tui/model.go</files>
  <action>
1. Add error filter state to Model struct:
   ```go
   showErrorsOnly bool
   ```

2. Add 'e' key handler in Update() (in KeyMsg switch):
   ```go
   case "e", "E":
       m.showErrorsOnly = !m.showErrorsOnly
       return m, nil
   ```

3. Create filter predicate function:
   ```go
   func (m Model) shouldShowEntry(entry RequestLogEntry) bool {
       if m.filter != "" && !strings.Contains(entry.Path, m.filter) {
           return false
       }
       if m.showErrorsOnly && entry.Status < 400 {
           return false
       }
       return true
   }
   ```

4. Update View() to show error filter indicator in status bar:
   Modify the status line rendering to include filter state:
   ```go
   statusText := fmt.Sprintf("Connected to https://%s.remo.dev | %d requests", m.subdomain, len(m.logs))
   if m.showErrorsOnly {
       statusText += " | errors only"
   }
   if m.filter != "" {
       statusText += fmt.Sprintf(" | filter: %s", m.filter)
   }
   ```

5. Update log rendering loop to use shouldShowEntry():
   Replace the filtering check in the View() log loop with:
   ```go
   if !m.shouldShowEntry(entry) {
       continue
   }
   ```
  </action>
  <verify>
    grep -n "showErrorsOnly\|shouldShowEntry" internal/tui/model.go && \
    grep -n "errors only" internal/tui/model.go
  </verify>
  <done>'e' key toggles error filter, shows "| errors only" indicator, filters to 4xx/5xx</done>
</task>

<task type="auto">
  <name>Add 'p' key pause functionality</name>
  <files>internal/tui/model.go</files>
  <action>
1. Add pause state to Model struct:
   ```go
   paused bool
   ```

2. Add 'p' key handler in Update() (in KeyMsg switch):
   ```go
   case "p", "P":
       m.paused = !m.paused
       return m, nil
   ```

3. Modify RequestLogMsg handling to respect pause state:
   ```go
   case RequestLogMsg:
       if m.paused {
           // Drop message when paused - don't add to logs or stats
           return m, nil
       }
       // ... existing log/stat update logic ...
   ```

4. Update View() to show PAUSED indicator:
   Modify status line to show pause state:
   ```go
   statusText := fmt.Sprintf("Connected to https://%s.remo.dev | %d requests", m.subdomain, len(m.logs))
   if m.paused {
       statusStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
       statusText += " " + statusStyle.Render("[PAUSED]")
   }
   if m.showErrorsOnly {
       statusText += " | errors only"
   }
   ```

5. Optional: Add visual feedback in log area when paused:
   When paused and no logs to show, display pause message:
   ```go
   if m.paused && len(visibleLogs) == 0 {
       pauseStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Italic(true)
       b.WriteString(pauseStyle.Render("  -- Polling paused, press 'p' to resume --"))
       b.WriteString("\n")
   }
   ```
  </action>
  <verify>
    grep -n "paused" internal/tui/model.go && \
    grep -n "\[PAUSED\]" internal/tui/model.go
  </verify>
  <done>'p' key pauses/resumes polling, shows red "[PAUSED]" indicator in status bar</done>
</task>

</tasks>

<verification>
- [ ] 'q' key shows "Export session log to file? (y/n)" prompt
- [ ] 'y' confirms export and sends QuitMsg with Export: true
- [ ] 'n' or 'esc' skips export and sends QuitMsg with Export: false
- [ ] After export decision, "Shutting down..." shown briefly
- [ ] QuitMsg type defined with Export bool field
- [ ] 'e' key toggles errors-only filter (4xx/5xx responses only)
- [ ] Status bar shows "| errors only" text when filter is active
- [ ] 'e' again turns off filter and removes "| errors only" indicator
- [ ] 'p' key pauses event polling (new requests not added to logs)
- [ ] Status bar shows red "[PAUSED]" indicator when paused
- [ ] 'p' again resumes polling and removes "[PAUSED]" indicator
- [ ] Paused state prevents RequestLogMsg from updating logs
- [ ] showErrorsOnly and paused fields added to Model struct
</verification>

<success_criteria>
Keyboard controls fully functional:
1. 'q' triggers graceful shutdown with export prompt
2. 'y'/'n' handle export decision properly
3. QuitMsg signal properly communicates with client
4. 'e' toggles errors-only filter (4xx/5xx) with "| errors only" visual indicator
5. 'p' pauses/resumes event polling with red "[PAUSED]" visual indicator
6. Both filters can be active simultaneously
7. Visual indicators appear in status bar next to request count
</success_criteria>

<output>
After completion, create `.planning/phases/02-tui-request-logging/02-tui-request-logging-04-SUMMARY.md`
</output>
