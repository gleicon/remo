package client

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ClientState represents the persistent state for a client connection
// Stores non-sensitive data only (no SSH keys or private information)
type ClientState struct {
	Subdomain string    `json:"subdomain"`
	PID       int       `json:"pid"`
	Port      int       `json:"port"`
	StartTime time.Time `json:"startTime"`
}

// StateManager handles reading and writing client state to disk with secure permissions
type StateManager struct {
	mu       sync.RWMutex
	path     string
	state    ClientState
	hasState bool
}

// defaultStatePath returns the default path for client state
func defaultStatePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".remo", "client-state.json")
}

// NewStateManager creates a new state manager
func NewStateManager(path string) (*StateManager, error) {
	if path == "" {
		path = defaultStatePath()
	}

	// Ensure directory exists with appropriate permissions
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return nil, fmt.Errorf("create state directory: %w", err)
	}

	return &StateManager{
		path: path,
	}, nil
}

// Load reads the state from disk
func (sm *StateManager) Load() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	data, err := os.ReadFile(sm.path)
	if err != nil {
		if os.IsNotExist(err) {
			// No state file yet, that's ok
			sm.hasState = false
			return nil
		}
		return fmt.Errorf("read state file: %w", err)
	}

	if err := json.Unmarshal(data, &sm.state); err != nil {
		return fmt.Errorf("parse state file: %w", err)
	}

	sm.hasState = true
	return nil
}

// Save writes the state to disk with 0600 permissions (owner only)
func (sm *StateManager) Save(state ClientState) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.state = state
	sm.hasState = true

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}

	// Write with 0600 permissions (owner read/write only)
	// This ensures no SSH keys or sensitive data is world-readable
	if err := os.WriteFile(sm.path, data, 0600); err != nil {
		return fmt.Errorf("write state file: %w", err)
	}

	return nil
}

// Get returns the current state
func (sm *StateManager) Get() (ClientState, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	return sm.state, sm.hasState
}

// Clear removes the state file
func (sm *StateManager) Clear() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.hasState = false
	sm.state = ClientState{}

	if err := os.Remove(sm.path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove state file: %w", err)
	}

	return nil
}

// Path returns the state file path
func (sm *StateManager) Path() string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	return sm.path
}
