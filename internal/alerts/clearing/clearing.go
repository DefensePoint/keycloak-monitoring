// Package clearing resolves alerts whose condition has cleared.
//
// The threshold checks in eventscheck and amfacheck raise an alert while a
// condition holds, "N errors in the last few minutes", and used to leave it
// active forever once it stopped: only a person could clear the list, and an
// alert that never resolved kept qualifying for re-notification long after the
// incident. A check that implements an Evaluate method reports not only what
// it detected but which conditions it actually evaluated, and the checker
// services use this package to resolve the open alerts whose condition was
// evaluated and not detected. The configuration checks in configcheck use it
// the same way for misconfigurations that have been fixed.
//
// "Evaluated" is the safety of it. A check that could not read a realm, during
// the very outage an alert is about, did not see the condition clear, so its
// alerts there are left alone.
package clearing

import (
	"context"
	"fmt"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

// DefaultResolveAfter is how long a condition must go undetected before its
// alert resolves, when the checker's config does not say. Three polls at the
// default five-minute interval, so one quiet poll in a noisy realm does not
// resolve an alert only to reopen it on the next.
const DefaultResolveAfter = 15 * time.Minute

// Evaluation is what one run of a check found.
type Evaluation struct {
	// TenantID is the tenant the check ran for.
	TenantID string
	// Alerts are the conditions detected in this run.
	Alerts []*domain.Alert
	// Evaluated holds the alert ID of every condition the run evaluated,
	// detected or not. A condition whose data could not be read is absent.
	Evaluated map[string]bool
	// EvaluatedRealms holds every realm whose conditions the run evaluated in
	// full. A per-user or per-client check only hears back about the users or
	// clients over its threshold, so it cannot name the ones that dropped
	// below it; it marks the realm instead, and every open alert of the check
	// in that realm counts as evaluated.
	EvaluatedRealms map[string]bool
	// Unread holds the resources, by realm and resource name, whose data the
	// run could not read although it evaluated the rest of their realm, such
	// as an identity provider whose metadata could not be fetched. Their
	// alerts count as not evaluated, whatever else the run marked.
	Unread map[ResourceRef]bool
}

// ResourceRef names a resource within a realm, as an alert's RealmName and
// ResourceName do.
type ResourceRef struct {
	Realm    string
	Resource string
}

// MarkEvaluated records that the run evaluated the condition behind alertID.
func (e *Evaluation) MarkEvaluated(alertID string) {
	if e.Evaluated == nil {
		e.Evaluated = map[string]bool{}
	}
	e.Evaluated[alertID] = true
}

// MarkRealmEvaluated records that the run evaluated every condition in realm.
func (e *Evaluation) MarkRealmEvaluated(realm string) {
	if e.EvaluatedRealms == nil {
		e.EvaluatedRealms = map[string]bool{}
	}
	e.EvaluatedRealms[realm] = true
}

// MarkUnread records that the run could not read resource in realm, so its
// alerts are left alone even when the realm is marked evaluated.
func (e *Evaluation) MarkUnread(realm, resource string) {
	if e.Unread == nil {
		e.Unread = map[ResourceRef]bool{}
	}
	e.Unread[ResourceRef{Realm: realm, Resource: resource}] = true
}

// Covers reports whether the run evaluated the condition behind alert.
func (e *Evaluation) Covers(alert *domain.Alert) bool {
	return e.evaluated(alert)
}

// evaluated reports whether the run evaluated the condition behind alert.
func (e *Evaluation) evaluated(alert *domain.Alert) bool {
	if e.Unread[ResourceRef{Realm: alert.RealmName, Resource: alert.ResourceName}] {
		return false
	}
	return e.Evaluated[alert.AlertID] || (alert.RealmName != "" && e.EvaluatedRealms[alert.RealmName])
}

// Store is the alert storage clearing needs.
type Store interface {
	ListOpenAlertsByCheck(ctx context.Context, tenantID, checkType string) ([]*domain.Alert, error)
	ResolveAlert(ctx context.Context, tenantID, alertID string) error
}

// Notifier sends the notification that an alert resolved.
type Notifier interface {
	NotifyAlertResolution(ctx context.Context, alert *domain.Alert) error
}

// Logger is the logging clearing needs; both checker packages' loggers have it.
type Logger interface {
	Info(msg string, args ...any)
	Error(msg string, args ...any)
}

// Reopens reports whether detecting existing again starts a new occurrence: it
// was resolved and had gone undetected for at least resolveAfter. One resolved
// by hand while the condition still held keeps being detected, so it stays
// resolved until the condition has actually cleared and come back.
func Reopens(existing *domain.Alert, now time.Time, resolveAfter time.Duration) bool {
	return existing != nil &&
		existing.Status == domain.AlertStatusResolved &&
		now.Sub(existing.LastSeen) >= resolveAfter
}

// ResolveCleared resolves checkType's open alerts that eval evaluated without
// detecting and that have gone undetected for at least resolveAfter, and sends
// each one's resolution notification. It returns how many it resolved.
func ResolveCleared(
	ctx context.Context,
	store Store,
	notifier Notifier,
	log Logger,
	checkType string,
	eval Evaluation,
	now time.Time,
	resolveAfter time.Duration,
) int {
	open, err := store.ListOpenAlertsByCheck(ctx, eval.TenantID, checkType)
	if err != nil {
		log.Error("Failed to list open alerts to resolve", "check_type", checkType, "error", err)
		return 0
	}

	detected := make(map[string]bool, len(eval.Alerts))
	for _, a := range eval.Alerts {
		detected[a.AlertID] = true
	}

	resolved := 0
	for _, alert := range open {
		if detected[alert.AlertID] || !eval.evaluated(alert) {
			continue
		}
		if now.Sub(alert.LastSeen) < resolveAfter {
			continue
		}
		if err := store.ResolveAlert(ctx, alert.TenantID, alert.AlertID); err != nil {
			log.Error("Failed to resolve cleared alert", "alert_id", alert.AlertID, "error", err)
			continue
		}
		resolved++
		log.Info("Resolved alert whose condition cleared",
			"alert_id", alert.AlertID,
			"check_type", checkType,
			"last_seen", alert.LastSeen.Format(time.RFC3339))

		if notifier == nil {
			continue
		}
		alert.Status = domain.AlertStatusResolved
		resolvedAt := now
		alert.ResolvedAt = &resolvedAt
		alert.Description = fmt.Sprintf("%s\n\nCleared: not detected since %s.",
			alert.Description, alert.LastSeen.Format(time.RFC3339))
		if err := notifier.NotifyAlertResolution(ctx, alert); err != nil {
			log.Error("Failed to send resolution notification", "alert_id", alert.AlertID, "error", err)
		}
	}
	return resolved
}
