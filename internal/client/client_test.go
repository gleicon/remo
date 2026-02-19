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

func TestParsePortFromOutput(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantPort int
		wantErr  bool
	}{
		{
			name:     "OpenSSH 8.x format with debug prefix",
			input:    "debug1: Allocated port 12345 for remote forward to 127.0.0.1:18080",
			wantPort: 12345,
			wantErr:  false,
		},
		{
			name:     "Simple format without prefix",
			input:    "Allocated port 8080 for remote forward",
			wantPort: 8080,
			wantErr:  false,
		},
		{
			name:     "OpenSSH 9.x format with different host",
			input:    "debug1: Allocated port 9999 for remote forward to localhost:3000",
			wantPort: 9999,
			wantErr:  false,
		},
		{
			name:     "High port number",
			input:    "Allocated port 65000 for remote forward",
			wantPort: 65000,
			wantErr:  false,
		},
		{
			name:     "Minimum valid port",
			input:    "Allocated port 1 for remote forward",
			wantPort: 1,
			wantErr:  false,
		},
		{
			name:    "No port in output",
			input:   "some other debug output",
			wantErr: true,
		},
		{
			name:    "Invalid port format",
			input:   "Allocated port abc for remote forward",
			wantErr: true,
		},
		{
			name:    "Port zero is invalid",
			input:   "Allocated port 0 for remote forward",
			wantErr: true,
		},
		{
			name:    "Port too high",
			input:   "Allocated port 70000 for remote forward",
			wantErr: true,
		},
		{
			name:    "Negative port",
			input:   "Allocated port -1 for remote forward",
			wantErr: true,
		},
		{
			name:    "Empty string",
			input:   "",
			wantErr: true,
		},
		{
			name:     "Port embedded in longer line",
			input:    "some prefix Allocated port 54321 for remote forward to host:80 and suffix",
			wantPort: 54321,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPort, err := parsePortFromOutput(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parsePortFromOutput() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotPort != tt.wantPort {
				t.Errorf("parsePortFromOutput() = %v, want %v", gotPort, tt.wantPort)
			}
		})
	}
}

func TestUpstreamPort(t *testing.T) {
	tests := []struct {
		name     string
		upstream string
		want     string
	}{
		{
			name:     "HTTP with port",
			upstream: "http://localhost:18080",
			want:     "18080",
		},
		{
			name:     "HTTPS with port",
			upstream: "https://localhost:443",
			want:     "443",
		},
		{
			name:     "HTTP without port",
			upstream: "http://localhost",
			want:     "80",
		},
		{
			name:     "HTTPS without port",
			upstream: "https://localhost",
			want:     "443",
		},
		{
			name:     "Different port",
			upstream: "http://127.0.0.1:3000",
			want:     "3000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Client{
				upstream: tt.upstream,
			}
			got := c.upstreamPort()
			if got != tt.want {
				t.Errorf("upstreamPort() = %v, want %v", got, tt.want)
			}
		})
	}
}
