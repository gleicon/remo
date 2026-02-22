package tui

import (
	"fmt"
	"strings"
	"time"

	textinput "github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const maxLogs = 100

// Styles
type styles struct {
	header       lipgloss.Style
	headerURL    lipgloss.Style
	headerStats  lipgloss.Style
	tableHeader  lipgloss.Style
	tableRow     lipgloss.Style
	footer       lipgloss.Style
	status2xx    lipgloss.Style
	status3xx    lipgloss.Style
	status4xx    lipgloss.Style
	status5xx    lipgloss.Style
	connected    lipgloss.Style
	disconnected lipgloss.Style
	error        lipgloss.Style
	errorBanner  lipgloss.Style
	muted        lipgloss.Style
}

func makeStyles() styles {
	return styles{
		header: lipgloss.NewStyle().
			Background(lipgloss.Color("0")).
			Foreground(lipgloss.Color("15")).
			Padding(0, 1),
		headerURL: lipgloss.NewStyle().
			Foreground(lipgloss.Color("42")).
			Bold(true),
		headerStats: lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")),
		tableHeader: lipgloss.NewStyle().
			Background(lipgloss.Color("238")).
			Foreground(lipgloss.Color("15")).
			Bold(true).
			Padding(0, 1),
		tableRow: lipgloss.NewStyle().
			Padding(0, 1),
		footer: lipgloss.NewStyle().
			Background(lipgloss.Color("236")).
			Foreground(lipgloss.Color("250")).
			Padding(0, 1),
		status2xx: lipgloss.NewStyle().
			Foreground(lipgloss.Color("42")).
			Bold(true),
		status3xx: lipgloss.NewStyle().
			Foreground(lipgloss.Color("33")).
			Bold(true),
		status4xx: lipgloss.NewStyle().
			Foreground(lipgloss.Color("220")).
			Bold(true),
		status5xx: lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Bold(true),
		connected: lipgloss.NewStyle().
			Foreground(lipgloss.Color("42")).
			Bold(true),
		disconnected: lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Bold(true),
		error: lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")),
		errorBanner: lipgloss.NewStyle().
			Background(lipgloss.Color("196")).
			Foreground(lipgloss.Color("15")).
			Bold(true).
			Padding(0, 1),
		muted: lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")),
	}
}

// ViewType represents the current TUI view
type ViewType int

const (
	ViewLogs ViewType = iota
	ViewConnections
)

type Model struct {
	subdomain      string
	url            string
	connected      bool
	attempt        int
	backoff        time.Duration
	lastError      string
	logs           []RequestLogMsg
	width          int
	height         int
	paused         bool
	showErrorsOnly bool
	filter         string
	filtering      bool
	filterInput    textinput.Model
	stats          SessionStats
	quitting       bool
	exportPrompt   bool
	exportAnswer   string
	currentView    ViewType
	connections    []ConnectionEntry
	connSelected   int
	errorBanner    *ErrorBanner
}

// ErrorBanner displays inline error messages in the TUI
type ErrorBanner struct {
	Type       string // "no-upstream", "no-tunnel"
	Message    string
	Subdomain  string
	Time       time.Time
	StatusCode int
}

// ConnectionEntry represents a single connection in the connections view
type ConnectionEntry struct {
	Subdomain string
	Status    string
	Uptime    time.Duration
	Port      int
	CreatedAt time.Time
	LastPing  time.Time
}

type StateMsg struct {
	Connected bool
	Attempt   int
	Backoff   time.Duration
	Err       string
}

type URLMsg struct {
	URL string
}

type RequestLogMsg struct {
	Time     time.Time
	Method   string
	Path     string
	Status   int
	Latency  time.Duration
	Remote   string
	BytesIn  int
	BytesOut int
}

type QuitMsg struct {
	Export   bool
	FilePath string
}

// ConnectionsMsg updates the connections list in the TUI
type ConnectionsMsg struct {
	Connections []ConnectionEntry
}

// ErrorMsg displays an error banner in the TUI
type ErrorMsg struct {
	Type       string // "no-upstream", "no-tunnel"
	Message    string
	Subdomain  string
	StatusCode int
}

func NewModel(subdomain string) Model {
	input := textinput.New()
	input.Prompt = "/"
	input.CharLimit = 64
	input.Placeholder = "filter..."
	return Model{
		subdomain:    subdomain,
		filterInput:  input,
		currentView:  ViewLogs,
		connections:  []ConnectionEntry{},
		connSelected: 0,
	}
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case StateMsg:
		m.connected = msg.Connected
		m.attempt = msg.Attempt
		m.backoff = msg.Backoff
		m.lastError = msg.Err
	case URLMsg:
		m.url = msg.URL
	case RequestLogMsg:
		if !m.paused {
			m.logs = append(m.logs, msg)
			if len(m.logs) > maxLogs {
				m.logs = m.logs[len(m.logs)-maxLogs:]
			}
			m.stats.apply(msg)
		}
	case ConnectionsMsg:
		m.connections = msg.Connections
		if m.connSelected >= len(m.connections) {
			m.connSelected = max(0, len(m.connections)-1)
		}
	case ErrorMsg:
		m.errorBanner = &ErrorBanner{
			Type:       msg.Type,
			Message:    msg.Message,
			Subdomain:  msg.Subdomain,
			Time:       time.Now(),
			StatusCode: msg.StatusCode,
		}
	case QuitMsg:
		m.quitting = true
		return m, tea.Quit
	case tea.KeyMsg:
		// Dismiss error banner on any key press
		if m.errorBanner != nil {
			m.errorBanner = nil
			return m, nil
		}
		if m.filtering {
			var cmd tea.Cmd
			m.filterInput, cmd = m.filterInput.Update(msg)
			switch msg.Type {
			case tea.KeyEnter:
				m.filter = strings.TrimSpace(m.filterInput.Value())
				m.filtering = false
				m.filterInput.SetValue("")
			case tea.KeyEsc:
				m.filtering = false
				m.filterInput.SetValue("")
			}
			return m, cmd
		}
		if m.exportPrompt {
			switch msg.String() {
			case "y", "Y":
				m.exportAnswer = "y"
				m.exportPrompt = false
				return m, func() tea.Msg { return QuitMsg{Export: true} }
			case "n", "N":
				m.exportPrompt = false
				return m, func() tea.Msg { return QuitMsg{Export: false} }
			case "esc", "ctrl+c":
				// Cancel export and quit immediately
				m.exportPrompt = false
				return m, tea.Quit
			}
			return m, nil
		}

		switch msg.String() {
		case "q", "Q":
			if !m.quitting {
				m.quitting = true
				m.exportPrompt = true
				return m, nil
			}
		case "p", "P":
			m.paused = !m.paused
		case "c", "C":
			m.logs = nil
			m.stats = SessionStats{}
		case "e", "E":
			m.showErrorsOnly = !m.showErrorsOnly
		case "/":
			m.filtering = true
			m.filterInput.Focus()
		case "ctrl+c":
			// If export prompt is showing, cancel it
			if m.exportPrompt {
				m.exportPrompt = false
				m.quitting = false
				return m, tea.Quit
			}
			return m, tea.Quit
		case "tab":
			// Switch between Logs and Connections views
			if m.currentView == ViewLogs {
				m.currentView = ViewConnections
			} else {
				m.currentView = ViewLogs
			}
			m.connSelected = 0
		case "shift+tab":
			// Switch views in reverse
			if m.currentView == ViewLogs {
				m.currentView = ViewConnections
			} else {
				m.currentView = ViewLogs
			}
			m.connSelected = 0
		case "up", "k":
			if m.currentView == ViewConnections && m.connSelected > 0 {
				m.connSelected--
			}
		case "down", "j":
			if m.currentView == ViewConnections && m.connSelected < len(m.connections)-1 {
				m.connSelected++
			}
		}
	}
	return m, nil
}

