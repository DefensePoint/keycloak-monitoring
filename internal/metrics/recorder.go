package metrics

import "time"

// Recorder is the write side of the registry, consumed by domain packages that
// should not depend on the Prometheus client directly. A nil Recorder is not
// valid; use NopRecorder when metrics are disabled.
type Recorder interface {
	SetTenantMonitorUp(tenantID, tenantName string, up bool)
	SetTenantMonitorsActive(n int)
	ObservePollCycle(tenantID, pollType string, d time.Duration)
	IncPollCycleError(tenantID, pollType string)
	AddEventsCollected(tenantID, realm string, n int)
	IncAlert(severity, checkType string)
	IncNotification(channel, result string)
}

// SetTenantMonitorUp records whether a tenant's monitor is running.
func (m *Registry) SetTenantMonitorUp(tenantID, tenantName string, up bool) {
	v := 0.0
	if up {
		v = 1
	}
	m.TenantMonitorUp.WithLabelValues(tenantID, tenantName).Set(v)
}

// SetTenantMonitorsActive records the number of running monitors.
func (m *Registry) SetTenantMonitorsActive(n int) {
	m.TenantMonitorsActive.Set(float64(n))
}

// ObservePollCycle records the duration of one poll cycle.
func (m *Registry) ObservePollCycle(tenantID, pollType string, d time.Duration) {
	m.PollCycleDuration.WithLabelValues(tenantID, pollType).Observe(d.Seconds())
}

// IncPollCycleError records a failed poll cycle.
func (m *Registry) IncPollCycleError(tenantID, pollType string) {
	m.PollCycleErrors.WithLabelValues(tenantID, pollType).Inc()
}

// AddEventsCollected records how many events were collected for a realm.
func (m *Registry) AddEventsCollected(tenantID, realm string, n int) {
	if n <= 0 {
		return
	}
	m.EventsCollected.WithLabelValues(tenantID, realm).Add(float64(n))
}

// IncAlert records an alert by severity and check type.
func (m *Registry) IncAlert(severity, checkType string) {
	m.AlertsTotal.WithLabelValues(severity, checkType).Inc()
}

// IncNotification records a notification delivery attempt.
func (m *Registry) IncNotification(channel, result string) {
	m.NotificationsSent.WithLabelValues(channel, result).Inc()
}

// NopRecorder discards every observation. Used when metrics are disabled and in
// tests, so call sites never need a nil check.
type NopRecorder struct{}

func (NopRecorder) SetTenantMonitorUp(string, string, bool)        {}
func (NopRecorder) SetTenantMonitorsActive(int)                    {}
func (NopRecorder) ObservePollCycle(string, string, time.Duration) {}
func (NopRecorder) IncPollCycleError(string, string)               {}
func (NopRecorder) AddEventsCollected(string, string, int)         {}
func (NopRecorder) IncAlert(string, string)                        {}
func (NopRecorder) IncNotification(string, string)                 {}
