package configcheck

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
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
)

// Service defines the interface for configuration checking operations.
type Service interface {
	// Start begins the configuration checking service.
	Start(ctx context.Context) error

	// Stop gracefully stops the configuration checking service.
	Stop() error

	// AddCheck adds a new configuration check to the service.
	AddCheck(check Check)

	// RunCheckNow executes all checks immediately.
	RunCheckNow(ctx context.Context) error
}

// service implements the Service interface.
type service struct {
	checks       []Check
	alertStore   AlertStore
	notifier     NotificationService
	logger       *logger.Logger
	pollInterval time.Duration
	// runMu makes runs take turns. Start's first run and the poller's first
	// tick overlap when that run outlasts the poll interval, and RunCheckNow
	// may come at any time; two runs at once would each count a miss in the
	// same round, and save the same alerts concurrently. It also guards
	// misses.
	runMu sync.Mutex
	// misses counts, per tenant and check, and per open alert, the runs in a
	// row that read the alert's realm without finding its problem. See
	// resolveFixed. Guarded by runMu.
	misses map[missesKey]map[string]int
	// poll collects what the current run's checks learn about realms; see
	// reportRealms. Set only while runChecks runs, and guarded by runMu.
	poll *realmPoll
	// unreadable holds, per tenant, the realms last reported as unreadable.
	// Guarded by runMu.
	unreadable map[tenantRealm]bool
	// silencer, when set, keeps alerts a silence rule covers from being
	// stored or notified. See WithSilencer.
	silencer Silencer
	// workers tracks every goroutine Start launches, including the
	// immediate first run, so Stop drains all of them.
	workers lifecycle.Workers
}

