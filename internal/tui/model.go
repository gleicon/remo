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
		// Client will handle the actual shutdown
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
		// Handle export prompt first
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
		case "e", "E":
			m.showErrorsOnly = !m.showErrorsOnly
		case "/":
			m.filtering = true
			m.filterInput.Focus()
		}
	}
	return m, nil
}

func (m Model) shouldShowEntry(entry RequestLogMsg) bool {
	if m.filter != "" && !strings.Contains(entry.Path, m.filter) {
		return false
	}
	if m.showErrorsOnly && entry.Status < 400 {
		return false
	}
	return true
}

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

func (m Model) helpFooter() string {
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	return style.Render("q:quit c:clear e:errors p:pause /:filter")
}

func (m Model) View() string {
	// Handle quitting state first
	if m.quitting {
		if m.exportPrompt {
			return "Export session log to file? (y/n)\n"
		}
		return "Shutting down...\n"
	}

	var b strings.Builder
	status := lipgloss.NewStyle().Bold(true)
	url := lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)
	stateText := "disconnected"
	if m.connected {
		stateText = "connected"
	}
	statusLine := fmt.Sprintf("remo %s | %s | attempt %d", m.subdomain, stateText, m.attempt)
	if !m.connected && m.backoff > 0 {
		statusLine += fmt.Sprintf(" | next retry in %s", formatDuration(m.backoff))
	}
	if m.lastError != "" {
		statusLine += fmt.Sprintf(" | last error: %s", m.lastError)
	}
	if m.paused {
		statusLine += " | paused"
	}
	if m.showErrorsOnly {
		statusLine += " | errors only"
	}
	statusLine += fmt.Sprintf(" | req %d err %d bytes %d/%d avg %.1fms", m.stats.requests, m.stats.errors, m.stats.bytesIn, m.stats.bytesOut, m.stats.avgLatency())
	b.WriteString(status.Render(statusLine))
	b.WriteString("\n")
	if m.url != "" {
		b.WriteString(url.Render("→ " + m.url + "\n"))
	}
	b.WriteString("Recent requests\n")
	if m.filter != "" {
		b.WriteString(fmt.Sprintf("filter: %s\n", m.filter))
	}
	if m.filtering {
		b.WriteString(m.filterInput.View())
		b.WriteString("\n")
	}
	if len(m.logs) == 0 {
		b.WriteString("  waiting for traffic...\n")
		b.WriteString("\n")
		b.WriteString(m.helpFooter())
		return b.String()
	}

	// Calculate available lines for logs
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

	count := 0
	for i := len(m.logs) - 1; i >= 0 && count < availableLines; i-- {
		entry := m.logs[i]
		if !m.shouldShowEntry(entry) {
			continue
		}

		statusStyle := lipgloss.NewStyle().Foreground(statusColor(entry.Status)).Bold(true)
		lines := wrapPath(entry.Path, 40)
		for j, line := range lines {
			if j == 0 {
				b.WriteString(fmt.Sprintf("  %s | %-4s %-40s %s | %s\n",
					entry.Time.Format("15:04:05"),
					entry.Method,
					line,
					statusStyle.Render(fmt.Sprintf("%3d", entry.Status)),
					formatDuration(entry.Latency)))
			} else {
				b.WriteString(fmt.Sprintf("  %s      %s\n", strings.Repeat(" ", 8), line))
			}
		}
		count++
	}
	b.WriteString("\n")
	b.WriteString(m.helpFooter())
	return b.String()
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
		return "0s"
	}
	if d < time.Millisecond {
		return fmt.Sprintf("%dµs", d.Microseconds())
	}
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return d.Truncate(100 * time.Millisecond).String()
}

type stats struct {
	requests uint64
	errors   uint64
	bytesIn  uint64
	bytesOut uint64
	latency  time.Duration
}

func (s *stats) apply(msg RequestLogMsg) {
	s.requests++
	s.bytesIn += uint64(max(0, msg.BytesIn))
	s.bytesOut += uint64(max(0, msg.BytesOut))
	s.latency += msg.Latency
	if msg.Status >= 400 {
		s.errors++
	}
}

func (s *stats) avgLatency() float64 {
	if s.requests == 0 {
		return 0
	}
	return float64(s.latency.Microseconds()) / float64(s.requests) / 1000
}

func max(a, b int) int {
	if a < b {
		return b
	}
	return a
}
