package fx

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"go.uber.org/fx"

	"github.com/DefensePoint/keycloak-monitoring/internal/alerts"
	alertspostgres "github.com/DefensePoint/keycloak-monitoring/internal/alerts/postgres"
	"github.com/DefensePoint/keycloak-monitoring/internal/config"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/events"
	"github.com/DefensePoint/keycloak-monitoring/internal/keycloak"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/internal/metrics"
	"github.com/DefensePoint/keycloak-monitoring/internal/notifications"
	"github.com/DefensePoint/keycloak-monitoring/internal/tenant"
	"github.com/DefensePoint/keycloak-monitoring/pkg/database"
	"github.com/DefensePoint/keycloak-monitoring/pkg/keycloakadmin"
	"github.com/DefensePoint/keycloak-monitoring/pkg/saferequest"
)

// MonitoringModule provides the monitoring infrastructure (MonitorPoolManager, ConfigChecker, etc.)
// This is the CORE of the application - it starts goroutines that collect data from Keycloak instances.
var MonitoringModule = fx.Module("monitoring",
	fx.Provide(
		// Provide silence manager for alerts
		provideSilenceManager,

		// Provide event alert converter (converts keycloak events to alerts)
		provideEventAlertConverter,

		// Provide health alert manager (creates alerts from health status)
		provideHealthAlertManager,

		// Provide keycloak monitor factory
		provideKeycloakMonitorFactory,

		// Provide the MonitorPoolManager
		provideMonitorPoolManager,
	),
	// Register lifecycle hooks to start/stop monitoring
	fx.Invoke(registerMonitoringHooks),
)

// ============================================================================
// SILENCE MANAGER
// ============================================================================

// provideSilenceManager creates the silence manager and loads config.
func provideSilenceManager(cfg *config.AppConfig, log *logger.Logger) alerts.SilenceManager {
	sm := alerts.NewSilenceManager(log)

	// Load silence rules from config
	if err := sm.LoadSilences(context.Background(), cfg.AlertSilences); err != nil {
		log.Error("Failed to load silence rules", logger.Err(err))
	}

	return sm
}

// ============================================================================
// EVENT ALERT CONVERTER (converts Keycloak events to alerts)
// ============================================================================

// eventAlertConverterImpl converts Keycloak events to platform alerts.
// Implements both tenant.EventAlertConverter and keycloak.EventAlertConverter.
//
// Key behaviors:
// - Filters events by severity (ERROR, UPDATE, DELETE generate alerts)
// - Uses deterministic AlertID for deduplication (same event type+realm+client = same alert)
// - Checks silence rules before creating alerts
// - Sends notifications for new alerts
type eventAlertConverterImpl struct {
	alertRepo      alerts.Repository
	notifier       notifications.Service
	silenceManager alerts.SilenceManager
	log            *logger.Logger
}

func provideEventAlertConverter(
	db *database.Client,
	notifier notifications.Service,
	silenceManager alerts.SilenceManager,
	log *logger.Logger,
) *eventAlertConverterImpl {
	return &eventAlertConverterImpl{
		alertRepo:      alertspostgres.NewRepository(db.DB()),
		notifier:       notifier,
		silenceManager: silenceManager,
		log:            log.WithComponent("event_alert_converter"),
	}
}

