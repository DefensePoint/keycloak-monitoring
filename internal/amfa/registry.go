package amfa

import (
	"fmt"
	"sort"
	"sync"
)

// Registry holds one Repository per KMT tenant. Lookups are O(1) and
// concurrent-safe. Tenants without AMFA configured are simply absent.
type Registry struct {
	mu    sync.RWMutex
	repos map[string]Repository

	// changeMu serialises each change together with its notifications, so
	// listeners see changes in the order they were made. It is separate from
	// mu so lookups never wait for a listener.
	changeMu  sync.Mutex
	listeners []ChangeListener
}

// ChangeListener is told about each change to a tenant's repository: repo is
// the new repository, or nil when the tenant no longer has one.
type ChangeListener func(tenantID string, repo Repository)

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry {
	return &Registry{repos: make(map[string]Repository)}
}

// Register associates a Repository with a tenant ID, replacing any it had.
// Called at startup per tenant that has amfa.enabled = true, and again whenever
// a tenant saved through the API has AMFA configured.
func (r *Registry) Register(tenantID string, repo Repository) {
	r.changeMu.Lock()
	defer r.changeMu.Unlock()
	r.mu.Lock()
	r.repos[tenantID] = repo
	r.mu.Unlock()
	r.notify(tenantID, repo)
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

// Unregister removes a tenant's repository. Used when a tenant is deleted, has
// AMFA switched off, or is reconfigured, so a stale endpoint is never queried
// after the tenant stopped pointing at it.
func (r *Registry) Unregister(tenantID string) {
	r.changeMu.Lock()
	defer r.changeMu.Unlock()
	r.mu.Lock()
	_, had := r.repos[tenantID]
	delete(r.repos, tenantID)
	r.mu.Unlock()
	if had {
		r.notify(tenantID, nil)
	}
}

// Subscribe adds a listener for later changes. It is not told about the
// repositories already registered. A listener must not call Register,
// Unregister or Subscribe.
func (r *Registry) Subscribe(fn ChangeListener) {
	r.changeMu.Lock()
	defer r.changeMu.Unlock()
	r.listeners = append(r.listeners, fn)
}

// notify runs the listeners. Callers hold changeMu.
func (r *Registry) notify(tenantID string, repo Repository) {
	for _, fn := range r.listeners {
		fn(tenantID, repo)
	}
}
