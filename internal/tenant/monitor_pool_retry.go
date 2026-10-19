package tenant

import (
	"context"
	"fmt"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

// A monitor that fails to start (typically because the tenant's Keycloak is
// unreachable) is retried in the background, first after defaultRetryInitial
// and then twice as long after each further failure, up to defaultRetryMax,
// for as long as the tenant stays enabled. Without this, a Keycloak outage
// during a deploy left the tenant unmonitored until the next restart.
const (
	defaultRetryInitial = 15 * time.Second
	defaultRetryMax     = 5 * time.Minute
)

// pendingStart is a tenant whose monitor failed to start and is due for a
// retry.
type pendingStart struct {
	attempts int // failed start attempts so far
	nextAt   time.Time
}

// retryBackoff is the wait after the given number of failed attempts: initial
// after the first, doubling after each further one, never more than max.
func retryBackoff(initial, max time.Duration, attempt int) time.Duration {
	d := initial
	for i := 1; i < attempt && d < max; i++ {
		d *= 2
	}
	if d > max {
		return max
	}
	return d
}

// startFailed handles a monitor that could not start: it raises the tenant's
// health alert, then schedules a retry. startMonitorForTenant already marks
// the tenant unhealthy, but no health check runs until the monitor starts, so
// a tenant unreachable at startup raised no health alert at all. Reporting it
// DOWN goes through the same alerting as a failed health check: one alert per
// outage, and the first successful check after a retry resolves it.
func (m *MonitorPoolManager) startFailed(ctx context.Context, tenant *domain.KeycloakTenant, cause error) {
	if m.healthAlertManager != nil {
		if err := m.healthAlertManager.ProcessHealthStatus(ctx, tenant.TenantID, tenant.Name, "DOWN", cause.Error(), 0); err != nil {
			m.logger.Warn("Failed to raise a health alert for a monitor that did not start",
				"tenant_id", tenant.TenantID, "error", err)
		}
	}
	m.scheduleRetry(tenant.TenantID, cause)
}

// closeHealthAlerts resolves a tenant's open health alerts once it is no
// longer monitored. A disabled or removed tenant runs no health checks, so an
// alert left open would stay open, with its GitLab issue, forever.
func (m *MonitorPoolManager) closeHealthAlerts(ctx context.Context, tenant *domain.KeycloakTenant, how string) {
	if m.healthAlertManager == nil {
		return
	}
	if err := m.healthAlertManager.TenantUnmonitored(ctx, tenant.TenantID, tenant.Name, how); err != nil {
		m.logger.Warn("Failed to resolve health alerts for a tenant no longer monitored",
			"tenant_id", tenant.TenantID, "error", err)
	}
}

// scheduleRetry records a failed start and when to try again.
func (m *MonitorPoolManager) scheduleRetry(tenantID string, cause error) {
	m.retryMu.Lock()
	p := m.pending[tenantID]
	if p == nil {
		p = &pendingStart{}
		m.pending[tenantID] = p
	}
	p.attempts++
	wait := retryBackoff(m.retryInitial, m.retryMax, p.attempts)
	p.nextAt = time.Now().Add(wait)
	attempts := p.attempts
	m.retryMu.Unlock()

	m.logger.Warn("Monitor start failed, will retry",
		"tenant_id", tenantID,
		"failed_attempts", attempts,
		"retry_in", wait.String(),
		"error", cause)
	m.wakeRetryLoop()
}

// cancelRetry forgets a pending retry, for a tenant that was disabled,
// removed, or started by other means.
func (m *MonitorPoolManager) cancelRetry(tenantID string) {
	m.retryMu.Lock()
	delete(m.pending, tenantID)
	m.retryMu.Unlock()
	m.wakeRetryLoop()
}

func (m *MonitorPoolManager) retryPending(tenantID string) bool {
	m.retryMu.Lock()
	defer m.retryMu.Unlock()
	_, ok := m.pending[tenantID]
	return ok
}

func (m *MonitorPoolManager) wakeRetryLoop() {
	select {
	case m.retryWake <- struct{}{}:
	default:
	}
}

// startRetryLoop runs the retry loop until stopRetryLoop.
func (m *MonitorPoolManager) startRetryLoop() {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	m.retryMu.Lock()
	m.retryCancel, m.retryDone = cancel, done
	m.retryMu.Unlock()
	go m.retryLoop(ctx, done)
}

// stopRetryLoop ends the retry loop, waiting for an attempt in progress, and
// drops every pending retry.
func (m *MonitorPoolManager) stopRetryLoop() {
	m.retryMu.Lock()
	cancel, done := m.retryCancel, m.retryDone
	m.retryCancel, m.retryDone = nil, nil
	m.pending = map[string]*pendingStart{}
	m.retryMu.Unlock()
	if cancel == nil {
		return
	}
	cancel()
	<-done
}

func (m *MonitorPoolManager) retryLoop(ctx context.Context, done chan struct{}) {
	defer close(done)
	for {
		var fire <-chan time.Time
		var timer *time.Timer
		if wait, ok := m.nextRetryIn(); ok {
			timer = time.NewTimer(wait)
			fire = timer.C
		}
		select {
		case <-ctx.Done():
			if timer != nil {
				timer.Stop()
			}
			return
		case <-m.retryWake:
		case <-fire:
			for _, tenantID := range m.dueRetries() {
				if ctx.Err() != nil {
					return
				}
				m.attemptRetry(ctx, tenantID)
			}
		}
		if timer != nil {
			timer.Stop()
		}
	}
}

// nextRetryIn is the wait until the earliest pending retry, if there is one.
func (m *MonitorPoolManager) nextRetryIn() (time.Duration, bool) {
	m.retryMu.Lock()
	defer m.retryMu.Unlock()
	var earliest time.Time
	for _, p := range m.pending {
		if earliest.IsZero() || p.nextAt.Before(earliest) {
			earliest = p.nextAt
		}
	}
	if earliest.IsZero() {
		return 0, false
	}
	if wait := time.Until(earliest); wait > 0 {
		return wait, true
	}
	return 0, true
}

func (m *MonitorPoolManager) dueRetries() []string {
	m.retryMu.Lock()
	defer m.retryMu.Unlock()
	now := time.Now()
	var due []string
	for id, p := range m.pending {
		if !p.nextAt.After(now) {
			due = append(due, id)
		}
	}
	return due
}

// attemptRetry tries to start one pending tenant's monitor again. It re-reads
// the tenant first, so a retry uses the current settings and never revives a
// tenant that was disabled or deleted meanwhile.
func (m *MonitorPoolManager) attemptRetry(ctx context.Context, tenantID string) {
	m.retryMu.Lock()
	p := m.pending[tenantID]
	m.retryMu.Unlock()
	if p == nil {
		return
	}

	tenant, err := m.service.GetTenant(ctx, tenantID)
	if err != nil {
		m.scheduleRetry(tenantID, fmt.Errorf("failed to load tenant: %w", err))
		return
	}
	if tenant == nil || !tenant.Enabled {
		m.cancelRetry(tenantID)
		m.logger.Info("Dropping monitor retry: tenant is no longer enabled",
			"tenant_id", tenantID)
		return
	}
	if _, running := m.monitors.Load(tenantID); running {
		m.cancelRetry(tenantID)
		return
	}

	startErr := m.startMonitorForTenant(ctx, tenant)

	m.retryMu.Lock()
	current := m.pending[tenantID] == p
	if startErr == nil && current {
		delete(m.pending, tenantID)
	}
	attempts := p.attempts
	m.retryMu.Unlock()

	switch {
	case startErr != nil:
		if _, running := m.monitors.Load(tenantID); running {
			// Started by an admin action while this attempt ran.
			m.cancelRetry(tenantID)
			return
		}
		if current {
			m.startFailed(ctx, tenant, startErr)
		}
	case !current:
		// The tenant changed while this attempt ran. If it was disabled or
		// removed, the monitor this attempt just started must not keep running.
		if t, err := m.service.GetTenant(ctx, tenantID); err == nil && (t == nil || !t.Enabled) {
			_ = m.stopMonitorForTenant(tenantID)
		}
	default:
		m.logger.Info("Monitor started after retry",
			"tenant_id", tenantID,
			"failed_attempts", attempts)
	}
}
