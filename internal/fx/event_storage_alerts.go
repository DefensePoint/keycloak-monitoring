package fx

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/alerts"
	alertspostgres "github.com/DefensePoint/keycloak-monitoring/internal/alerts/postgres"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/keycloak"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/internal/notifications"
	"github.com/DefensePoint/keycloak-monitoring/pkg/database"
)

// ============================================================================
// EVENT STORAGE ALERT MANAGER (alerts when polled events cannot be stored)
// ============================================================================

// eventStorageFailingCycles is how many events cycles in a row must fail to
// store events before the tenant is alerted. One failing cycle is often a
// blip, and the next cycle retries it: each cycle fetches the whole lookback
// window again. Three in a row, 90 s at the default 30 s interval, is not.
const eventStorageFailingCycles = 3

// eventStorageAlertID names a tenant's event storage alert.
func eventStorageAlertID(tenantID string) string {
	return fmt.Sprintf("tenant-event-storage-%s", tenantID)
}

// eventStorageAlertManager raises an alert when a tenant's polled events keep
// failing to be stored, and resolves it once they are stored again.
//
// A failed write used to show only in the server log, so a tenant could lose
// every event without anyone noticing: a write that keeps failing is retried
// only while the event is inside the lookback window, and lost after that.
//
// Implements keycloak.EventStorageReporter. A cycle with nothing to write is
// neither a failure nor a recovery, so it changes nothing. The first report
// for a tenant after startup that stores everything resolves an alert left
// active from before the restart, as the health alerts reconcile.
type eventStorageAlertManager struct {
	alertRepo      alerts.Repository
	notifier       notifications.Service
	silenceManager alerts.SilenceManager
	log            *logger.Logger

	mu    sync.Mutex
	state map[string]*eventStorageState // tenantID -> state
}

type eventStorageState struct {
	failingCycles int
	// alertOpen is set once this process raised the alert, or has not yet
	// checked whether one is left from before a restart.
	alertOpen bool
}

func provideEventStorageAlertManager(
	db *database.Client,
	notifier notifications.Service,
	silenceManager alerts.SilenceManager,
	log *logger.Logger,
) *eventStorageAlertManager {
	return newEventStorageAlertManager(alertspostgres.NewRepository(db.DB()), notifier, silenceManager, log)
}

func newEventStorageAlertManager(
	repo alerts.Repository,
	notifier notifications.Service,
	silenceManager alerts.SilenceManager,
	log *logger.Logger,
) *eventStorageAlertManager {
	return &eventStorageAlertManager{
		alertRepo:      repo,
		notifier:       notifier,
		silenceManager: silenceManager,
		log:            log.WithComponent("event_storage_alert_manager"),
		state:          make(map[string]*eventStorageState),
	}
}

var _ keycloak.EventStorageReporter = (*eventStorageAlertManager)(nil)

// ReportEventStorage records one events cycle's outcome for a tenant.
func (m *eventStorageAlertManager) ReportEventStorage(ctx context.Context, tenantID, tenantName string, o keycloak.EventStorageOutcome) {
	if o.Attempted == 0 {
		return
	}

	m.mu.Lock()
	st, ok := m.state[tenantID]
	if !ok {
		// Unknown since startup: an alert may be left from before.
		st = &eventStorageState{alertOpen: true}
		m.state[tenantID] = st
	}
	if o.Failed > 0 {
		st.failingCycles++
		raise := st.failingCycles == eventStorageFailingCycles
		if raise {
			st.alertOpen = true
		}
		m.mu.Unlock()
		if raise {
			m.raise(ctx, tenantID, tenantName, o)
		}
		return
	}
	st.failingCycles = 0
	resolve := st.alertOpen
	st.alertOpen = false
	m.mu.Unlock()
	if resolve {
		m.resolve(ctx, tenantID, tenantName)
	}
}

