package metricscheck

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/lifecycle"
)

// Service defines the interface for metrics checking operations.
type Service interface {
	// Start begins the metrics checking service.
	Start(ctx context.Context) error

	// Stop gracefully stops the metrics checking service.
	Stop() error

	// AddCheck adds a new metrics check to the service.
	AddCheck(check Checker)

	// RunCheckNow executes all checks immediately.
	RunCheckNow(ctx context.Context) error
}

// service implements the Service interface.
type service struct {
	checks       []Checker
	alertStore   AlertStore
	notifier     NotificationService
	logger       Logger
	pollInterval time.Duration
	// workers tracks every goroutine Start launches, including the
	// immediate first run, so Stop drains all of them.
	workers lifecycle.Workers
}

// NewService creates a new metrics checker service.
func NewService(
	checks []Checker,
	alertStore AlertStore,
	notifier NotificationService,
	logger Logger,
	pollInterval time.Duration,
) Service {
	return &service{
		checks:       checks,
		alertStore:   alertStore,
		notifier:     notifier,
		logger:       logger.WithComponent("metrics_checker"),
		pollInterval: pollInterval,
	}
}

// AddCheck adds a new metrics check to the service.
func (s *service) AddCheck(check Checker) {
	s.checks = append(s.checks, check)
	s.logger.Info("Added metrics check", "check_type", check.GetCheckType())
}

// Start begins the metrics checking service.
func (s *service) Start(ctx context.Context) error {
	s.logger.Info("Starting metrics checker service",
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

// Stop gracefully stops the metrics checking service.
func (s *service) Stop() error {
	s.logger.Info("Stopping metrics checker service")
	if s.workers.Drain(10 * time.Second) {
		s.logger.Info("Metrics checker service stopped gracefully")
	} else {
		s.logger.Warn("Metrics checker service stop timeout")
	}

	return nil
}

// pollChecks periodically executes all metrics checks.
func (s *service) pollChecks(ctx context.Context) {
	ticker := time.NewTicker(s.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.runChecks(ctx)
		case <-s.workers.Stopping():
			s.logger.Info("Metrics checker polling stopped")
			return
		case <-ctx.Done():
			s.logger.Info("Metrics checker polling cancelled")
			return
		}
	}
}

// runChecks executes all registered metrics checks.
func (s *service) runChecks(ctx context.Context) {
	s.logger.Debug("Running metrics checks", "num_checks", len(s.checks))

	for _, check := range s.checks {
		if err := s.executeCheck(ctx, check); err != nil {
			s.logger.Error("Failed to execute metrics check",
				"check_type", check.GetCheckType(),
				"error", err)
		}
	}
}

// executeCheck runs a single metrics check and saves detected alerts.
func (s *service) executeCheck(ctx context.Context, check Checker) error {
	startTime := time.Now()

	s.logger.Debug("Executing metrics check", "check_type", check.GetCheckType())

	// Execute the check
	alerts, err := check.Execute(ctx)
	if err != nil {
		return fmt.Errorf("check execution failed: %w", err)
	}

	// Process detected alerts
	newAlerts := 0
	updatedAlerts := 0

	now := time.Now()
	for _, alert := range alerts {
		// Check if alert already exists
		existing, err := s.alertStore.GetAlertByID(ctx, alert.TenantID, alert.AlertID)
		isNewAlert := err != nil || existing == nil

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

		// Determine if we should send a notification
		shouldNotify := false
		notificationReason := ""

		if isNewAlert {
			shouldNotify = true
			notificationReason = "new alert"
		} else if existing.Status == domain.AlertStatusActive && existing.AcknowledgedAt == nil {
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
		if shouldNotify && alert.Metadata != "" {
			var meta map[string]interface{}
			if err := json.Unmarshal([]byte(alert.Metadata), &meta); err == nil {
				meta["last_notified_at"] = now.Format(time.RFC3339)
				if metadataBytes, err := json.Marshal(meta); err == nil {
					alert.Metadata = string(metadataBytes)
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
				s.logger.Debug("Sent notification",
					"alert_id", alert.AlertID,
					"reason", notificationReason)
			}
		}
	}

	duration := time.Since(startTime)
	s.logger.Info("Metrics check completed",
		"check_type", check.GetCheckType(),
		"new_alerts", newAlerts,
		"updated_alerts", updatedAlerts,
		"duration_ms", duration.Milliseconds())

	return nil
}

// RunCheckNow executes all checks immediately.
func (s *service) RunCheckNow(ctx context.Context) error {
	s.logger.Info("Running metrics checks manually")
	s.runChecks(ctx)
	return nil
}
