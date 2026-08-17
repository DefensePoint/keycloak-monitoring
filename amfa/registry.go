package amfa

import (
	"fmt"
	"sort"
	"sync"
)

// Registry holds one Repository per POINT tenant. Lookups are O(1) and
// concurrent-safe. Tenants without AMFA configured are simply absent.
type Registry struct {
	mu    sync.RWMutex
	repos map[string]Repository
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry {
	return &Registry{repos: make(map[string]Repository)}
}

// Register associates a Repository with a tenant ID. Called once at startup per
// tenant that has amfa.enabled = true.
func (r *Registry) Register(tenantID string, repo Repository) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.repos[tenantID] = repo
}

// RepositoryFor returns the repository for the given tenant. Returns
// ErrAmfaNotConfigured (wrapped) if the tenant has no AMFA configured.
func (r *Registry) RepositoryFor(tenantID string) (Repository, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	repo, ok := r.repos[tenantID]
	if !ok {
		return nil, fmt.Errorf("%w: tenant=%s", ErrAmfaNotConfigured, tenantID)
	}
	return repo, nil
}

// TenantIDs returns a sorted list of tenant IDs that have AMFA configured.
// Useful for startup logging and operator visibility.
func (r *Registry) TenantIDs() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := make([]string, 0, len(r.repos))
	for id := range r.repos {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