func (m *eventStorageAlertManager) raise(ctx context.Context, tenantID, tenantName string, o keycloak.EventStorageOutcome) {
	alertID := eventStorageAlertID(tenantID)
	errText := ""
	if o.FirstError != nil {
		errText = o.FirstError.Error()
	}
	metadata, _ := json.Marshal(map[string]string{
		"tenant_id":   tenantID,
		"tenant_name": tenantName,
		"failed":      fmt.Sprintf("%d", o.Failed),
		"attempted":   fmt.Sprintf("%d", o.Attempted),
		"first_error": errText,
		"check_type":  "event_storage",
	})

	now := time.Now()
	alert := &domain.Alert{
		TenantID:     tenantID,
		AlertID:      alertID,
		Source:       domain.AlertSourceMetric,
		Type:         domain.AlertTypePerformance,
		Severity:     domain.AlertSeverityCritical,
		Status:       domain.AlertStatusActive,
		Title:        fmt.Sprintf("Events for tenant '%s' are not being stored", tenantName),
		Description:  fmt.Sprintf("Storing polled Keycloak events for tenant '%s' (ID: %s) has failed in %d events cycles in a row; in the last one, %d of %d writes failed. Each cycle retries events still inside the lookback window, so events older than that are lost while this lasts. First error: %s", tenantName, tenantID, eventStorageFailingCycles, o.Failed, o.Attempted, errText),
		ResourceType: "tenant",
		ResourceID:   tenantID,
		ResourceName: tenantName,
		CheckType:    "event_storage",
		Recommendation: "Check the platform database: that it is reachable and has disk space, and that the events tables' indexes and constraints are as the release expects. " +
			"The server log lines 'Failed to save Keycloak events' and 'Failed to save events to the events table' name the realm and the error.",
		Metadata:      string(metadata),
		FirstDetected: now,
		LastSeen:      now,
	}

	if m.silenceManager != nil {
		if silenced, _ := m.silenceManager.ShouldSilence(alert); silenced {
			m.log.Info("Event storage alert silenced by rule",
				logger.Str("tenant_id", tenantID), logger.Str("alert_id", alertID))
			return
		}
	}

	if err := m.alertRepo.Save(ctx, alert); err != nil {
		m.log.Error("Failed to save event storage alert",
			logger.Err(err), logger.Str("tenant_id", tenantID))
		return
	}
	m.log.Info("Created event storage alert",
		logger.Str("tenant_id", tenantID), logger.Str("alert_id", alertID))

	// Raised once per run of failing cycles, and the notification service
	// sends at most once per occurrence, so a run that outlasts a restart is
	// not announced again.
	if m.notifier != nil {
		if err := m.notifier.NotifyAlert(ctx, alert); err != nil {
			m.log.Error("Failed to send event storage notification",
				logger.Err(err), logger.Str("tenant_id", tenantID))
		}
	}
}

// resolve resolves the tenant's alert once its events are stored again, when
// it is active or acknowledged. An acknowledged alert must resolve too: a new
// occurrence starts only from a resolved alert, so one left acknowledged would
// make the next outage reopen it without being announced. An alert someone
// chose to ignore is left as they set it.
func (m *eventStorageAlertManager) resolve(ctx context.Context, tenantID, tenantName string) {
	alertID := eventStorageAlertID(tenantID)
	alert, err := m.alertRepo.GetByAlertID(ctx, tenantID, alertID)
	if err != nil || alert == nil ||
		(alert.Status != domain.AlertStatusActive && alert.Status != domain.AlertStatusAcknowled) {
		return
	}
	if err := m.alertRepo.Resolve(ctx, tenantID, alertID); err != nil {
		m.log.Error("Failed to resolve event storage alert",
			logger.Err(err), logger.Str("tenant_id", tenantID))
		m.mu.Lock()
		if st := m.state[tenantID]; st != nil {
			st.alertOpen = true // try again after the next cycle
		}
		m.mu.Unlock()
		return
	}
	m.log.Info("Resolved event storage alert",
		logger.Str("tenant_id", tenantID), logger.Str("alert_id", alertID))

	if m.notifier != nil {
		now := time.Now()
		alert.Status = domain.AlertStatusResolved
		alert.ResolvedAt = &now
		alert.Description = fmt.Sprintf("%s\n\nEvents for tenant '%s' are being stored again since %s.",
			alert.Description, tenantName, now.Format(time.RFC3339))
		if err := m.notifier.NotifyAlertResolution(ctx, alert); err != nil {
			m.log.Error("Failed to send event storage recovery notification",
				logger.Err(err), logger.Str("tenant_id", tenantID))
		}
	}
}
