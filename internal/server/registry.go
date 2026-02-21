package server

import (
	"slices"
	"sync"
	"time"
)

// tunnelEntry represents an active tunnel with health tracking
type tunnelEntry struct {
	port      int
	pubKey    string
	lastPing  time.Time
	createdAt time.Time
}

// registry manages active tunnels with automatic cleanup
type registry struct {
	mu      sync.RWMutex
	active  map[string]*tunnelEntry
	timeout time.Duration
}

// newRegistry creates a new registry with the specified timeout for stale tunnels
func newRegistry(timeout time.Duration) *registry {
	if timeout <= 0 {
		timeout = 5 * time.Minute // Default 5 minute timeout
	}
	return &registry{
		active:  make(map[string]*tunnelEntry),
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
	r.active[subdomain] = &tunnelEntry{
		port:      port,
		pubKey:    pubKey,
		lastPing:  time.Now(),
		createdAt: time.Now(),
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
	entry.lastPing = time.Now()
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
	return entry.port, entry.pubKey, true
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
		if now.Sub(entry.lastPing) > r.timeout {
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
		if now.Sub(entry.lastPing) > r.timeout {
			stale++
		}
	}
	return total, stale
}
