// Package eventscheck provides event-based security checking services.
package eventscheck

import (
	"time"
)

// CheckConfig contains common configuration for event checks.
type CheckConfig struct {
	TenantID   string
	Realms     []string
	Threshold  int
	TimeWindow time.Duration
}