// ConvertEvent converts a Keycloak event to an alert if needed.
// Only alertable events (ERROR, UPDATE, DELETE) generate alerts.
// Uses deterministic AlertID for deduplication - same event type/realm/client = same alert.
func (c *eventAlertConverterImpl) ConvertEvent(ctx context.Context, event *domain.KeycloakEvent) error {
	// Determine severity - skip if not alertable
	severity := c.mapEventTypeToSeverity(event.EventType)
	if severity == "" {
		// Not an alertable event (info level like LOGIN, LOGOUT)
		return nil
	}

	// Build metadata with event details for silence matching and context
	metadata := map[string]string{
		"client_id":  event.ClientID,
		"username":   event.Username,
		"user_id":    event.UserID,
		"ip_address": event.IPAddress,
		"session_id": event.SessionID,
	}
	metadataJSON, _ := json.Marshal(metadata)

	// Generate deterministic AlertID for deduplication
	// Same tenant+event_type+realm+client = same AlertID = updates existing alert
	alertID := c.generateAlertID(event)

	// Create alert from event
	alert := &domain.Alert{
		TenantID:       event.TenantID,
		AlertID:        alertID,
		Source:         domain.AlertSourceEvent,
		Type:           domain.AlertTypeEvent,
		Severity:       severity,
		Status:         domain.AlertStatusActive,
		Title:          fmt.Sprintf("%s in %s", event.EventType, event.RealmName),
		Description:    fmt.Sprintf("Event type: %s, User: %s, Client: %s", event.EventType, event.UserID, event.ClientID),
		ResourceType:   "event",
		ResourceID:     event.EventID,
		ResourceName:   event.EventType,
		RealmName:      event.RealmName,
		CheckType:      "event_monitor",
		EventID:        &event.EventID,
		Recommendation: c.getRecommendationForEvent(event),
		Metadata:       string(metadataJSON),
		FirstDetected:  event.Time,
		LastSeen:       event.Time,
	}

	// Check if alert should be silenced
	if c.silenceManager != nil {
		silenced, silenceID := c.silenceManager.ShouldSilence(alert)
		if silenced {
			c.log.Debug("Alert silenced",
				logger.Str("alert_id", alert.AlertID),
				logger.Str("silence_id", silenceID),
				logger.Str("event_type", event.EventType))
			return nil
		}
	}

	// Check if alert already exists (for logging purposes - Save does upsert)
	existingAlert, _ := c.alertRepo.GetByAlertID(ctx, alert.TenantID, alert.AlertID)
	isNewAlert := existingAlert == nil

	// Save alert (upsert - creates new or updates last_seen)
	if err := c.alertRepo.Save(ctx, alert); err != nil {
		return fmt.Errorf("failed to save alert: %w", err)
	}

	if isNewAlert {
		c.log.Info("Created alert from event",
			logger.Str("alert_id", alert.AlertID),
			logger.Str("event_type", event.EventType),
			logger.Str("severity", string(alert.Severity)))

		// Send notification only for new alerts
		if c.notifier != nil {
			if err := c.notifier.NotifyAlert(ctx, alert); err != nil {
				c.log.Error("Failed to send alert notification",
					logger.Err(err),
					logger.Str("alert_id", alert.AlertID))
				// Don't fail the whole operation - alert was saved
			}
		}
	} else {
		c.log.Debug("Updated existing alert",
			logger.Str("alert_id", alert.AlertID),
			logger.Str("event_type", event.EventType))
	}

	return nil
}

// generateAlertID generates a deterministic alert ID for deduplication.
// Same tenant+event_type+realm+client = same AlertID.
func (c *eventAlertConverterImpl) generateAlertID(event *domain.KeycloakEvent) string {
	data := fmt.Sprintf("%s:%s:%s:%s",
		event.TenantID,
		event.EventType,
		event.RealmName,
		event.ClientID)
	hash := sha256.Sum256([]byte(data))
	return fmt.Sprintf("%x", hash[:16])
}

// mapEventTypeToSeverity maps event types to severity levels.
// Returns empty string for non-alertable events (LOGIN, LOGOUT, etc).
func (c *eventAlertConverterImpl) mapEventTypeToSeverity(eventType string) domain.AlertSeverity {
	switch {
	case strings.Contains(eventType, "ERROR"):
		return domain.AlertSeverityError
	case strings.Contains(eventType, "UPDATE"):
		return domain.AlertSeverityWarning
	case strings.Contains(eventType, "DELETE"):
		return domain.AlertSeverityWarning
	default:
		// INFO level events (LOGIN, LOGOUT, CREATE, etc.) - don't alert
		return ""
	}
}

// getRecommendationForEvent returns a recommendation based on event type.
func (c *eventAlertConverterImpl) getRecommendationForEvent(event *domain.KeycloakEvent) string {
	switch {
	case strings.Contains(event.EventType, "LOGIN_ERROR"):
		return "Review failed login attempts. Check for brute force attacks or misconfigured clients."
	case strings.Contains(event.EventType, "CLIENT_LOGIN_ERROR"):
		return "Verify client credentials and configuration. Check client secret validity."
	case strings.Contains(event.EventType, "CODE_TO_TOKEN_ERROR"):
		return "Check OAuth2 flow configuration. Verify redirect URIs and client settings."
	case strings.Contains(event.EventType, "UPDATE"):
		return "Review the configuration change and verify it was intentional."
	case strings.Contains(event.EventType, "DELETE"):
		return "Verify the deletion was authorized and intentional."
	default:
		return "Review event details and take appropriate action."
	}
}

