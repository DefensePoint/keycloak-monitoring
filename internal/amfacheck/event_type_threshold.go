package amfacheck

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/amfa"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

// eventTypeThresholdParams carries the per-rule differences for the shared
// window-threshold logic used by rules 4a and 4b.
type eventTypeThresholdParams struct {
	tenantID       string
	realmsFn       RealmsFunc
	repo           amfa.Repository
	eventType      string // AMFA event_type to count, e.g. "LOGIN_ERROR"
	threshold      int
	window         time.Duration
	checkType      string // GetCheckType() value
	idPrefix       string // AlertID prefix
	title          string
	recommendation string
}

// runEventTypeThresholdCheck counts events of a single type in a rolling window
// [now-window, now] per realm and emits one alert per realm whose count reaches
// the threshold. A rolling window (rather than a fixed clock-aligned bucket)
// means a recent burst is always counted regardless of when the check runs, so
// a manual trigger or a poll can't "miss" a burst that landed just before a
// bucket boundary. The AlertID is stable per (realm, eventType): repeated polls
// and manual runs dedup to one alert row (the service bumps LastSeen on
// recurrence), matching how eventscheck handles its error-threshold alerts.
func runEventTypeThresholdCheck(ctx context.Context, p eventTypeThresholdParams) ([]*domain.Alert, error) {
	realms, err := p.realmsFn(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: load realms: %w", p.checkType, err)
	}
	now := time.Now().UTC()
	start := now.Add(-p.window) // rolling window: always covers the last p.window

	var alerts []*domain.Alert
	for _, realm := range realms {
		count, err := p.repo.CountByEventTypeInWindow(ctx, realm, p.eventType, start, now)
		if err != nil {
			return nil, fmt.Errorf("%s: realm=%s: %w", p.checkType, realm, err)
		}
		if count < int64(p.threshold) {
			continue
		}
		md, _ := json.Marshal(map[string]any{
			"event_type":   p.eventType,
			"count":        count,
			"threshold":    p.threshold,
			"window":       p.window.String(),
			"window_start": start.Format(time.RFC3339),
			"realm_name":   realm,
		})
		alerts = append(alerts, &domain.Alert{
			TenantID:       p.tenantID,
			AlertID:        alertID(p.idPrefix, p.tenantID, realm),
			Source:         domain.AlertSourceEvent,
			Type:           domain.AlertTypeSecurity,
			Severity:       domain.AlertSeverityWarning,
			Status:         domain.AlertStatusActive,
			Title:          p.title,
			Description:    fmt.Sprintf("%d %s events in realm '%s' within %s (threshold: %d).", count, p.eventType, realm, p.window.String(), p.threshold),
			ResourceType:   "realm",
			ResourceID:     realm,
			ResourceName:   realm,
			RealmName:      realm,
			CheckType:      p.checkType,
			Recommendation: p.recommendation,
			Metadata:       string(md),
		})
	}
	return alerts, nil
}
