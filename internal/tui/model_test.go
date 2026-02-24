package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestModelStateUpdates(t *testing.T) {
	killCh := make(chan KillConnectionMsg)
	m := NewModel("demo", killCh)
	if _, ok := interface{}(m).(tea.Model); !ok {
		t.Fatalf("model does not implement tea.Model")
	}
	updated, _ := m.Update(StateMsg{Connected: true, Attempt: 3, Backoff: time.Second, Err: "boom"})
	m = updated.(Model)
	if !m.connected || m.attempt != 3 || m.backoff != time.Second || m.lastError != "boom" {
		t.Fatalf("state not applied: %#v", m)
	}
}

func TestModelLogsFilters(t *testing.T) {
	killCh := make(chan KillConnectionMsg)
	m := NewModel("demo", killCh)
	for i := 0; i < maxLogs; i++ {
		status := 200
		if i%2 == 0 {
			status = 500
		}
		msg := RequestLogMsg{Time: time.Unix(int64(i), 0), Method: "GET", Path: "/foo", Status: status}
		updated, _ := m.Update(msg)
		m = updated.(Model)
	}
	m.showErrorsOnly = true
	view := m.View()
	if !strings.Contains(view, "errors") {
		t.Fatalf("view missing errors indicator: %s", view)
	}
	m.showErrorsOnly = false
	m.filter = "bar"
	view = m.View()
	if strings.Contains(view, "/foo") {
		t.Fatalf("filter not applied")
	}
}

func TestModelPauseClear(t *testing.T) {
	killCh := make(chan KillConnectionMsg)
	m := NewModel("demo", killCh)
	m.paused = true
	updated, _ := m.Update(RequestLogMsg{Time: time.Now(), Method: "GET", Path: "/", Status: 200})
	m = updated.(Model)
	if len(m.logs) != 0 {
		t.Fatalf("logs should not append while paused")
	}
	m.paused = false
	updated, _ = m.Update(RequestLogMsg{Time: time.Now(), Method: "GET", Path: "/", Status: 200})
	m = updated.(Model)
	if len(m.logs) != 1 {
		t.Fatalf("log not appended")
	}
}
