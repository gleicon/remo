package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Connection represents a single connection in the state file
type Connection struct {
	Subdomain string        `json:"subdomain"`
	PID       int           `json:"pid"`
	StartTime time.Time     `json:"start_time"`
	Port      int           `json:"port"`
	LastPing  time.Time     `json:"last_ping"`
	Status    string        `json:"status"`
	Uptime    time.Duration `json:"uptime"`
}

// State manages the connection state file at ~/.remo/state.json
type State struct {
	mu          sync.RWMutex
	path        string
	connections map[string]Connection
}

// DefaultPath returns the default state file path
func DefaultPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".remo", "state.json")
}

// New creates a new State manager
func New(path string) (*State, error) {
	if path == "" {
		path = DefaultPath()
	}

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create state directory: %w", err)
	}

	s := &State{
		path:        path,
		connections: make(map[string]Connection),
	}

	// Load existing state
	if err := s.load(); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("load state: %w", err)
	}

	return s, nil
}

// load reads the state from disk
func (s *State) load() error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return err
	}

	var conns []Connection
	if err := json.Unmarshal(data, &conns); err != nil {
		return err
	}

	s.connections = make(map[string]Connection)
	for _, c := range conns {
		s.connections[c.Subdomain] = c
	}

	return nil
}

// save writes the state to disk
func (s *State) save() error {
	conns := s.List()

	data, err := json.MarshalIndent(conns, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}

	if err := os.WriteFile(s.path, data, 0600); err != nil {
		return fmt.Errorf("write state file: %w", err)
	}

	return nil
}

// Add adds or updates a connection in the state
func (s *State) Add(conn Connection) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	conn.LastPing = time.Now()
	s.connections[conn.Subdomain] = conn

	return s.save()
}

// Remove removes a connection from the state
func (s *State) Remove(subdomain string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.connections, subdomain)

	return s.save()
}

// Get retrieves a connection by subdomain
func (s *State) Get(subdomain string) (Connection, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	conn, ok := s.connections[subdomain]
	return conn, ok
}

// List returns all connections
func (s *State) List() []Connection {
	s.mu.RLock()
	defer s.mu.RUnlock()

	conns := make([]Connection, 0, len(s.connections))
	for _, c := range s.connections {
		// Calculate uptime
		if c.Status == "ON" {
			c.Uptime = time.Since(c.StartTime)
		}
		conns = append(conns, c)
	}

	return conns
}

// Count returns the number of connections
func (s *State) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return len(s.connections)
}

// Clear removes all connections
func (s *State) Clear() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.connections = make(map[string]Connection)

	return s.save()
}
