package server

import (
	"slices"
	"sync"
)

type tunnelEntry struct {
	port   int
	pubKey string
}

type registry struct {
	mu     sync.RWMutex
	active map[string]*tunnelEntry
}

func newRegistry() *registry {
	return &registry{active: make(map[string]*tunnelEntry)}
}

func (r *registry) register(subdomain string, port int, pubKey string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.active[subdomain]; exists {
		return false
	}
	r.active[subdomain] = &tunnelEntry{port: port, pubKey: pubKey}
	return true
}

func (r *registry) unregister(subdomain string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.active, subdomain)
}

func (r *registry) get(subdomain string) (int, string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	entry, ok := r.active[subdomain]
	if !ok {
		return 0, "", false
	}
	return entry.port, entry.pubKey, true
}

func (r *registry) has(subdomain string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.active[subdomain]
	return ok
}

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
