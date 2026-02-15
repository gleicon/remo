package client

import (
	"testing"
	"time"

	"github.com/gleicon/remo/internal/identity"
	"github.com/rs/zerolog"
	"io"
)

func testIdentity(t *testing.T) *identity.Identity {
	t.Helper()
	id, err := identity.Generate()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	return id
}

func TestNewClientRequiresIdentity(t *testing.T) {
	_, err := New(Config{
		Server:      "localhost",
		ServerPort:  22,
		Subdomain:   "foo",
		UpstreamURL: "http://localhost:3000",
		Logger:      zerolog.New(io.Discard),
	})
	if err == nil {
		t.Fatal("expected error without identity")
	}
}

func TestNewClientWithServer(t *testing.T) {
	_, err := New(Config{
		Server:      "localhost",
		ServerPort:  22,
		Subdomain:   "foo",
		UpstreamURL: "http://localhost:3000",
		Logger:      zerolog.New(io.Discard),
		Identity:    testIdentity(t),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewClientDefaults(t *testing.T) {
	c, err := New(Config{
		Server:      "localhost",
		ServerPort:  22,
		Subdomain:   "foo",
		UpstreamURL: "http://localhost:3000",
		Logger:      zerolog.New(io.Discard),
		Identity:    testIdentity(t),
	})
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	if c.reconnectMin != time.Second {
		t.Fatalf("expected 1s reconnect min, got %v", c.reconnectMin)
	}
	if c.reconnectMax != 30*time.Second {
		t.Fatalf("expected 30s reconnect max, got %v", c.reconnectMax)
	}
	if c.cfg.DialTimeout != 15*time.Second {
		t.Fatalf("expected 15s dial timeout, got %v", c.cfg.DialTimeout)
	}
}

func TestNewClientReconnectMaxClamped(t *testing.T) {
	c, err := New(Config{
		Server:       "localhost",
		ServerPort:   22,
		Subdomain:    "foo",
		UpstreamURL:  "http://localhost:3000",
		Logger:       zerolog.New(io.Discard),
		Identity:     testIdentity(t),
		ReconnectMin: 10 * time.Second,
		ReconnectMax: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	if c.reconnectMax != c.reconnectMin {
		t.Fatalf("max should be clamped to min: min=%v max=%v", c.reconnectMin, c.reconnectMax)
	}
}

func TestBackoffDuration(t *testing.T) {
	c, err := New(Config{
		Server:       "localhost",
		ServerPort:   22,
		Subdomain:    "foo",
		UpstreamURL:  "http://localhost:3000",
		Logger:       zerolog.New(io.Discard),
		Identity:     testIdentity(t),
		ReconnectMin: time.Second,
		ReconnectMax: 30 * time.Second,
	})
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	d1 := c.backoffDuration(1)
	if d1 < time.Second || d1 > 2*time.Second {
		t.Fatalf("attempt 1 backoff out of range: %v", d1)
	}
	d2 := c.backoffDuration(2)
	if d2 < 2*time.Second || d2 > 3*time.Second {
		t.Fatalf("attempt 2 backoff out of range: %v", d2)
	}
	d10 := c.backoffDuration(10)
	if d10 > 40*time.Second {
		t.Fatalf("attempt 10 should be capped near max: %v", d10)
	}
}

func TestBackoffDurationZeroAttempt(t *testing.T) {
	c, err := New(Config{
		Server:       "localhost",
		ServerPort:   22,
		Subdomain:    "foo",
		UpstreamURL:  "http://localhost:3000",
		Logger:       zerolog.New(io.Discard),
		Identity:     testIdentity(t),
		ReconnectMin: time.Second,
		ReconnectMax: 30 * time.Second,
	})
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	d := c.backoffDuration(0)
	if d < time.Second {
		t.Fatalf("zero attempt should use min: %v", d)
	}
}

func TestNewClientNoTUI(t *testing.T) {
	c, err := New(Config{
		Server:      "localhost",
		ServerPort:  22,
		Subdomain:   "foo",
		UpstreamURL: "http://localhost:3000",
		Logger:      zerolog.New(io.Discard),
		Identity:    testIdentity(t),
		EnableTUI:   false,
	})
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	if c.uiProgram != nil {
		t.Fatal("uiProgram should be nil when TUI is disabled")
	}
}

func TestSendUINoOp(t *testing.T) {
	c, err := New(Config{
		Server:      "localhost",
		ServerPort:  22,
		Subdomain:   "foo",
		UpstreamURL: "http://localhost:3000",
		Logger:      zerolog.New(io.Discard),
		Identity:    testIdentity(t),
	})
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	c.sendUI(nil)
}
