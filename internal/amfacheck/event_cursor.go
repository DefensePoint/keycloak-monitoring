package amfacheck

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/amfa"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

// lateEventWindow is how far back each read reaches before the cursor, so an
// event stored after a newer one was read is still found. Events already
// alerted are skipped by alert ID.
const lateEventWindow = 10 * time.Minute

// EventCheck is a Check that raises one alert per AMFA event and reads events
// from a per-realm cursor. The service skips events whose alert already exists
// and calls Commit with every alert that is stored, so the cursor never moves
// past an event whose alert was not saved.
type EventCheck interface {
	Check
	Commit(stored map[string]bool)
}

// ResumeFunc returns when the newest alert of checkType in realm was first
// detected, so a restarted check resumes from there instead of one poll
// interval back. ok is false when there is none.
type ResumeFunc func(ctx context.Context, checkType, realm string) (at time.Time, ok bool, err error)

// EventCheckOption configures a per-event check.
type EventCheckOption func(*eventCursor)

// WithResume sets how a check finds where to resume after a restart.
func WithResume(fn ResumeFunc) EventCheckOption {
	return func(c *eventCursor) { c.resume = fn }
}

type pendingEvent struct {
	alertID string
	at      time.Time
}

// eventCursor tracks, per realm, the time up to which every event's alert is
// stored.
type eventCursor struct {
	checkType string
	lookback  time.Duration
	resume    ResumeFunc
	logger    Logger

	mu      sync.Mutex
	cursor  map[string]time.Time
	pending map[string][]pendingEvent
	// handled holds, per realm, the events already stored or silenced that
	// a read can still return because of lateEventWindow, so they are not
	// raised again: a silenced event has no alert to find.
	handled map[string]map[string]time.Time
}

func newEventCursor(checkType string, lookback time.Duration, log Logger, opts []EventCheckOption) *eventCursor {
	c := &eventCursor{
		checkType: checkType,
		lookback:  lookback,
		logger:    log,
		cursor:    map[string]time.Time{},
		pending:   map[string][]pendingEvent{},
		handled:   map[string]map[string]time.Time{},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// read lists each realm's events after its cursor, less lateEventWindow, and
// turns them into alerts. A realm whose query fails keeps its cursor and is
// reported in the returned error; the other realms' alerts are still returned.
func (c *eventCursor) read(
	ctx context.Context,
	realms []string,
	list func(ctx context.Context, realm string, since time.Time) ([]amfa.EventRow, error),
	toAlert func(realm string, ev amfa.EventRow) *domain.Alert,
) ([]*domain.Alert, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.pending = map[string][]pendingEvent{}
	var alerts []*domain.Alert
	var errs []error
	for _, realm := range realms {
		events, err := list(ctx, realm, c.start(ctx, realm).Add(-lateEventWindow))
		if err != nil {
			errs = append(errs, fmt.Errorf("realm=%s: %w", realm, err))
			continue
		}
		pending := make([]pendingEvent, 0, len(events))
		for _, ev := range events {
			alert := toAlert(realm, ev)
			if _, done := c.handled[realm][alert.AlertID]; done {
				continue
			}
			alerts = append(alerts, alert)
			pending = append(pending, pendingEvent{alertID: alert.AlertID, at: ev.EventTime})
		}
		c.pending[realm] = pending
	}
	return alerts, errors.Join(errs...)
}

// start returns the realm's cursor, setting it on first sight from the newest
// stored alert when that reaches further back than one lookback.
func (c *eventCursor) start(ctx context.Context, realm string) time.Time {
	if at, ok := c.cursor[realm]; ok {
		return at
	}
	at := time.Now().Add(-c.lookback)
	if c.resume != nil {
		latest, ok, err := c.resume(ctx, c.checkType, realm)
		switch {
		case err != nil:
			c.logger.Warn("amfacheck: could not find where to resume; reading one poll interval back",
				"check_type", c.checkType, "realm", realm, "error", err)
		case ok:
			// An alert is first detected up to one poll after its event.
			if from := latest.Add(-c.lookback); from.Before(at) {
				at = from
			}
		}
	}
	c.cursor[realm] = at
	return at
}

// Commit moves each realm read by the last read past the events whose alerts
// are stored. When one is not, the cursor stops just before it, so the next
// read returns it again.
func (c *eventCursor) Commit(stored map[string]bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for realm, events := range c.pending {
		next := c.cursor[realm]
		var firstUnstored time.Time
		for _, ev := range events {
			if !stored[ev.alertID] {
				if firstUnstored.IsZero() || ev.at.Before(firstUnstored) {
					firstUnstored = ev.at
				}
				continue
			}
			if ev.at.After(next) {
				next = ev.at
			}
		}
		if !firstUnstored.IsZero() && !firstUnstored.After(next) {
			next = firstUnstored.Add(-time.Nanosecond)
		}
		c.cursor[realm] = next

		if c.handled[realm] == nil {
			c.handled[realm] = map[string]time.Time{}
		}
		for _, ev := range events {
			if stored[ev.alertID] {
				c.handled[realm][ev.alertID] = ev.at
			}
		}
		// A read returns only events after next minus lateEventWindow.
		for id, at := range c.handled[realm] {
			if !at.After(next.Add(-lateEventWindow)) {
				delete(c.handled[realm], id)
			}
		}
	}
	c.pending = map[string][]pendingEvent{}
}
