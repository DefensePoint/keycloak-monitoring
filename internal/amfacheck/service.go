package amfacheck

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/lifecycle"
)

// reNotifyInterval is how long an unacknowledged, still-active alert waits
// before a repeat notification is sent. Mirrors eventscheck.
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
) Service {
	return &service{
		tenantID:     tenantID,
		checks:       checks,
		alertStore:   alertStore,
		notifier:     notifier,
		logger:       log.WithComponent("amfa_checker"),
		pollInterval: pollInterval,
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
	alerts, err := check.Execute(ctx)
	if err != nil {
		return fmt.Errorf("check execution failed: %w", err)
	}

	newAlerts, updatedAlerts := 0, 0
	now := time.Now()
	for _, alert := range alerts {
		existing, gerr := s.alertStore.GetAlertByID(ctx, alert.TenantID, alert.AlertID)
		isNewAlert := gerr != nil || existing == nil

		if isNewAlert {
			alert.FirstDetected = now
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
		} else if existing.Status == domain.AlertStatusActive && existing.AcknowledgedAt == nil {
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

		if shouldNotify && alert.Metadata != "" {
			var meta map[string]interface{}
			if jerr := json.Unmarshal([]byte(alert.Metadata), &meta); jerr == nil {
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

	s.logger.Debug("amfacheck: check completed",
		"tenant_id", s.tenantID,
		"check", check.GetCheckType(),
		"new_alerts", newAlerts,
		"updated_alerts", updatedAlerts,
		"duration_ms", time.Since(startTime).Milliseconds())
	return nil
}

func (s *service) RunCheckNow(ctx context.Context) error {
	return s.runChecks(ctx)
}