func (m Model) View() string {
	if m.quitting {
		if m.exportPrompt {
			return "Export session logs to file? (y/n)"
		}
		return "Shutting down..."
	}

	s := makeStyles()

	// Calculate dimensions
	w := m.width
	if w < 80 {
		w = 80
	}

	// Build the view
	var sections []string

	// Header section
	sections = append(sections, m.renderHeader(s, w))

	// Error banner (if present)
	if m.errorBanner != nil {
		sections = append(sections, m.renderErrorBanner(s, w))
	}

	// View-specific content
	if m.currentView == ViewLogs {
		// Table header
		sections = append(sections, m.renderTableHeader(s, w))

		// Table rows
		availableHeight := m.height - 7 // header (3) + table header (1) + footer (1) + padding (2)
		if availableHeight < 3 {
			availableHeight = 3
		}
		sections = append(sections, m.renderTableRows(s, w, availableHeight))
	} else {
		// Connections view
		sections = append(sections, m.renderConnectionsHeader(s, w))

		availableHeight := m.height - 7
		if availableHeight < 3 {
			availableHeight = 3
		}
		sections = append(sections, m.renderConnectionsRows(s, w, availableHeight))
	}

	// Footer
	sections = append(sections, m.renderFooter(s, w))

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

func (m Model) renderErrorBanner(s styles, w int) string {
	if m.errorBanner == nil {
		return ""
	}

	errType := m.errorBanner.Type
	if errType == "" {
		errType = "error"
	}

	message := fmt.Sprintf("⚠ %s: %s (%d)",
		errType,
		m.errorBanner.Message,
		m.errorBanner.StatusCode)

	if m.errorBanner.Subdomain != "" {
		message += fmt.Sprintf(" | Subdomain: %s", m.errorBanner.Subdomain)
	}

	message += fmt.Sprintf(" | %s | Press any key", m.errorBanner.Time.Format("15:04:05"))

	return s.errorBanner.Width(w).Render(message)
}

func (m Model) renderHeader(s styles, w int) string {
	var lines []string

	// Line 1: Status indicator, subdomain, connection state, view indicator
	statusDot := "●"
	statusStyle := s.disconnected
	statusText := "disconnected"
	if m.connected {
		statusStyle = s.connected
		statusText = "connected"
	}

	viewText := "Logs"
	if m.currentView == ViewConnections {
		viewText = "Connections"
	}

	line1 := fmt.Sprintf("%s %s | %s | View: %s",
		statusStyle.Render(statusDot),
		lipgloss.NewStyle().Bold(true).Render(m.subdomain),
		statusStyle.Render(statusText),
		lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("33")).Render(viewText))

	if m.attempt > 0 {
		line1 += fmt.Sprintf(" | attempt %d", m.attempt)
	}
	if !m.connected && m.backoff > 0 {
		line1 += s.error.Render(fmt.Sprintf(" | retry in %s", formatDuration(m.backoff)))
	}
	if m.lastError != "" {
		line1 += s.error.Render(fmt.Sprintf(" | %s", truncate(m.lastError, 40)))
	}

	lines = append(lines, s.header.Width(w).Render(line1))

	// Line 2: URL
	if m.url != "" {
		lines = append(lines, s.headerURL.Render("→ "+m.url))
	}

	// Line 3: Stats
	avgLatency := time.Duration(0)
	if m.stats.RequestCount > 0 {
		avgLatency = m.stats.TotalLatency / time.Duration(m.stats.RequestCount)
	}

	statsText := fmt.Sprintf("Requests: %d | Errors: %d | Bytes In: %s | Bytes Out: %s | Latency: %s",
		m.stats.RequestCount,
		m.stats.ErrorCount,
		formatBytes(m.stats.BytesIn),
		formatBytes(m.stats.BytesOut),
		formatDuration(avgLatency))

	if m.paused {
		statsText += " | PAUSED"
	}
	if m.showErrorsOnly {
		statsText += " | ERRORS ONLY"
	}
	if m.filter != "" {
		statsText += fmt.Sprintf(" | Filter: %s", m.filter)
	}

	lines = append(lines, s.headerStats.Render(statsText))

	// Filter input line
	if m.filtering {
		lines = append(lines, m.filterInput.View())
	}

	return strings.Join(lines, "\n")
}

