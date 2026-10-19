package amfacheck

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	alertspkg "github.com/DefensePoint/keycloak-monitoring/internal/alerts"
	"github.com/DefensePoint/keycloak-monitoring/internal/alerts/clearing"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/lifecycle"
)

// reNotifyInterval is how long an unacknowledged, still-active alert waits
// before its notification is requested again. Channels that already received
// it skip the request; for a channel whose delivery failed it is the retry.
// Mirrors eventscheck.
const reNotifyInterval = time.Hour

// Service runs AMFA risk checks for a single tenant.
type Service interface {
	// Start begins polling. Runs an immediate check, then on a ticker.
	Start(ctx context.Context) error
	// Stop gracefully stops polling (10s drain).
	Stop() error
	// AddCheck registers an additional check.
	AddCheck(check Check)
	// RunCheckNow executes all checks once, synchronously.
	RunCheckNow(ctx context.Context) error
}

type service struct {
	tenantID     string
	checks       []Check
	alertStore   AlertStore
	notifier     NotificationService
	logger       Logger
	pollInterval time.Duration
	// resolveAfter is how long a condition must go undetected before its
	// alert resolves; see clearing.
	resolveAfter time.Duration

	// silencer, when set, keeps alerts a silence rule covers from being
	// stored or notified. See WithSilencer.
	silencer Silencer

	// runMu makes runs take turns. A manual run can overlap the poll loop, and
	// two runs that each see an alert as new, or each see it cleared, would
	// each announce it.
	runMu sync.Mutex
	// workers tracks every goroutine Start launches, including the
	// immediate first run, so Stop drains all of them.
	workers lifecycle.Workers
}