// ============================================================================
// HEALTH ALERT MANAGER (creates alerts from health status)
// ============================================================================

// healthAlertManagerImpl manages health-related alerts for tenants.
// Implements tenant.HealthAlertManager.
//
// Key behaviors:
// - Tracks previous health status to detect transitions (UP -> DOWN, DOWN -> UP, etc.)
// - Only alerts on status TRANSITIONS, not on every check
// - Uses separate AlertIDs for DOWN and DEGRADED states
// - Includes rate limiting for notifications (5 min between alerts)
// - Sends notifications for new alerts and recoveries
type healthAlertManagerImpl struct {
	alertRepo      alerts.Repository
	notifier       notifications.Service
	silenceManager alerts.SilenceManager
	log            *logger.Logger

	// Track previous health status to detect transitions
	mu              sync.RWMutex
	previousStatus  map[string]string    // tenantID -> last known status
	lastAlertSentAt map[string]time.Time // alertID -> last notification time
}

func provideHealthAlertManager(
	db *database.Client,
	notifier notifications.Service,
	silenceManager alerts.SilenceManager,
	log *logger.Logger,
) *healthAlertManagerImpl {
	return &healthAlertManagerImpl{
		alertRepo:       alertspostgres.NewRepository(db.DB()),
		notifier:        notifier,
		silenceManager:  silenceManager,
		log:             log.WithComponent("health_alert_manager"),
		previousStatus:  make(map[string]string),
		lastAlertSentAt: make(map[string]time.Time),
	}
}

// ProcessHealthStatus processes a health check result and generates alerts on status transitions.
// Status values from keycloak.Client.GetHealthMetrics: "UP", "DOWN", "DEGRADED"
func (m *healthAlertManagerImpl) ProcessHealthStatus(ctx context.Context, tenantID, tenantName, status, errorMessage string, responseTime int) error {
	m.mu.Lock()
	previousStatus, hasPrevious := m.previousStatus[tenantID]
	m.previousStatus[tenantID] = status
	m.mu.Unlock()

	// If this is the first check, don't alert yet (wait for transition)
	if !hasPrevious {
		m.log.Debug("First health check for tenant, establishing baseline",
			logger.Str("tenant_id", tenantID),
			logger.Str("status", status))
		return nil
	}

	// No change in status - nothing to do
	if previousStatus == status {
		return nil
	}

	// Status changed - log the transition
	m.log.Info("Tenant health status changed",
		logger.Str("tenant_id", tenantID),
		logger.Str("tenant_name", tenantName),
		logger.Str("previous_status", previousStatus),
		logger.Str("current_status", status))

	// Handle transitions
	switch {
	case status == "DOWN":
		return m.createTenantDownAlert(ctx, tenantID, tenantName, errorMessage, responseTime)

	case previousStatus == "DOWN" && (status == "UP" || status == "DEGRADED"):
		return m.resolveTenantDownAlert(ctx, tenantID, tenantName, status)

	case status == "DEGRADED" && previousStatus == "UP":
		return m.createTenantDegradedAlert(ctx, tenantID, tenantName, errorMessage, responseTime)

	case previousStatus == "DEGRADED" && status == "UP":
		return m.resolveTenantDegradedAlert(ctx, tenantID, tenantName)
	}

	return nil
}