func (m Model) renderTableHeader(s styles, w int) string {
	// Calculate column widths based on available width
	colWidths := []int{10, 8, 35, 8, 10, 15}
	totalWidth := 0
	for _, cw := range colWidths {
		totalWidth += cw + 2 // +2 for padding
	}

	// Adjust path column if terminal is wider
	if w > totalWidth {
		colWidths[2] = w - (colWidths[0] + colWidths[1] + colWidths[3] + colWidths[4] + colWidths[5] + 12)
	}

	headers := []string{"Time", "Method", "Path", "Status", "Latency", "Remote"}
	var parts []string

	for i, h := range headers {
		parts = append(parts, s.tableHeader.Width(colWidths[i]).Render(h))
	}

	return lipgloss.JoinHorizontal(lipgloss.Left, parts...)
}

func (m Model) renderTableRows(s styles, w int, maxLines int) string {
	if len(m.logs) == 0 {
		if m.paused {
			return s.muted.Render("  -- Paused (press 'p' to resume) --")
		}
		return s.muted.Render("  -- Waiting for traffic... --")
	}

	// Calculate column widths
	colWidths := []int{10, 8, 35, 8, 10, 15}
	totalWidth := 0
	for _, cw := range colWidths {
		totalWidth += cw + 2
	}
	if w > totalWidth {
		colWidths[2] = w - (colWidths[0] + colWidths[1] + colWidths[3] + colWidths[4] + colWidths[5] + 12)
	}

	var lines []string
	count := 0

	// Iterate in reverse order (newest first)
	for i := len(m.logs) - 1; i >= 0 && count < maxLines; i-- {
		entry := m.logs[i]

		if m.showErrorsOnly && entry.Status < 400 {
			continue
		}
		if m.filter != "" && !strings.Contains(entry.Path, m.filter) {
			continue
		}

		row := m.renderRow(s, entry, colWidths)
		lines = append(lines, row)
		count++
	}

	return strings.Join(lines, "\n")
}