// NewService creates a per-tenant AMFA checker service.
func NewService(
	tenantID string,
	checks []Check,
	alertStore AlertStore,
	notifier NotificationService,
	log Logger,
	pollInterval time.Duration,
	opts ...Option,
) Service {
	s := &service{
		tenantID:     tenantID,
		checks:       checks,
		alertStore:   alertStore,
		notifier:     notifier,
		logger:       log.WithComponent("amfa_checker"),
		pollInterval: pollInterval,
		resolveAfter: clearing.DefaultResolveAfter,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Option configures a Service.
type Option func(*service)

// WithSilencer applies the configured alert silences: an alert a rule covers
// is neither stored nor notified, as for alerts raised from single events.
//
// A silenced alert still counts as detected, so its condition is not mistaken
// for one that cleared: an alert already open when the silence began stays
// open, unannounced, and resolves only once the condition really clears.
func WithSilencer(silencer Silencer) Option {
	return func(s *service) { s.silencer = silencer }
}

// WithResolveAfter sets how long a condition must go undetected before its
// alert resolves. Zero or less keeps clearing.DefaultResolveAfter.
func WithResolveAfter(d time.Duration) Option {
	return func(s *service) {
		if d > 0 {
			s.resolveAfter = d
		}
	}
}

func (s *service) AddCheck(check Check) {
	s.checks = append(s.checks, check)
	s.logger.Info("Added AMFA check", "tenant_id", s.tenantID, "check_type", check.GetCheckType())
}

func (s *service) Start(ctx context.Context) error {
	s.logger.Info("amfacheck service starting",
		"tenant_id", s.tenantID,
		"num_checks", len(s.checks),
		"poll_interval", s.pollInterval.String())

	s.workers.Go(func() {
		_ = s.runChecks(ctx) // best-effort: errors are logged; the poller's first tick will retry
	})

	s.workers.Go(func() {
		s.pollChecks(ctx)
	})
	return nil
}

func (s *service) Stop() error {
	s.logger.Info("Stopping amfacheck service", "tenant_id", s.tenantID)
	if s.workers.Drain(10 * time.Second) {
		s.logger.Info("amfacheck service stopped gracefully", "tenant_id", s.tenantID)
	} else {
		s.logger.Warn("amfacheck service stop timeout", "tenant_id", s.tenantID)
	}
	return nil
}

func (s *service) pollChecks(ctx context.Context) {
	ticker := time.NewTicker(s.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			_ = s.runChecks(ctx) // best-effort: errors are logged; the loop retries next tick
		case <-s.workers.Stopping():
			s.logger.Info("amfacheck polling stopped", "tenant_id", s.tenantID)
			return
		case <-ctx.Done():
			s.logger.Info("amfacheck polling cancelled", "tenant_id", s.tenantID)
			return
		}
	}
}

// runChecks executes every check once. It logs each failure and keeps going
// (one bad check never blocks the others), and returns the failures joined.
// The background poller ignores the return (best-effort, retries next tick);
// RunCheckNow returns it so a manual trigger can report errors to the caller
// (e.g. the "Reload AMFA alerts" button gets a 503 when AMFA is unreachable
// instead of a silent success).
func (s *service) runChecks(ctx context.Context) error {
	s.runMu.Lock()
	defer s.runMu.Unlock()
	s.logger.Debug("amfacheck: running checks", "tenant_id", s.tenantID, "num_checks", len(s.checks))
	var errs []error
	for _, check := range s.checks {
		if err := s.executeCheck(ctx, check); err != nil {
			s.logger.Error("amfacheck: check failed",
				"tenant_id", s.tenantID,
				"check_type", check.GetCheckType(),
				"error", err)
			errs = append(errs, fmt.Errorf("%s: %w", check.GetCheckType(), err))
		}
	}
	return errors.Join(errs...)
}

func (s *service) executeCheck(ctx context.Context, check Check) error {
	startTime := time.Now()
	// An Evaluator also says which conditions it evaluated, which is what
	// lets a cleared condition's alert resolve.
	var alerts []*domain.Alert
	var eval *clearing.Evaluation
	if ev, ok := check.(Evaluator); ok {
		e, err := ev.Evaluate(ctx)
		if err != nil {
			return fmt.Errorf("check execution failed: %w", err)
		}
		alerts, eval = e.Alerts, &e
	}

	// A per-event check returns the realms it could read even when another
	// failed, and is told which alerts are stored so it rereads the rest.
	eventCheck, perEvent := check.(EventCheck)
	var partialErr error
	if eval == nil {
		var err error
		alerts, err = check.Execute(ctx)
		if err != nil && !perEvent {
			return fmt.Errorf("check execution failed: %w", err)
		}
		if err != nil {
			partialErr = fmt.Errorf("check execution failed: %w", err)
		}
	}
	stored := map[string]bool{}

	newAlerts, updatedAlerts, silencedAlerts, lookupFailed := 0, 0, 0, 0
	now := time.Now()
	for _, alert := range alerts {
		if s.silencer != nil {
			if silenced, silenceID := s.silencer.ShouldSilence(alert); silenced {
				silencedAlerts++
				s.logger.Debug("Alert silenced by rule",
					"alert_id", alert.AlertID,
					"check_type", check.GetCheckType(),
					"silence_id", silenceID)
				// A silenced event is handled: its cursor moves past it, so
				// it is not alerted when the silence ends, and the events
				// after it are not reread on every tick.
				if perEvent {
					stored[alert.AlertID] = true
				}
				continue
			}
		}

		if perEvent {
			exists, xerr := s.alertStore.AlertExists(ctx, alert.TenantID, alert.AlertID)
			if xerr != nil {
				s.logger.Error("amfacheck: failed to look up alert; event will be reread",
					"tenant_id", s.tenantID, "alert_id", alert.AlertID, "error", xerr.Error())
				lookupFailed++
				continue
			}
			if exists {
				stored[alert.AlertID] = true
				continue
			}
		}
		existing, gerr := s.alertStore.GetAlertByID(ctx, alert.TenantID, alert.AlertID)
		if gerr != nil && !errors.Is(gerr, alertspkg.ErrNotFound) {
			// A failed read says nothing about whether the alert exists.
			// Treating it as new would save it as a fresh active alert, which
			// reactivates an acknowledged or ignored one and clears its
			// acknowledgment, so leave it alone this cycle; the next run looks
			// it up again.
			// The error goes in as text: the logger adapter renders an error
			// value as {}.
			s.logger.Error("amfacheck: failed to look up existing alert; skipping it this cycle",
				"tenant_id", s.tenantID, "alert_id", alert.AlertID, "error", gerr.Error())
			lookupFailed++
			continue
		}
		isNewAlert := existing == nil
		// A resolved alert whose condition went away and came back is a new
		// occurrence, not a continuation of the resolved one.
		reopened := !isNewAlert && eval != nil && clearing.Reopens(existing, now, s.resolveAfter)

		if isNewAlert {
			alert.FirstDetected = now
			alert.LastSeen = now
			alert.Status = domain.AlertStatusActive
			newAlerts++
		} else if reopened {
			alert.FirstDetected = existing.FirstDetected
			alert.LastSeen = now
			alert.Status = domain.AlertStatusActive
			newAlerts++
		} else {
			alert.FirstDetected = existing.FirstDetected
			alert.LastSeen = now
			alert.Status = existing.Status
			updatedAlerts++
		}

		shouldNotify := false
		notificationReason := ""
		if isNewAlert {
			shouldNotify = true
			notificationReason = "new alert"
		} else if reopened {
			shouldNotify = true
			notificationReason = "recurred after resolving"
		} else if existing.Status == domain.AlertStatusActive && existing.AcknowledgedAt == nil {
			// While the alert stays active and unacknowledged, ask for a
			// notification again each hour. The notification service sends
			// each channel one notification per occurrence and counts only
			// successful sends, so a channel that already received this one
			// skips the request, and a channel whose delivery failed gets it
			// now: this is the only retry a failed delivery has.
			lastNotifiedAt := existing.UpdatedAt
			if existing.Metadata != "" {
				var meta map[string]interface{}
				if jerr := json.Unmarshal([]byte(existing.Metadata), &meta); jerr == nil {
					if s, ok := meta["last_notified_at"].(string); ok {
						if parsed, perr := time.Parse(time.RFC3339, s); perr == nil {
							lastNotifiedAt = parsed
						}
					}
				}
			}
			if time.Since(lastNotifiedAt) > reNotifyInterval {
				shouldNotify = true
				notificationReason = "re-notification (1+ hour, not acknowledged)"
			}
		}

		// Record the notification even on an alert with no metadata yet: the
		// re-notification check reads last_notified_at, and without it falls
		// back to updated_at, which every save of a re-detection moves.
		if shouldNotify {
			meta := map[string]interface{}{}
			if alert.Metadata == "" || json.Unmarshal([]byte(alert.Metadata), &meta) == nil {
				if meta == nil {
					meta = map[string]interface{}{}
				}
				meta["last_notified_at"] = now.Format(time.RFC3339)
				if b, merr := json.Marshal(meta); merr == nil {
					alert.Metadata = string(b)
				}
			}
		}

		if serr := s.alertStore.SaveAlert(ctx, alert); serr != nil {
			s.logger.Error("amfacheck: failed to save alert",
				"tenant_id", s.tenantID, "alert_id", alert.AlertID, "error", serr)
			continue
		}
		stored[alert.AlertID] = true

		if isNewAlert {
			s.logger.Info("amfacheck: new alert",
				"tenant_id", s.tenantID,
				"alert_id", alert.AlertID,
				"check", check.GetCheckType(),
				"severity", string(alert.Severity))
		}

		if shouldNotify && s.notifier != nil {
			if nerr := s.notifier.NotifyAlert(ctx, alert); nerr != nil {
				s.logger.Error("amfacheck: notification failed",
					"tenant_id", s.tenantID,
					"alert_id", alert.AlertID,
					"reason", notificationReason,
					"error", nerr)
			}
		}
	}

	if perEvent {
		eventCheck.Commit(stored)
	}

	resolved := 0
	if eval != nil {
		resolved = clearing.ResolveCleared(ctx, s.alertStore, s.notifier, s.logger,
			check.GetCheckType(), *eval, now, s.resolveAfter)
	}

	s.logger.Debug("amfacheck: check completed",
		"tenant_id", s.tenantID,
		"check", check.GetCheckType(),
		"new_alerts", newAlerts,
		"updated_alerts", updatedAlerts,
		"silenced_alerts", silencedAlerts,
		"lookup_failed_alerts", lookupFailed,
		"resolved_alerts", resolved,
		"duration_ms", time.Since(startTime).Milliseconds())
	return partialErr
}

func (s *service) RunCheckNow(ctx context.Context) error {
	return s.runChecks(ctx)
}