// createTenantDownAlert creates a critical alert when a tenant goes down.
func (m *healthAlertManagerImpl) createTenantDownAlert(ctx context.Context, tenantID, tenantName, errorMessage string, responseTime int) error {
	alertID := fmt.Sprintf("tenant-health-down-%s", tenantID)

	// Build metadata
	metadata := map[string]string{
		"tenant_id":     tenantID,
		"tenant_name":   tenantName,
		"status":        "DOWN",
		"error_message": errorMessage,
		"response_time": fmt.Sprintf("%dms", responseTime),
		"check_type":    "health",
	}
	metadataJSON, _ := json.Marshal(metadata)

	now := time.Now()
	alert := &domain.Alert{
		TenantID:       tenantID,
		AlertID:        alertID,
		Source:         domain.AlertSourceMetric,
		Type:           domain.AlertTypePerformance,
		Severity:       domain.AlertSeverityCritical,
		Status:         domain.AlertStatusActive,
		Title:          fmt.Sprintf("Tenant '%s' is DOWN", tenantName),
		Description:    fmt.Sprintf("Tenant '%s' (ID: %s) health check failed and is now marked as DOWN. Error: %s", tenantName, tenantID, errorMessage),
		ResourceType:   "tenant",
		ResourceID:     tenantID,
		ResourceName:   tenantName,
		CheckType:      "health_check",
		Recommendation: "Check tenant server availability and network connectivity. Review tenant logs for errors. Verify Keycloak service is running.",
		Metadata:       string(metadataJSON),
		FirstDetected:  now,
		LastSeen:       now,
	}

	// Check if alert should be silenced
	if m.silenceManager != nil {
		if silenced, _ := m.silenceManager.ShouldSilence(alert); silenced {
			m.log.Info("Tenant down alert silenced by rule",
				logger.Str("tenant_id", tenantID),
				logger.Str("alert_id", alertID))
			return nil
		}
	}

	// Save alert
	if err := m.alertRepo.Save(ctx, alert); err != nil {
		m.log.Error("Failed to save tenant down alert",
			logger.Err(err),
			logger.Str("tenant_id", tenantID))
		return fmt.Errorf("failed to save tenant down alert: %w", err)
	}

	m.log.Info("Created tenant down alert",
		logger.Str("tenant_id", tenantID),
		logger.Str("alert_id", alertID))

	// Send notification with rate limiting
	if m.notifier != nil {
		m.mu.Lock()
		lastSent := m.lastAlertSentAt[alertID]
		m.mu.Unlock()

		// Only send if we haven't sent recently (prevent spam)
		if now.Sub(lastSent) > 5*time.Minute {
			if err := m.notifier.NotifyAlert(ctx, alert); err != nil {
				m.log.Error("Failed to send tenant down notification",
					logger.Err(err),
					logger.Str("tenant_id", tenantID))
			} else {
				m.mu.Lock()
				m.lastAlertSentAt[alertID] = now
				m.mu.Unlock()
				m.log.Info("Sent tenant down notification",
					logger.Str("tenant_id", tenantID))
			}
		}
	}

	return nil
}

// resolveTenantDownAlert resolves the down alert when tenant recovers.
func (m *healthAlertManagerImpl) resolveTenantDownAlert(ctx context.Context, tenantID, tenantName, recoveryStatus string) error {
	alertID := fmt.Sprintf("tenant-health-down-%s", tenantID)

	// Find the active alert
	alert, err := m.alertRepo.GetByAlertID(ctx, tenantID, alertID)
	if err != nil || alert == nil {
		m.log.Debug("No tenant down alert to resolve",
			logger.Str("tenant_id", tenantID),
			logger.Str("alert_id", alertID))
		return nil
	}

	// Only resolve if currently active
	if alert.Status != domain.AlertStatusActive {
		m.log.Debug("Tenant down alert already resolved",
			logger.Str("tenant_id", tenantID),
			logger.Str("alert_id", alertID))
		return nil
	}

	// Resolve the alert
	if err := m.alertRepo.Resolve(ctx, tenantID, alertID); err != nil {
		m.log.Error("Failed to resolve tenant down alert",
			logger.Err(err),
			logger.Str("tenant_id", tenantID))
		return fmt.Errorf("failed to resolve tenant down alert: %w", err)
	}

	m.log.Info("Resolved tenant down alert",
		logger.Str("tenant_id", tenantID),
		logger.Str("alert_id", alertID),
		logger.Str("recovery_status", recoveryStatus))

	// Send recovery notification
	if m.notifier != nil {
		// Update alert for notification
		alert.Status = domain.AlertStatusResolved
		now := time.Now()
		alert.ResolvedAt = &now
		alert.Description = fmt.Sprintf("%s\n\nRecovered at %s with status: %s",
			alert.Description, now.Format(time.RFC3339), recoveryStatus)

		if err := m.notifier.NotifyAlertResolution(ctx, alert); err != nil {
			m.log.Error("Failed to send tenant recovery notification",
				logger.Err(err),
				logger.Str("tenant_id", tenantID))
		}
	}

	return nil
}

