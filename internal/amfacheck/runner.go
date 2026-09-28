package amfacheck

import (
	"context"
	"errors"
)

// ErrCheckerNotEnabled is returned when no AMFA checker is running for a tenant
// (the tenant has amfa.enabled=false, or the global amfa_checker switch is off).
var ErrCheckerNotEnabled = errors.New("amfacheck: checker not enabled for tenant")

// Runner exposes the per-tenant AMFA checker services for on-demand triggering.
// It is constructed once at startup with the same Service instances the fx
// lifecycle starts, so RunNow drives the exact same checks the ticker runs.
type Runner struct {
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
	_, ok := r.services[tenantID]
	return ok
}

// RunNow runs all of the tenant's checks once, synchronously, returning
// ErrCheckerNotEnabled if the tenant has no checker. It delegates to the
// tenant's Service.RunCheckNow - the same path the ticker uses.
func (r *Runner) RunNow(ctx context.Context, tenantID string) error {
	svc, ok := r.services[tenantID]
	if !ok {
		return ErrCheckerNotEnabled
	}
	return svc.RunCheckNow(ctx)
}

// Services returns the underlying services (used by the fx lifecycle starter).
func (r *Runner) Services() []Service {
	out := make([]Service, 0, len(r.services))
	for _, s := range r.services {
		out = append(out, s)
	}
	return out
}