func (m Model) renderRow(s styles, entry RequestLogMsg, widths []int) string {
	// Status style based on code
	statusStr := fmt.Sprintf("%d", entry.Status)
	var statusStyled string
	switch {
	case entry.Status >= 200 && entry.Status < 300:
		statusStyled = s.status2xx.Render(statusStr)
	case entry.Status >= 300 && entry.Status < 400:
		statusStyled = s.status3xx.Render(statusStr)
	case entry.Status >= 400 && entry.Status < 500:
		statusStyled = s.status4xx.Render(statusStr)
	case entry.Status >= 500:
		statusStyled = s.status5xx.Render(statusStr)
	default:
		statusStyled = s.muted.Render(statusStr)
	}

	// Build row with proper spacing
	parts := []string{
		s.tableRow.Width(widths[0]).Render(entry.Time.Format("15:04:05")),
		s.tableRow.Width(widths[1]).Render(entry.Method),
		s.tableRow.Width(widths[2]).Render(truncate(entry.Path, widths[2])),
		s.tableRow.Width(widths[3]).Render(statusStyled),
		s.tableRow.Width(widths[4]).Render(formatDuration(entry.Latency)),
		s.tableRow.Width(widths[5]).Render(truncate(entry.Remote, widths[5])),
	}

	return lipgloss.JoinHorizontal(lipgloss.Left, parts...)
}