// createTenantDegradedAlert creates a warning alert when tenant is degraded.
func (m *healthAlertManagerImpl) createTenantDegradedAlert(ctx context.Context, tenantID, tenantName, errorMessage string, responseTime int) error {
	alertID := fmt.Sprintf("tenant-health-degraded-%s", tenantID)

	// Build metadata
	metadata := map[string]string{
		"tenant_id":     tenantID,
		"tenant_name":   tenantName,
		"status":        "DEGRADED",
		"error_message": errorMessage,
		"response_time": fmt.Sprintf("%dms", responseTime),
		"check_type":    "health",
	}
	metadataJSON, _ := json.Marshal(metadata)

	now := time.Now()
	alert := &domain.Alert{
		TenantID:       tenantID,
		AlertID:        alertID,
		Source:         domain.AlertSourceMetric,
		Type:           domain.AlertTypePerformance,
		Severity:       domain.AlertSeverityWarning,
		Status:         domain.AlertStatusActive,
		Title:          fmt.Sprintf("Tenant '%s' is DEGRADED", tenantName),
		Description:    fmt.Sprintf("Tenant '%s' (ID: %s) health check indicates degraded performance. Response time: %dms. Message: %s", tenantName, tenantID, responseTime, errorMessage),
		ResourceType:   "tenant",
		ResourceID:     tenantID,
		ResourceName:   tenantName,
		CheckType:      "health_check",
		Recommendation: "Monitor tenant performance. Check for high load or resource constraints. Review application logs.",
		Metadata:       string(metadataJSON),
		FirstDetected:  now,
		LastSeen:       now,
	}

	// Check if alert should be silenced
	if m.silenceManager != nil {
		if silenced, _ := m.silenceManager.ShouldSilence(alert); silenced {
			m.log.Info("Tenant degraded alert silenced by rule",
				logger.Str("tenant_id", tenantID),
				logger.Str("alert_id", alertID))
			return nil
		}
	}

	// Save alert
	if err := m.alertRepo.Save(ctx, alert); err != nil {
		m.log.Error("Failed to save tenant degraded alert",
			logger.Err(err),
			logger.Str("tenant_id", tenantID))
		return fmt.Errorf("failed to save tenant degraded alert: %w", err)
	}

	m.log.Info("Created tenant degraded alert",
		logger.Str("tenant_id", tenantID),
		logger.Str("alert_id", alertID))

	// Send notification
	if m.notifier != nil {
		if err := m.notifier.NotifyAlert(ctx, alert); err != nil {
			m.log.Error("Failed to send tenant degraded notification",
				logger.Err(err),
				logger.Str("tenant_id", tenantID))
		}
	}

	return nil
}

// resolveTenantDegradedAlert resolves the degraded alert when tenant recovers to UP.
func (m *healthAlertManagerImpl) resolveTenantDegradedAlert(ctx context.Context, tenantID, tenantName string) error {
	alertID := fmt.Sprintf("tenant-health-degraded-%s", tenantID)

	alert, err := m.alertRepo.GetByAlertID(ctx, tenantID, alertID)
	if err != nil || alert == nil {
		m.log.Debug("No tenant degraded alert to resolve",
			logger.Str("tenant_id", tenantID),
			logger.Str("alert_id", alertID))
		return nil
	}

	if alert.Status != domain.AlertStatusActive {
		m.log.Debug("Tenant degraded alert already resolved",
			logger.Str("tenant_id", tenantID),
			logger.Str("alert_id", alertID))
		return nil
	}

	// Resolve the alert
	if err := m.alertRepo.Resolve(ctx, tenantID, alertID); err != nil {
		m.log.Error("Failed to resolve tenant degraded alert",
			logger.Err(err),
			logger.Str("tenant_id", tenantID))
		return fmt.Errorf("failed to resolve tenant degraded alert: %w", err)
	}

	m.log.Info("Resolved tenant degraded alert",
		logger.Str("tenant_id", tenantID),
		logger.Str("alert_id", alertID))

	// Send recovery notification
	if m.notifier != nil {
		alert.Status = domain.AlertStatusResolved
		now := time.Now()
		alert.ResolvedAt = &now
		alert.Description = fmt.Sprintf("%s\n\nRecovered to UP status at %s", alert.Description, now.Format(time.RFC3339))

		if err := m.notifier.NotifyAlertResolution(ctx, alert); err != nil {
			m.log.Error("Failed to send tenant recovery notification",
				logger.Err(err),
				logger.Str("tenant_id", tenantID))
		}
	}

	return nil
}

// ResetTenantStatus resets the tracked status for a tenant (useful for testing or tenant removal).
func (m *healthAlertManagerImpl) ResetTenantStatus(tenantID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.previousStatus, tenantID)
	delete(m.lastAlertSentAt, fmt.Sprintf("tenant-health-down-%s", tenantID))
	delete(m.lastAlertSentAt, fmt.Sprintf("tenant-health-degraded-%s", tenantID))
}