// NewService creates a new configuration checker service.
func NewService(
	checks []Check,
	alertStore AlertStore,
	notifier NotificationService,
	log *logger.Logger,
	pollInterval time.Duration,
	opts ...Option,
) Service {
	s := &service{
		checks:       checks,
		alertStore:   alertStore,
		notifier:     notifier,
		logger:       log.WithComponent("config_checker"),
		pollInterval: pollInterval,
		misses:       map[missesKey]map[string]int{},
		unreadable:   map[tenantRealm]bool{},
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Option configures a Service.
type Option func(*service)

// WithSilencer applies the configured alert silences: an alert a rule covers
// is neither stored nor notified, as in the event and AMFA checkers.
func WithSilencer(silencer Silencer) Option {
	return func(s *service) { s.silencer = silencer }
}

// AddCheck adds a new configuration check to the service.
func (s *service) AddCheck(check Check) {
	s.checks = append(s.checks, check)
	s.logger.Info("Added configuration check", logger.Str("check_type", check.GetCheckType()))
}

// Start begins the configuration checking service.
func (s *service) Start(ctx context.Context) error {
	s.logger.Info("Starting configuration checker service",
		logger.Int("num_checks", len(s.checks)),
		logger.Str("poll_interval", s.pollInterval.String()))

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

// Stop gracefully stops the configuration checking service.
func (s *service) Stop() error {
	s.logger.Info("Stopping configuration checker service")
	if s.workers.Drain(10 * time.Second) {
		s.logger.Info("Configuration checker service stopped gracefully")
	} else {
		s.logger.Warn("Configuration checker service stop timeout")
	}

	return nil
}

// pollChecks periodically executes all configuration checks.
func (s *service) pollChecks(ctx context.Context) {
	ticker := time.NewTicker(s.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.runChecks(ctx)
		case <-s.workers.Stopping():
			s.logger.Info("Configuration checker polling stopped")
			return
		case <-ctx.Done():
			s.logger.Info("Configuration checker polling cancelled")
			return
		}
	}
}

// runChecks executes all registered configuration checks.
func (s *service) runChecks(ctx context.Context) {
	s.runMu.Lock()
	defer s.runMu.Unlock()
	s.logger.Debug("Running configuration checks", logger.Int("num_checks", len(s.checks)))

	s.poll = newRealmPoll()
	for _, check := range s.checks {
		if err := s.executeCheck(ctx, check); err != nil {
			s.logger.Error("Failed to execute configuration check",
				logger.Str("check_type", check.GetCheckType()),
				logger.Err(err))
		}
	}
	s.reportRealms()
	s.poll = nil
}

// executeCheck runs a single configuration check and saves detected alerts.
func (s *service) executeCheck(ctx context.Context, check Check) error {
	startTime := time.Now()

	s.logger.Debug("Executing configuration check", logger.Str("check_type", check.GetCheckType()))

	// Execute the check. An Evaluator also says which realms it read, which
	// is what lets the alert of a fixed problem resolve.
	var alerts []*domain.Alert
	var eval *Evaluation
	if ev, ok := check.(Evaluator); ok {
		e, err := ev.Evaluate(ctx)
		if err != nil {
			s.poll.failed(e.TenantID)
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
	updatedAlerts := 0
	silencedAlerts := 0
	lookupFailed := 0

	now := time.Now()
	for _, alert := range alerts {
		if s.silencer != nil {
			if silenced, silenceID := s.silencer.ShouldSilence(alert); silenced {
				silencedAlerts++
				s.logger.Debug("Alert silenced by rule",
					logger.Str("alert_id", alert.AlertID),
					logger.Str("check_type", check.GetCheckType()),
					logger.Str("silence_id", silenceID))
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
			// it up again.
			s.logger.Error("Failed to look up existing alert; skipping it this cycle",
				logger.Str("alert_id", alert.AlertID),
				logger.Err(err))
			lookupFailed++
			continue
		}
		isNewAlert := existing == nil
		// A misconfiguration found again after its alert was resolved, by the
		// checker or by hand, was not fixed or has come back: a new occurrence.
		// Unlike a burst of errors it is a standing state, so it reopens even
		// when it was never seen to clear. Acknowledged and ignored alerts keep
		// the status an operator gave them.
		reopened := !isNewAlert && eval != nil && existing.Status == domain.AlertStatusResolved

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
			notificationReason = "found again after resolving"
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
			s.logger.Error("Failed to save alert",
				logger.Str("alert_id", alert.AlertID),
				logger.Err(err))
			continue
		}

		s.resolveSuperseded(ctx, alert)

		// Send notification if needed
		if shouldNotify && s.notifier != nil {
			if err := s.notifier.NotifyAlert(ctx, alert); err != nil {
				s.logger.Error("Failed to send notification",
					logger.Str("alert_id", alert.AlertID),
					logger.Str("reason", notificationReason),
					logger.Err(err))
			} else {
				s.logger.Debug("Requested notification; the notification service logs each channel's outcome",
					logger.Str("alert_id", alert.AlertID),
					logger.Str("reason", notificationReason))
			}
		}
	}

	resolved := 0
	if eval != nil {
		resolved = s.resolveFixed(ctx, check, *eval, now)
		// After resolveFixed: judging an alert can find another unreadable
		// realm.
		s.poll.add(eval)
	}

	duration := time.Since(startTime)
	s.logger.Info("Configuration check completed",
		logger.Str("check_type", check.GetCheckType()),
		logger.Int("new_alerts", newAlerts),
		logger.Int("updated_alerts", updatedAlerts),
		logger.Int("silenced_alerts", silencedAlerts),
		logger.Int("lookup_failed_alerts", lookupFailed),
		logger.Int("resolved_alerts", resolved),
		logger.Int64("duration_ms", duration.Milliseconds()))

	return nil
}

const (
	cannotViewRealm = "Monitoring client cannot view this realm's settings; keeping its alerts as they are"
	canViewRealm    = "Monitoring client can view this realm's settings again"
)

// tenantRealm names a realm of one tenant: tenants have realms of the same
// name, and the service checks every tenant.
type tenantRealm struct {
	tenantID string
	realm    string
}

// realmPoll is what one run of every check learned about realms. Its methods
// do nothing on a nil poll, as when a test runs one check on its own.
type realmPoll struct {
	unreadable map[tenantRealm]bool
	read       map[tenantRealm]bool
	// failedTenants holds the tenants a check failed for outright: that run
	// learned nothing about their realms.
	failedTenants map[string]bool
}

func newRealmPoll() *realmPoll {
	return &realmPoll{
		unreadable:    map[tenantRealm]bool{},
		read:          map[tenantRealm]bool{},
		failedTenants: map[string]bool{},
	}
}

// add records the realms one check read and the ones it could not read.
func (p *realmPoll) add(eval *Evaluation) {
	if p == nil {
		return
	}
	for realm := range eval.EvaluatedRealms {
		p.read[tenantRealm{eval.TenantID, realm}] = true
	}
	for _, realm := range eval.unreadableRealms() {
		p.unreadable[tenantRealm{eval.TenantID, realm}] = true
	}
}

// failed records that a check failed outright for tenantID.
func (p *realmPoll) failed(tenantID string) {
	if p == nil || tenantID == "" {
		return
	}
	p.failedTenants[tenantID] = true
}

// reportRealms logs a realm once when the checks stop being able to read it,
// and once when they can again, rather than on every check of every poll. A
// tenant a check failed outright for keeps the realms reported before, since
// the run learned nothing about them. A realm counts as readable again only
// when a check read it this poll: one that merely dropped out, deleted or no
// longer judged because its alerts were resolved, is forgotten without a word.
func (s *service) reportRealms() {
	poll := s.poll
	now := poll.unreadable
	for key := range s.unreadable {
		if poll.failedTenants[key.tenantID] {
			now[key] = true
		}
	}
	for key := range now {
		if !s.unreadable[key] {
			s.logger.Warn(cannotViewRealm, logger.Str("tenant_id", key.tenantID), logger.Str("realm", key.realm))
		}
	}
	for key := range s.unreadable {
		if !now[key] && poll.read[key] {
			s.logger.Info(canViewRealm, logger.Str("tenant_id", key.tenantID), logger.Str("realm", key.realm))
		}
	}
	s.unreadable = now
}

// missesKey names one check of one tenant.
type missesKey struct {
	tenantID  string
	checkType string
}

// missesToResolve is how many runs in a row must read a realm without finding
// a problem before its alert resolves, so one odd read does not resolve an
// alert only to reopen it on the next run.
const missesToResolve = 2

// resolveFixed resolves, with a resolution notification, the open alerts of
// checkType whose problem missesToResolve runs in a row have read and not
// found. It counts runs, not time: after a restart, or once Keycloak is
// reachable again, the time since an alert was last seen says nothing about
// how many reads have missed it. A run that could not read an alert's realm
// leaves its count as it is, as does a run whose sub-check for the alert is
// switched off; the counts start over when the process does.
func (s *service) resolveFixed(ctx context.Context, check Check, eval Evaluation, now time.Time) int {
	checkType := check.GetCheckType()
	open, err := s.alertStore.ListOpenAlertsByCheck(ctx, eval.TenantID, checkType)
	if err != nil {
		s.logger.Error("Failed to list open alerts to resolve",
			logger.Str("check_type", checkType), logger.Err(err))
		return 0
	}
	detected := make(map[string]bool, len(eval.Alerts))
	for _, a := range eval.Alerts {
		detected[a.AlertID] = true
	}

	// Only the open alerts carry a count over, so the counts stay bounded and
	// an alert that resolves or is ignored starts again from zero.
	fixed := clearing.Evaluation{TenantID: eval.TenantID, Alerts: eval.Alerts}
	key := missesKey{tenantID: eval.TenantID, checkType: checkType}
	prev := s.misses[key]
	counts := make(map[string]int, len(open))
	for _, a := range open {
		switch {
		case detected[a.AlertID]:
			// Found: the count starts over.
		case eval.Covers(a) && looksFor(check, a),
			// The realm was deleted, disabled or left out of the run: the
			// problem went with it, whatever the sub-checks.
			!eval.Covers(a) && eval.goneWith(a):
			counts[a.AlertID] = prev[a.AlertID] + 1
			if counts[a.AlertID] >= missesToResolve {
				fixed.MarkEvaluated(a.AlertID)
			}
		case prev[a.AlertID] > 0:
			// Not read this run: neither a miss nor a sighting.
			counts[a.AlertID] = prev[a.AlertID]
		}
	}
	s.misses[key] = counts

	return clearing.ResolveCleared(ctx, s.alertStore, s.notifier, clearingLogger{s.logger},
		checkType, fixed, now, 0)
}

// looksFor reports whether check still looks for the problem behind alert:
// false when the sub-check that raised it is switched off.
func looksFor(check Check, alert *domain.Alert) bool {
	switches, ok := check.(subCheckSwitches)
	if !ok || alert.Metadata == "" {
		return true
	}
	var meta struct {
		AlertType string `json:"alert_type"`
	}
	if err := json.Unmarshal([]byte(alert.Metadata), &meta); err != nil || meta.AlertType == "" {
		return true
	}
	return switches.looksFor(meta.AlertType)
}

// resolveSuperseded resolves the open alerts that alert replaces. Their
// replacement carries the news, so no resolution notification is sent.
func (s *service) resolveSuperseded(ctx context.Context, alert *domain.Alert) {
	for _, alertID := range alert.Supersedes {
		existing, err := s.alertStore.GetAlertByID(ctx, alert.TenantID, alertID)
		if err != nil || existing == nil || existing.Status == domain.AlertStatusResolved {
			continue
		}
		if err := s.alertStore.ResolveAlert(ctx, alert.TenantID, alertID); err != nil {
			s.logger.Error("Failed to resolve superseded alert",
				logger.Str("alert_id", alertID),
				logger.Str("superseded_by", alert.AlertID),
				logger.Err(err))
			continue
		}
		s.logger.Info("Resolved superseded alert",
			logger.Str("alert_id", alertID),
			logger.Str("superseded_by", alert.AlertID))
	}
}

// RunCheckNow executes all checks immediately.
func (s *service) RunCheckNow(ctx context.Context) error {
	s.logger.Info("Running configuration checks manually")
	s.runChecks(ctx)
	return nil
}

// clearingLogger gives clearing the key-value logging it expects.
type clearingLogger struct{ log *logger.Logger }

func (l clearingLogger) Info(msg string, args ...any)  { l.log.Info(msg, clearingFields(args)...) }
func (l clearingLogger) Error(msg string, args ...any) { l.log.Error(msg, clearingFields(args)...) }

func clearingFields(args []any) []logger.Field {
	fields := make([]logger.Field, 0, len(args)/2)
	for i := 0; i+1 < len(args); i += 2 {
		key, ok := args[i].(string)
		if !ok {
			continue
		}
		// An error value renders as {}, so it goes in as its text.
		if err, isErr := args[i+1].(error); isErr {
			fields = append(fields, logger.Str(key, err.Error()))
			continue
		}
		fields = append(fields, logger.Any(key, args[i+1]))
	}
	return fields
}
