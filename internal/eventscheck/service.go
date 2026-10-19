package eventscheck

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

// Service defines the interface for event checking operations.
type Service interface {
	// Start begins the event checking service.
	Start(ctx context.Context) error

	// Stop gracefully stops the event checking service.
	Stop() error

	// AddCheck adds a new event check to the service.
	AddCheck(check Check)

	// RunCheckNow executes all checks immediately.
	RunCheckNow(ctx context.Context) error
}

// service implements the Service interface.
type service struct {
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

// Option configures a Service.
type Option func(*service)

// WithResolveAfter sets how long a condition must go undetected before its
// alert resolves. Zero or less keeps clearing.DefaultResolveAfter.
func WithResolveAfter(d time.Duration) Option {
	return func(s *service) {
		if d > 0 {
			s.resolveAfter = d
		}
	}
}

// WithSilencer applies the configured alert silences: an alert a rule covers
// is neither stored nor notified, as for alerts raised from single events.
//
// A silenced alert still counts as detected, so its condition is not mistaken
// for one that cleared: an alert already open when the silence began stays
// open, unannounced, and resolves only once the condition really clears.
func WithSilencer(silencer Silencer) Option {
	return func(s *service) { s.silencer = silencer }
}

// NewService creates a new event checker service.
func NewService(
	checks []Check,
	alertStore AlertStore,
	notifier NotificationService,
	logger Logger,
	pollInterval time.Duration,
	opts ...Option,
) Service {
	s := &service{
		checks:       checks,
		alertStore:   alertStore,
		notifier:     notifier,
		logger:       logger.WithComponent("events_checker"),
		pollInterval: pollInterval,
		resolveAfter: clearing.DefaultResolveAfter,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// AddCheck adds a new event check to the service.
func (s *service) AddCheck(check Check) {
	s.checks = append(s.checks, check)
	s.logger.Info("Added event check", "check_type", check.GetCheckType())
}

// Start begins the event checking service.
func (s *service) Start(ctx context.Context) error {
	s.logger.Info("Starting event checker service",
		"num_checks", len(s.checks),
		"poll_interval", s.pollInterval.String())

	// Perform initial check immediately
	s.workers.Go(func() {
		s.runChecks(ctx)
	})

	// Start polling worker
	s.workers.Go(func() {
		s.pollChecks(ctx)
	})

	return nil
}

// Stop gracefully stops the event checking service.
func (s *service) Stop() error {
	s.logger.Info("Stopping event checker service")
	if s.workers.Drain(10 * time.Second) {
		s.logger.Info("Event checker service stopped gracefully")
	} else {
		s.logger.Warn("Event checker service stop timeout")
	}

	return nil
}

// pollChecks periodically executes all event checks.
func (s *service) pollChecks(ctx context.Context) {
	ticker := time.NewTicker(s.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.runChecks(ctx)
		case <-s.workers.Stopping():
			s.logger.Info("Event checker polling stopped")
			return
		case <-ctx.Done():
			s.logger.Info("Event checker polling cancelled")
			return
		}
	}
}

// runChecks executes all registered event checks.
func (s *service) runChecks(ctx context.Context) {
	s.runMu.Lock()
	defer s.runMu.Unlock()
	s.logger.Debug("Running event checks", "num_checks", len(s.checks))

	for _, check := range s.checks {
		if err := s.executeCheck(ctx, check); err != nil {
			s.logger.Error("Failed to execute event check",
				"check_type", check.GetCheckType(),
				"error", err)
		}
	}
}

// executeCheck runs a single event check and saves detected alerts.
func (s *service) executeCheck(ctx context.Context, check Check) error {
	startTime := time.Now()

	s.logger.Debug("Executing event check", "check_type", check.GetCheckType())

	// Execute the check. An Evaluator also says which conditions it
	// evaluated, which is what lets a cleared condition's alert resolve.
	var alerts []*domain.Alert
	var eval *clearing.Evaluation
	if ev, ok := check.(Evaluator); ok {
		e, err := ev.Evaluate(ctx)
		if err != nil {
			return fmt.Errorf("check execution failed: %w", err)
		}
		alerts, eval = e.Alerts, &e
	} else {
		var err error
		alerts, err = check.Execute(ctx)
		if err != nil {
			return fmt.Errorf("check execution failed: %w", err)
		}
	}

	// Process detected alerts
	newAlerts := 0
	silencedAlerts := 0
	updatedAlerts := 0
	lookupFailed := 0

	now := time.Now()
	for _, alert := range alerts {
		if s.silencer != nil {
			if silenced, silenceID := s.silencer.ShouldSilence(alert); silenced {
				silencedAlerts++
				s.logger.Debug("Alert silenced by rule",
					"alert_id", alert.AlertID,
					"check_type", check.GetCheckType(),
					"silence_id", silenceID)
				continue
			}
		}

		// Check if alert already exists
		existing, err := s.alertStore.GetAlertByID(ctx, alert.TenantID, alert.AlertID)
		if err != nil && !errors.Is(err, alertspkg.ErrNotFound) {
			// A failed read says nothing about whether the alert exists.
			// Treating it as new would save it as a fresh active alert, which
			// reactivates an acknowledged or ignored one and clears its
			// acknowledgment, so leave it alone this cycle; the next run looks
			// it up again. The error goes in as text: the logger adapter
			// renders an error value as {}.
			s.logger.Error("Failed to look up existing alert; skipping it this cycle",
				"alert_id", alert.AlertID,
				"error", err.Error())
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

		// Determine if we should send a notification
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
				if err := json.Unmarshal([]byte(existing.Metadata), &meta); err == nil {
					if lastNotifiedStr, ok := meta["last_notified_at"].(string); ok {
						if parsed, err := time.Parse(time.RFC3339, lastNotifiedStr); err == nil {
							lastNotifiedAt = parsed
						}
					}
				}
			}

			if time.Since(lastNotifiedAt) > time.Hour {
				shouldNotify = true
				notificationReason = "re-notification (1+ hour, not acknowledged)"
			}
		}

		// Update metadata with last notification time
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

		// Save alert
		if err := s.alertStore.SaveAlert(ctx, alert); err != nil {
			s.logger.Error("Failed to save alert", "alert_id", alert.AlertID, "error", err)
			continue
		}

		// Send notification if needed
		if shouldNotify && s.notifier != nil {
			if err := s.notifier.NotifyAlert(ctx, alert); err != nil {
				s.logger.Error("Failed to send notification",
					"alert_id", alert.AlertID,
					"reason", notificationReason,
					"error", err)
			} else {
				s.logger.Debug("Requested notification; the notification service logs each channel's outcome",
					"alert_id", alert.AlertID,
					"reason", notificationReason)
			}
		}
	}

	resolved := 0
	if eval != nil {
		resolved = clearing.ResolveCleared(ctx, s.alertStore, s.notifier, s.logger,
			check.GetCheckType(), *eval, now, s.resolveAfter)
	}

	duration := time.Since(startTime)
	s.logger.Info("Event check completed",
		"check_type", check.GetCheckType(),
		"new_alerts", newAlerts,
		"updated_alerts", updatedAlerts,
		"silenced_alerts", silencedAlerts,
		"lookup_failed_alerts", lookupFailed,
		"resolved_alerts", resolved,
		"duration_ms", duration.Milliseconds())

	return nil
}

// RunCheckNow executes all checks immediately.
func (s *service) RunCheckNow(ctx context.Context) error {
	s.logger.Info("Running event checks manually")
	s.runChecks(ctx)
	return nil
}
