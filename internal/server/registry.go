package server

import (
	"os/exec"
	"slices"
	"strings"
	"sync"
	"time"
)

// TunnelEntry represents an active tunnel with health tracking
type TunnelEntry struct {
	Subdomain string
	Port      int
	PubKey    string
	LastPing  time.Time
	CreatedAt time.Time
}

// registry manages active tunnels with automatic cleanup
type registry struct {
	mu      sync.RWMutex
	active  map[string]*TunnelEntry
	timeout time.Duration
}

// newRegistry creates a new registry with the specified timeout for stale tunnels
func newRegistry(timeout time.Duration) *registry {
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	return &registry{
		active:  make(map[string]*TunnelEntry),
		timeout: timeout,
	}
}

// register adds a new tunnel to the registry
func (r *registry) register(subdomain string, port int, pubKey string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.active[subdomain]; exists {
		return false
	}

	r.active[subdomain] = &TunnelEntry{
		Subdomain: subdomain,
		Port:      port,
		PubKey:    pubKey,
		LastPing:  time.Now(),
		CreatedAt: time.Now(),
	}
	return true
}

// unregister removes a tunnel from the registry
func (r *registry) unregister(subdomain string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.active, subdomain)
}

// ping updates the last ping time for a tunnel, returns false if tunnel doesn't exist
func (r *registry) ping(subdomain string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	entry, ok := r.active[subdomain]
	if !ok {
		return false
	}
	entry.LastPing = time.Now()
	return true
}

// get retrieves tunnel information
func (r *registry) get(subdomain string) (port int, pubKey string, exists bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	entry, ok := r.active[subdomain]
	if !ok {
		return 0, "", false
	}
	return entry.Port, entry.PubKey, true
}

// has checks if a tunnel exists
func (r *registry) has(subdomain string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.active[subdomain]
	return ok
}

// list returns all active subdomains
func (r *registry) list() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]string, 0, len(r.active))
	for subdomain := range r.active {
		result = append(result, subdomain)
	}
	slices.Sort(result)
	return result
}

// listStale returns subdomains that haven't pinged within the timeout
func (r *registry) listStale() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	now := time.Now()
	var stale []string
	for subdomain, entry := range r.active {
		if now.Sub(entry.LastPing) > r.timeout {
			stale = append(stale, subdomain)
		}
	}
	return stale
}

// cleanup removes stale tunnels and returns the count removed
func (r *registry) cleanup() int {
	stale := r.listStale()
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, subdomain := range stale {
		delete(r.active, subdomain)
	}
	return len(stale)
}

// getStats returns statistics about active tunnels
func (r *registry) getStats() (total int, stale int) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	now := time.Now()
	total = len(r.active)
	for _, entry := range r.active {
		if now.Sub(entry.LastPing) > r.timeout {
			stale++
		}
	}
	return total, stale
}

// listByPubKey returns all tunnels belonging to a specific public key
func (r *registry) listByPubKey(pubKey string) []TunnelEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []TunnelEntry
	for _, entry := range r.active {
		if entry.PubKey == pubKey {
			result = append(result, *entry)
		}
	}
	return result
}

// clearAll removes all tunnels from the registry and kills ALL remo-owned SSH processes
// Returns list of removed subdomains and executed kill commands
func (r *registry) clearAll(log func(string, ...interface{})) ([]string, []string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	removed := make([]string, 0, len(r.active))
	for subdomain := range r.active {
		removed = append(removed, subdomain)
		delete(r.active, subdomain)
	}

	// Kill ALL remo-owned sshd processes at once
	killCmd := r.killAllRemoSSH(log)
	killCommands := []string{}
	if killCmd != "" {
		killCommands = append(killCommands, killCmd)
	}

	return removed, killCommands
}

// killAllRemoSSH kills ALL sshd processes owned by the remo user
// This is the simplest approach - when remo stops, all remo SSH tunnels die
func (r *registry) killAllRemoSSH(log func(string, ...interface{})) string {
	// Use pgrep to find all sshd processes owned by remo
	cmd := exec.Command("pgrep", "-u", "remo", "-x", "sshd")

	output, err := cmd.Output()
	if err != nil {
		// No processes found or error
		return ""
	}

	pids := strings.Fields(string(output))
	if len(pids) == 0 {
		return ""
	}

	// Kill all PIDs at once
	args := []string{"-9"}
	args = append(args, pids...)
	killCmd := exec.Command("kill", args...)

	killErr := killCmd.Run()

	cmdStr := "pgrep -u remo -x sshd | xargs kill -9"

	if killErr != nil {
		if log != nil {
			log("Killed %d remo-owned sshd processes (PIDs: %s) - some may have failed: %v",
				len(pids), strings.Join(pids, ", "), killErr)
		}
		return cmdStr + " (PIDs: " + strings.Join(pids, ", ") + ", some FAILED: " + killErr.Error() + ")"
	}

	if log != nil {
		log("Killed all %d remo-owned sshd processes (PIDs: %s)", len(pids), strings.Join(pids, ", "))
	}
	return cmdStr + " (PIDs: " + strings.Join(pids, ", ") + ")"
}
