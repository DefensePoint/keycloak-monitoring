package amfacheck

import (
	"context"
	"errors"
	"sync"
)

// ErrCheckerNotEnabled is returned when no AMFA checker is running for a tenant
// (the tenant has amfa.enabled=false, or the global amfa_checker switch is off).
var ErrCheckerNotEnabled = errors.New("amfacheck: checker not enabled for tenant")

// Runner exposes the per-tenant AMFA checker services for on-demand triggering.
// It holds the same Service instances the fx lifecycle starts, so RunNow drives
// the exact same checks the ticker runs. Set and Remove keep it in step as
// tenants are added, changed or removed while the platform runs.
type Runner struct {
	mu       sync.RWMutex
	services map[string]Service // tenantID -> Service
}

// NewRunner wraps the per-tenant service map. A nil map is tolerated (no tenant
// has a checker), so the Runner is always safe to construct and inject.
func NewRunner(services map[string]Service) *Runner {
	if services == nil {
		services = map[string]Service{}
	}
	return &Runner{services: services}
}

// Enabled reports whether a checker is running for the tenant.
func (r *Runner) Enabled(tenantID string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.services[tenantID]
	return ok
}

// RunNow runs all of the tenant's checks once, synchronously, returning
// ErrCheckerNotEnabled if the tenant has no checker. It delegates to the
// tenant's Service.RunCheckNow - the same path the ticker uses.
func (r *Runner) RunNow(ctx context.Context, tenantID string) error {
	r.mu.RLock()
	svc, ok := r.services[tenantID]
	r.mu.RUnlock()
	if !ok {
		return ErrCheckerNotEnabled
	}
	return svc.RunCheckNow(ctx)
}

// Set makes svc the tenant's checker, replacing any it had.
func (r *Runner) Set(tenantID string, svc Service) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.services[tenantID] = svc
}

// Remove drops the tenant's checker, if it has one.
func (r *Runner) Remove(tenantID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.services, tenantID)
}

// Services returns the underlying services (used by the fx lifecycle starter).
func (r *Runner) Services() []Service {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Service, 0, len(r.services))
	for _, s := range r.services {
		out = append(out, s)
	}
	return out
}