// ============================================================================
// KEYCLOAK MONITOR FACTORY
// ============================================================================

// keycloakMonitorFactory creates Keycloak clients and monitors
type keycloakMonitorFactory struct {
	log           *logger.Logger
	cfg           *config.AppConfig
	healthUpdater keycloak.TenantHealthUpdater
	metrics       metrics.Recorder
}

func provideKeycloakMonitorFactory(cfg *config.AppConfig, log *logger.Logger, svc tenant.Service, m *metrics.Registry) tenant.KeycloakMonitorFactory {
	return &keycloakMonitorFactory{log: log, cfg: cfg, healthUpdater: svc, metrics: m}
}

// CreateClient creates a new Keycloak client for a tenant
func (f *keycloakMonitorFactory) CreateClient(ctx context.Context, cfg *tenant.MonitorConfig, instanceCfg any) (tenant.KeycloakClient, error) {
	kcConfig, ok := instanceCfg.(*config.KeycloakInstanceConfig)
	if !ok {
		// Use default config if not provided
		kcConfig = &config.KeycloakInstanceConfig{
			Polling: config.KeycloakPollingConfig{
				MetricsInterval:   5 * time.Minute,
				EventsInterval:    30 * time.Second,
				HealthInterval:    1 * time.Minute,
				RealmInfoInterval: 10 * time.Minute,
			},
			Connection: config.KeycloakConnectionConfig{
				Timeout:      30 * time.Second,
				MaxRetries:   3,
				RetryBackoff: 1 * time.Second,
			},
		}
	}

	// SSRF guard at dial time: every outbound request from this client
	// re-checks the resolved IP against the policy. This is the defense
	// that catches DNS rebinding and tenants whose stored URL was tampered
	// with after passing the create-time validator.
	policy := saferequest.Policy{
		AllowPrivateRanges: f.cfg.Keycloak.Global.Connection.AllowPrivateRanges,
		Timeout:            kcConfig.Connection.Timeout,
		SkipTLSVerify:      kcConfig.Connection.SkipTLSVerify,
	}

	// Build keycloakadmin.ClientConfig from tenant.MonitorConfig and config.KeycloakInstanceConfig
	clientConfig := &keycloakadmin.ClientConfig{
		ServerURL:             cfg.ServerURL,
		AdminRealm:            cfg.AdminRealm,
		ClientID:              cfg.ClientID,
		ClientSecret:          cfg.ClientSecret,
		Timeout:               kcConfig.Connection.Timeout,
		SkipTLSVerify:         kcConfig.Connection.SkipTLSVerify,
		MaxRetries:            kcConfig.Connection.MaxRetries,
		RetryBackoff:          kcConfig.Connection.RetryBackoff,
		Realms:                kcConfig.Realms,
		EventTypes:            kcConfig.Events.Types,
		EventLookbackDuration: kcConfig.Events.LookbackDuration,
		MaxEventsPerPoll:      kcConfig.Events.MaxEventsPerPoll,
		InfinispanPort:        kcConfig.InfinispanPort,
		HTTPClient:            saferequest.HTTPClient(policy),
	}

	// Create logger adapter for keycloakadmin
	loggerAdapter := &keycloakadminLoggerAdapter{log: f.log}

	client, err := keycloakadmin.NewClient(ctx, clientConfig, loggerAdapter)
	if err != nil {
		return nil, err
	}

	return &keycloakClientAdapter{client: client}, nil
}

