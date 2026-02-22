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

// Color scheme - htop inspired
type colors struct {
	headerBg  lipgloss.Color
	headerFg  lipgloss.Color
	normal    lipgloss.Color
	selected  lipgloss.Color
	status2xx lipgloss.Color
	status3xx lipgloss.Color
	status4xx lipgloss.Color
	status5xx lipgloss.Color
	error     lipgloss.Color
	warning   lipgloss.Color
	success   lipgloss.Color
	muted     lipgloss.Color
	border    lipgloss.Color
}

var theme = colors{
	headerBg:  lipgloss.Color("0"),   // Black background
	headerFg:  lipgloss.Color("15"),  // White text
	normal:    lipgloss.Color("250"), // Light gray
	selected:  lipgloss.Color("33"),  // Blue
	status2xx: lipgloss.Color("42"),  // Green
	status3xx: lipgloss.Color("33"),  // Blue
	status4xx: lipgloss.Color("220"), // Yellow
	status5xx: lipgloss.Color("196"), // Red
	error:     lipgloss.Color("196"), // Red
	warning:   lipgloss.Color("220"), // Yellow
	success:   lipgloss.Color("42"),  // Green
	muted:     lipgloss.Color("240"), // Dark gray
	border:    lipgloss.Color("238"), // Gray border
}

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

func NewModel(subdomain string) Model {
	input := textinput.New()
	input.Prompt = "/"
	input.CharLimit = 64
	input.Placeholder = "filter..."
	return Model{subdomain: subdomain, filterInput: input}
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
	case QuitMsg:
		m.quitting = true
		return m, tea.Quit
	case tea.KeyMsg:
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
			case "n", "N", "esc":
				m.exportPrompt = false
				return m, func() tea.Msg { return QuitMsg{Export: false} }
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
			return m, tea.Quit
		}
	}
	return m, nil
}

func statusStyle(status int) lipgloss.Style {
	switch {
	case status >= 200 && status < 300:
		return lipgloss.NewStyle().Foreground(theme.status2xx).Bold(true)
	case status >= 300 && status < 400:
		return lipgloss.NewStyle().Foreground(theme.status3xx).Bold(true)
	case status >= 400 && status < 500:
		return lipgloss.NewStyle().Foreground(theme.status4xx).Bold(true)
	case status >= 500:
		return lipgloss.NewStyle().Foreground(theme.status5xx).Bold(true)
	default:
		return lipgloss.NewStyle().Foreground(theme.normal)
	}
}

