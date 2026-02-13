package server

import (
	"slices"
	"sync"
)

type registry struct {
	mu     sync.RWMutex
	active map[string]*Tunnel
}

func newRegistry() *registry {
	return &registry{active: make(map[string]*Tunnel)}
}

func (r *registry) register(subdomain string, t *Tunnel) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.active[subdomain]; exists {
		return false
	}
	r.active[subdomain] = t
	return true
}

func (r *registry) unregister(subdomain string, t *Tunnel) {
	r.mu.Lock()
	defer r.mu.Unlock()
	current, ok := r.active[subdomain]
	if ok && current == t {
		delete(r.active, subdomain)
	}
}

func (r *registry) get(subdomain string) (*Tunnel, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.active[subdomain]
	return t, ok
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