// CreateMonitor creates a new Keycloak monitor for a tenant
func (f *keycloakMonitorFactory) CreateMonitor(
	tenantID string,
	tenantName string,
	client tenant.KeycloakClient,
	instanceCfg any,
	eventRepo tenant.EventRepository,
	generalEventRepo tenant.GeneralEventRepository,
	metricsRepo tenant.MetricsRepository,
	healthRepo tenant.HealthRepository,
	realmRepo tenant.RealmRepository,
	eventAlertConverter tenant.EventAlertConverter,
	healthAlertManager tenant.HealthAlertManager,
) tenant.KeycloakMonitor {
	kcConfig, ok := instanceCfg.(*config.KeycloakInstanceConfig)
	if !ok {
		kcConfig = &config.KeycloakInstanceConfig{
			Polling: config.KeycloakPollingConfig{
				MetricsInterval:   5 * time.Minute,
				EventsInterval:    30 * time.Second,
				HealthInterval:    1 * time.Minute,
				RealmInfoInterval: 10 * time.Minute,
			},
		}
	}

	// Get the underlying keycloak client
	kcClient := client.(*keycloakClientAdapter).client

	// Create monitor - interfaces are now compatible (both use domain types)
	mon := keycloak.NewMonitor(
		tenantID,
		tenantName,
		kcClient,
		kcConfig,
		f.log,
		eventRepo,        // tenant.EventRepository is compatible with keycloak.MonitorEventRepository
		generalEventRepo, // tenant.GeneralEventRepository is compatible with keycloak.EventSaver
		metricsRepo,      // tenant.MetricsRepository is compatible with keycloak.MonitorMetricsRepository
		healthRepo,       // tenant.HealthRepository is compatible with keycloak.MonitorHealthRepository
		realmRepo,        // tenant.RealmRepository is compatible with keycloak.MonitorRealmRepository
		eventAlertConverter,
		healthAlertManager,
		keycloak.WithTenantHealthUpdater(f.healthUpdater),
	)
	mon.SetMetrics(f.metrics)

	return &keycloakMonitorAdapter{monitor: mon}
}

// ============================================================================
// MINIMAL ADAPTERS (only for bridging interface types, no data conversion)
// ============================================================================

// keycloakClientAdapter adapts keycloakadmin.Client to tenant.KeycloakClient
type keycloakClientAdapter struct {
	client *keycloakadmin.Client
}

func (a *keycloakClientAdapter) IsHealthy(ctx context.Context) bool {
	health := a.client.GetHealthMetrics(ctx)
	return health.Status == "UP"
}

func (a *keycloakClientAdapter) Close() error {
	return a.client.Close()
}

// keycloakadminLoggerAdapter adapts *logger.Logger to keycloakadmin.Logger
type keycloakadminLoggerAdapter struct {
	log *logger.Logger
}

func (a *keycloakadminLoggerAdapter) Info(msg string, fields ...any) {
	a.log.Info(msg, convertFieldsToLoggerFields(fields)...)
}
func (a *keycloakadminLoggerAdapter) Error(msg string, fields ...any) {
	a.log.Error(msg, convertFieldsToLoggerFields(fields)...)
}
func (a *keycloakadminLoggerAdapter) Warn(msg string, fields ...any) {
	a.log.Warn(msg, convertFieldsToLoggerFields(fields)...)
}
func (a *keycloakadminLoggerAdapter) Debug(msg string, fields ...any) {
	a.log.Debug(msg, convertFieldsToLoggerFields(fields)...)
}

// keycloakMonitorAdapter adapts keycloak.Monitor to tenant.KeycloakMonitor
type keycloakMonitorAdapter struct {
	monitor *keycloak.Monitor
}

func (a *keycloakMonitorAdapter) Start(ctx context.Context) error {
	return a.monitor.Start(ctx)
}

func (a *keycloakMonitorAdapter) Stop() error {
	return a.monitor.Stop()
}

// ============================================================================
// MONITOR POOL MANAGER PROVIDER
// ============================================================================

// MonitorPoolManagerParams contains all dependencies for MonitorPoolManager
type MonitorPoolManagerParams struct {
	fx.In

	TenantService       tenant.Service
	MonitorFactory      tenant.KeycloakMonitorFactory
	Config              *config.AppConfig
	Logger              *logger.Logger
	EventAlertConverter *eventAlertConverterImpl
	HealthAlertManager  *healthAlertManagerImpl

	// Repositories (now directly compatible with tenant interfaces)
	KcEventRepo   keycloak.EventRepository
	EventRepo     events.Repository
	KcMetricsRepo keycloak.MetricsRepository
	KcHealthRepo  keycloak.HealthRepository
	KcRealmRepo   keycloak.RealmRepository
}