func (m Model) renderConnectionsHeader(s styles, w int) string {
	// Calculate column widths based on available width
	colWidths := []int{15, 10, 12, 10, 12}
	totalWidth := 0
	for _, cw := range colWidths {
		totalWidth += cw + 2 // +2 for padding
	}

	// Adjust columns if terminal is wider
	if w > totalWidth {
		colWidths[4] = w - (colWidths[0] + colWidths[1] + colWidths[2] + colWidths[3] + 10)
	}

	headers := []string{"Subdomain", "Status", "Uptime", "Port", "Action"}
	var parts []string

	for i, h := range headers {
		parts = append(parts, s.tableHeader.Width(colWidths[i]).Render(h))
	}

	return lipgloss.JoinHorizontal(lipgloss.Left, parts...)
}

func (m Model) renderConnectionsRows(s styles, w int, maxLines int) string {
	if len(m.connections) == 0 {
		return s.muted.Render("  -- No active connections --")
	}

	// Calculate column widths
	colWidths := []int{15, 10, 12, 10, 12}
	totalWidth := 0
	for _, cw := range colWidths {
		totalWidth += cw + 2
	}
	if w > totalWidth {
		colWidths[4] = w - (colWidths[0] + colWidths[1] + colWidths[2] + colWidths[3] + 10)
	}

	var lines []string
	count := 0

	for i, conn := range m.connections {
		if count >= maxLines {
			break
		}

		// Status indicator
		statusDot := "●"
		statusStyle := s.connected
		if conn.Status != "ON" {
			statusStyle = s.disconnected
		}

		actionText := "press x to kill"
		if i == m.connSelected {
			actionText = "> " + actionText
		}

		row := lipgloss.JoinHorizontal(lipgloss.Left,
			s.tableRow.Width(colWidths[0]).Render(conn.Subdomain),
			s.tableRow.Width(colWidths[1]).Render(statusStyle.Render(statusDot+" "+conn.Status)),
			s.tableRow.Width(colWidths[2]).Render(formatDuration(conn.Uptime)),
			s.tableRow.Width(colWidths[3]).Render(fmt.Sprintf("%d", conn.Port)),
			s.tableRow.Width(colWidths[4]).Render(actionText),
		)

		lines = append(lines, row)
		count++
	}

	return strings.Join(lines, "\n")
}

func (m Model) renderFooter(s styles, w int) string {
	var keys []string
	if m.currentView == ViewLogs {
		keys = []string{
			"tab:connections",
			"q:quit",
			"p:pause",
			"c:clear",
			"e:errors",
			"/:filter",
		}
	} else {
		keys = []string{
			"tab:logs",
			"↑/↓:navigate",
			"x:kill",
			"q:quit",
		}
	}

	return s.footer.Width(w).Render("  " + strings.Join(keys, "   "))
}

func truncate(input string, size int) string {
	if size <= 0 || len(input) <= size {
		return input
	}
	if size <= 3 {
		return input[:size]
	}
	return input[:size-3] + "..."
}

func formatDuration(d time.Duration) string {
	if d <= 0 {
		return "0ms"
	}
	if d < time.Millisecond {
		return fmt.Sprintf("%dµs", d.Microseconds())
	}
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.1fs", d.Seconds())
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

type SessionStats struct {
	RequestCount int
	ErrorCount   int
	BytesIn      int64
	BytesOut     int64
	TotalLatency time.Duration
}

func (s *SessionStats) apply(msg RequestLogMsg) {
	s.RequestCount++
	s.BytesIn += int64(max(0, msg.BytesIn))
	s.BytesOut += int64(max(0, msg.BytesOut))
	s.TotalLatency += msg.Latency
	if msg.Status >= 400 {
		s.ErrorCount++
	}
}

func max(a, b int) int {
	if a < b {
		return b
	}
	return a
}

func (m Model) ExportRequested() bool {
	return m.exportAnswer == "y"
}