func (m Model) View() string {
	if m.quitting {
		if m.exportPrompt {
			return m.renderExportPrompt()
		}
		return m.renderShutdown()
	}

	// Calculate available space
	headerHeight := 5
	footerHeight := 1
	tableHeaderHeight := 2
	availableHeight := m.height - headerHeight - footerHeight - tableHeaderHeight
	if availableHeight < 3 {
		availableHeight = 3
	}

	var sections []string
	sections = append(sections, m.renderHeader())
	sections = append(sections, m.renderTableHeader())
	sections = append(sections, m.renderLogs(availableHeight))
	sections = append(sections, m.renderFooter())

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

func (m Model) renderHeader() string {
	width := m.width
	if width < 80 {
		width = 80
	}

	// Status bar at top
	statusBarStyle := lipgloss.NewStyle().
		Background(theme.headerBg).
		Foreground(theme.headerFg).
		Padding(0, 1).
		Width(width)

	// Connection status
	statusText := "●"
	statusStyle := lipgloss.NewStyle().Foreground(theme.error).Bold(true)
	if m.connected {
		statusText = "●"
		statusStyle = lipgloss.NewStyle().Foreground(theme.success).Bold(true)
	}

	// Build status line
	var statusParts []string
	statusParts = append(statusParts, statusStyle.Render(statusText))
	statusParts = append(statusParts, lipgloss.NewStyle().Bold(true).Render(m.subdomain))

	if m.connected {
		statusParts = append(statusParts, lipgloss.NewStyle().Foreground(theme.success).Render("connected"))
	} else {
		statusParts = append(statusParts, lipgloss.NewStyle().Foreground(theme.error).Render("disconnected"))
	}

	if m.attempt > 0 {
		statusParts = append(statusParts, fmt.Sprintf("attempt %d", m.attempt))
	}
	if !m.connected && m.backoff > 0 {
		statusParts = append(statusParts, lipgloss.NewStyle().Foreground(theme.warning).Render(fmt.Sprintf("retry in %s", formatDuration(m.backoff))))
	}

	// Error indicator
	if m.lastError != "" {
		errText := truncate(m.lastError, 50)
		statusParts = append(statusParts, lipgloss.NewStyle().Foreground(theme.error).Render("⚠ "+errText))
	}

	// URL line
	var urlLine string
	if m.url != "" {
		urlLine = lipgloss.NewStyle().
			Foreground(theme.success).
			Bold(true).
			Render("→ " + m.url)
	}

	// Stats line
	avgLatency := time.Duration(0)
	if m.stats.RequestCount > 0 {
		avgLatency = m.stats.TotalLatency / time.Duration(m.stats.RequestCount)
	}

	statsStyle := lipgloss.NewStyle().Foreground(theme.muted)
	statsLine := fmt.Sprintf("Requests: %d | Errors: %d | Bytes In: %s | Bytes Out: %s | Latency: %s",
		m.stats.RequestCount,
		m.stats.ErrorCount,
		formatBytes(m.stats.BytesIn),
		formatBytes(m.stats.BytesOut),
		formatDuration(avgLatency),
	)

	// Filter indicator
	var filterLine string
	if m.filter != "" {
		filterLine = lipgloss.NewStyle().
			Foreground(theme.selected).
			Render(fmt.Sprintf("Filter: %s", m.filter))
	}
	if m.filtering {
		filterLine = m.filterInput.View()
	}

	// Combine all header lines
	var lines []string
	lines = append(lines, statusBarStyle.Render(strings.Join(statusParts, " | ")))
	if urlLine != "" {
		lines = append(lines, urlLine)
	}
	lines = append(lines, statsStyle.Render(statsLine))
	if filterLine != "" {
		lines = append(lines, filterLine)
	}

	return strings.Join(lines, "\n")
}

func (m Model) renderTableHeader() string {
	cols := []struct {
		name  string
		width int
	}{
		{"Time", 8},
		{"Method", 6},
		{"Path", 30},
		{"Status", 6},
		{"Latency", 8},
		{"Remote", 15},
	}

	var parts []string
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(theme.headerFg).Background(theme.headerBg)

	for _, col := range cols {
		parts = append(parts, headerStyle.Width(col.width).Render(col.name))
	}

	return lipgloss.JoinHorizontal(lipgloss.Left, parts...)
}

func (m Model) renderLogs(maxLines int) string {
	if len(m.logs) == 0 {
		if m.paused {
			return lipgloss.NewStyle().
				Foreground(theme.muted).
				Italic(true).
				Render("  -- Paused (press 'p' to resume) --")
		}
		return lipgloss.NewStyle().
			Foreground(theme.muted).
			Italic(true).
			Render("  -- Waiting for traffic... --")
	}

	var lines []string
	count := 0

	for i := len(m.logs) - 1; i >= 0 && count < maxLines; i-- {
		entry := m.logs[i]

		if m.showErrorsOnly && entry.Status < 400 {
			continue
		}
		if m.filter != "" && !strings.Contains(entry.Path, m.filter) {
			continue
		}

		line := m.renderLogEntry(entry)
		lines = append(lines, line)
		count++
	}

	return strings.Join(lines, "\n")
}

func (m Model) renderLogEntry(entry RequestLogMsg) string {
	cols := []struct {
		content string
		width   int
		style   lipgloss.Style
	}{
		{entry.Time.Format("15:04:05"), 8, lipgloss.NewStyle().Foreground(theme.muted)},
		{entry.Method, 6, lipgloss.NewStyle().Foreground(theme.normal).Bold(true)},
		{truncate(entry.Path, 30), 30, lipgloss.NewStyle().Foreground(theme.normal)},
		{fmt.Sprintf("%d", entry.Status), 6, statusStyle(entry.Status)},
		{formatDuration(entry.Latency), 8, lipgloss.NewStyle().Foreground(theme.muted)},
		{truncate(entry.Remote, 15), 15, lipgloss.NewStyle().Foreground(theme.muted)},
	}

	var parts []string
	for _, col := range cols {
		parts = append(parts, col.style.Width(col.width).Render(col.content))
	}

	return lipgloss.JoinHorizontal(lipgloss.Left, parts...)
}

func (m Model) renderFooter() string {
	width := m.width
	if width < 80 {
		width = 80
	}

	footerStyle := lipgloss.NewStyle().
		Background(theme.headerBg).
		Foreground(theme.headerFg).
		Width(width)

	var keys []string
	keys = append(keys, "q:quit")
	keys = append(keys, "p:pause")
	keys = append(keys, "c:clear")
	keys = append(keys, "e:errors")
	keys = append(keys, "/:filter")

	if m.paused {
		keys = append(keys, lipgloss.NewStyle().Foreground(theme.warning).Render("PAUSED"))
	}
	if m.showErrorsOnly {
		keys = append(keys, lipgloss.NewStyle().Foreground(theme.status4xx).Render("ERRORS ONLY"))
	}

	return footerStyle.Render("  " + strings.Join(keys, "  "))
}

func (m Model) renderExportPrompt() string {
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.selected).
		Padding(2).
		Width(50)

	content := "Export session logs to file?\n\n"
	content += lipgloss.NewStyle().Foreground(theme.muted).Render("(y) yes  (n) no")

	return boxStyle.Render(content)
}

func (m Model) renderShutdown() string {
	return lipgloss.NewStyle().
		Foreground(theme.muted).
		Italic(true).
		Render("Shutting down...")
}

// Helper functions

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

func (s *SessionStats) avgLatency() float64 {
	if s.RequestCount == 0 {
		return 0
	}
	return float64(s.TotalLatency.Microseconds()) / float64(s.RequestCount) / 1000
}

func max(a, b int) int {
	if a < b {
		return b
	}
	return a
}

// ExportRequested returns whether the user requested log export
func (m Model) ExportRequested() bool {
	return m.exportAnswer == "y"
}