func provideMonitorPoolManager(p MonitorPoolManagerParams) *tenant.MonitorPoolManager {
	// Create logger adapter (still needed for interface bridging)
	loggerAdapter := &tenantLoggerAdapter{log: p.Logger}

	return tenant.NewMonitorPoolManager(
		p.TenantService,
		p.MonitorFactory,
		&p.Config.Keycloak.Global,
		loggerAdapter,
		p.KcEventRepo,         // Direct pass-through (compatible interfaces)
		p.EventRepo,           // Direct pass-through (compatible interfaces)
		p.KcMetricsRepo,       // Direct pass-through (compatible interfaces)
		p.KcHealthRepo,        // Direct pass-through (compatible interfaces)
		p.KcRealmRepo,         // Direct pass-through (compatible interfaces)
		p.EventAlertConverter, // Direct pass-through (compatible interfaces)
		p.HealthAlertManager,  // Direct pass-through (compatible interfaces)
	)
}

// ============================================================================
// LOGGER ADAPTER (still needed for interface bridging)
// ============================================================================

// tenantLoggerAdapter adapts *logger.Logger to tenant.Logger
type tenantLoggerAdapter struct {
	log *logger.Logger
}

func (a *tenantLoggerAdapter) Info(msg string, fields ...any) {
	a.log.Info(msg, convertFieldsToLoggerFields(fields)...)
}
func (a *tenantLoggerAdapter) Error(msg string, fields ...any) {
	a.log.Error(msg, convertFieldsToLoggerFields(fields)...)
}
func (a *tenantLoggerAdapter) Warn(msg string, fields ...any) {
	a.log.Warn(msg, convertFieldsToLoggerFields(fields)...)
}
func (a *tenantLoggerAdapter) Debug(msg string, fields ...any) {
	a.log.Debug(msg, convertFieldsToLoggerFields(fields)...)
}

// convertFieldsToLoggerFields converts key-value pairs to logger.Field slice
func convertFieldsToLoggerFields(fields []any) []logger.Field {
	if len(fields) == 0 {
		return nil
	}
	result := make([]logger.Field, 0, len(fields)/2)
	for i := 0; i < len(fields)-1; i += 2 {
		key, ok := fields[i].(string)
		if !ok {
			continue
		}
		value := fields[i+1]
		switch v := value.(type) {
		case string:
			result = append(result, logger.Str(key, v))
		case int:
			result = append(result, logger.Int(key, v))
		case int64:
			result = append(result, logger.Int(key, int(v)))
		case error:
			result = append(result, logger.Err(v))
		default:
			result = append(result, logger.Any(key, v))
		}
	}
	return result
}

// ============================================================================
// CLIENT PROVIDER ADAPTER
// ============================================================================

// monitorPoolClientProvider adapts MonitorPoolManager to keycloak.ClientProvider.
// This allows the keycloak.Service to access live Keycloak clients from the monitor pool.
type monitorPoolClientProvider struct {
	pool *tenant.MonitorPoolManager
}

// GetClient returns the Keycloak client for a specific tenant.
// It extracts the underlying *keycloakadmin.Client from the keycloakClientAdapter.
// The returned client implements keycloak.AdminAPI.
func (p *monitorPoolClientProvider) GetClient(tenantID string) keycloak.AdminAPI {
	client := p.pool.GetKeycloakClient(tenantID)
	if client == nil {
		return nil
	}
	// Type assertion to get the underlying keycloakadmin.Client from the adapter
	if adapter, ok := client.(*keycloakClientAdapter); ok {
		return adapter.client
	}
	return nil
}

// ============================================================================
// LIFECYCLE HOOKS
// ============================================================================

func registerMonitoringHooks(lc fx.Lifecycle, pool *tenant.MonitorPoolManager, tenantService tenant.Service, kcService keycloak.Service, log *logger.Logger) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			// STEP 1: Load all tenants into cache first
			log.Info("Loading tenants from database into cache")
			if err := tenantService.LoadTenants(ctx); err != nil {
				return fmt.Errorf("failed to load tenants: %w", err)
			}

			// STEP 2: Start the monitor pool (now the cache is populated)
			log.Info("Starting MonitorPoolManager - initializing goroutines for each tenant")
			if err := pool.Start(ctx); err != nil {
				return err
			}

			// STEP 3: Inject the ClientProvider into the keycloak.Service
			// This enables live Keycloak access (GetUsers, GetClients, etc.)
			clientProvider := &monitorPoolClientProvider{pool: pool}
			kcService.SetClientProvider(clientProvider)
			log.Info("Injected ClientProvider into keycloak.Service - live Keycloak access enabled")

			return nil
		},
		OnStop: func(ctx context.Context) error {
			log.Info("Stopping MonitorPoolManager - gracefully shutting down all tenant monitors")
			return pool.Stop()
		},
	})
}
