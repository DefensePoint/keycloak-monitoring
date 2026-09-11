package keycloak

import (
	"context"
	"fmt"
	"strconv"
	"time"

	eventsource "github.com/DefensePoint/keycloak-monitoring/events"
	"github.com/DefensePoint/keycloak-monitoring/internal/config"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/lifecycle"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/internal/metrics"
	"github.com/DefensePoint/keycloak-monitoring/pkg/keycloakadmin"
)

// Monitor represents the Keycloak monitoring service
type Monitor struct {
	tenantID       string
	tenantName     string
	client         AdminAPI
	instanceConfig *config.KeycloakInstanceConfig
	logger         *logger.Logger

	// Keycloak-specific repositories (using minimal Monitor interfaces)
	eventRepo   MonitorEventRepository
	metricsRepo MonitorMetricsRepository
	healthRepo  MonitorHealthRepository
	realmRepo   MonitorRealmRepository

	// Generic event repo (interface for dashboard events)
	generalEventRepo EventSaver

	// Event to alert converter
	eventAlertConverter EventAlertConverter

	// Health alert manager
	healthAlertManager HealthAlertManager

	// Metrics recorder (never nil; NopRecorder when metrics are disabled)
	metrics metrics.Recorder

	// Optional: keeps the tenant record's health_status current (best-effort).
	healthUpdater TenantHealthUpdater

	// workers tracks the polling workers Start launches so Stop can drain
	// them.
	workers lifecycle.Workers
}

// MonitorOption configures optional Monitor behavior.
type MonitorOption func(*Monitor)

// WithTenantHealthUpdater wires an updater that keeps the tenant record's
// health_status/last_error in sync with each health poll.
func WithTenantHealthUpdater(u TenantHealthUpdater) MonitorOption {
	return func(m *Monitor) { m.healthUpdater = u }
}

// mapHealthStatusToTenant translates poller status (UP/DOWN/...) to the
// tenant record vocabulary (healthy/unhealthy/degraded).
func mapHealthStatusToTenant(status string) string {
	switch status {
	case "UP":
		return "healthy"
	case "DOWN":
		return "unhealthy"
	default:
		return "degraded"
	}
}

// NewMonitor creates a new Keycloak monitor
func NewMonitor(
	tenantID string,
	tenantName string,
	client AdminAPI,
	instanceCfg *config.KeycloakInstanceConfig,
	log *logger.Logger,
	eventRepo MonitorEventRepository,
	generalEventRepo EventSaver,
	metricsRepo MonitorMetricsRepository,
	healthRepo MonitorHealthRepository,
	realmRepo MonitorRealmRepository,
	eventAlertConverter EventAlertConverter,
	healthAlertManager HealthAlertManager,
	opts ...MonitorOption,
) *Monitor {
	m := &Monitor{
		tenantID:            tenantID,
		tenantName:          tenantName,
		client:              client,
		instanceConfig:      instanceCfg,
		logger:              log,
		eventRepo:           eventRepo,
		generalEventRepo:    generalEventRepo,
		metricsRepo:         metricsRepo,
		healthRepo:          healthRepo,
		realmRepo:           realmRepo,
		eventAlertConverter: eventAlertConverter,
		healthAlertManager:  healthAlertManager,
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// Start begins the monitoring process
func (m *Monitor) Start(ctx context.Context) error {
	m.logger.Info("Starting Keycloak monitoring service")

	// Start all polling workers
	for _, poll := range []func(context.Context){
		m.pollEvents,
		m.pollMetrics,
		m.pollHealth,
		m.pollRealmInfo,
	} {
		poll := poll
		m.workers.Go(func() { poll(ctx) })
	}

	m.logger.Info("Keycloak monitoring service started successfully")
	return nil
}

// Stop gracefully stops the monitoring process
func (m *Monitor) Stop() error {
	m.logger.Info("Stopping Keycloak monitoring service")
	// Wait for all workers to finish (with timeout). A done channel used to
	// stand in for this, but nothing ever closed it, so every shutdown sat
	// here for the full 10s and then logged a spurious stop timeout.
	if m.workers.Drain(10 * time.Second) {
		m.logger.Info("Keycloak monitoring service stopped gracefully")
	} else {
		m.logger.Warn("Keycloak monitoring service stop timeout")
	}

	return nil
}

// pollEvents periodically fetches and stores Keycloak events
// SetMetrics attaches a metrics recorder. Passing nil installs a no-op
// recorder so call sites never need a nil check.
func (m *Monitor) SetMetrics(r metrics.Recorder) {
	if r == nil {
		r = metrics.NopRecorder{}
	}
	m.metrics = r
}

// recorder returns the configured recorder, falling back to a no-op when the
// monitor was constructed without one.
func (m *Monitor) recorder() metrics.Recorder {
	if m.metrics == nil {
		return metrics.NopRecorder{}
	}
	return m.metrics
}

func (m *Monitor) pollEvents(ctx context.Context) {
	ticker := time.NewTicker(m.instanceConfig.Polling.EventsInterval)
	defer ticker.Stop()

	m.logger.Info("Starting events polling worker",
		logger.Str("interval", m.instanceConfig.Polling.EventsInterval.String()))

	// Do first collection immediately
	m.collectEvents(ctx)

	for {
		select {
		case <-ticker.C:
			m.collectEvents(ctx)
		case <-m.workers.Stopping():
			m.logger.Info("Events polling worker stopped")
			return
		case <-ctx.Done():
			m.logger.Info("Events polling worker cancelled")
			return
		}
	}
}

// collectEvents fetches and stores events from Keycloak
func (m *Monitor) collectEvents(ctx context.Context) {
	start := time.Now()
	defer func() {
		m.recorder().ObservePollCycle(m.tenantID, "events", time.Since(start))
	}()

	m.logger.Info("🔍 [DEBUG] Starting event collection cycle",
		logger.Str("tenant_id", m.tenantID),
		logger.Str("tenant_name", m.tenantName))

	realms := m.getRealmsToMonitor(ctx)
	m.logger.Info("🔍 [DEBUG] Realms to monitor",
		logger.Str("tenant_id", m.tenantID),
		logger.Int("realm_count", len(realms)),
		logger.Strs("realms", realms))
	if len(realms) == 0 {
		m.logger.Warn("No realms to monitor")
		return
	}

	for _, realmName := range realms {
		events, err := m.client.GetRecentEvents(ctx, realmName)
		if err != nil {
			m.logger.Error("❌ [DEBUG] Failed to fetch events from Keycloak API",
				logger.Str("tenant_id", m.tenantID),
				logger.Str("realm", realmName),
				logger.Err(err))
			continue
		}

		m.logger.Info("✅ [DEBUG] Fetched events from Keycloak API",
			logger.Str("tenant_id", m.tenantID),
			logger.Str("realm", realmName),
			logger.Int("event_count", len(events)))

		if len(events) == 0 {
			m.logger.Info("ℹ️  [DEBUG] No new events in this polling cycle",
				logger.Str("tenant_id", m.tenantID),
				logger.Str("realm", realmName))
			continue
		}

		// Convert Keycloak events to domain types
		keycloakEvents := make([]*domain.KeycloakEvent, 0, len(events))
		generalEvents := make([]*domain.Event, 0, len(events))

		for _, event := range events {
			eventTime := time.UnixMilli(event.Time)
			// Prefer Keycloak's own event id (returned by the events API as
			// `id`). Fall back to a synthesized composite only for older
			// Keycloak versions that omit it, so the event_id unique key is
			// always populated.
			eventID := event.ID
			if eventID == "" {
				eventID = fmt.Sprintf("%s-%d-%s", event.RealmID, event.Time, event.Type)
			}

			// Use username/email from the event payload only. Keycloak records
			// "username" in the details for user-initiated events (LOGIN,
			// LOGIN_ERROR, etc.). Token-refresh and code-exchange events
			// (REFRESH_TOKEN, CODE_TO_TOKEN) do not include a username in
			// Details, so those events will have an empty username field.
			// A fallback GET /users/{id} call is intentionally omitted: for
			// federated realms it triggers getUserByUsername on the storage
			// provider while a service-account session is active in context,
			// which makes some custom user storage providers log a warning for
			// every event processed.
			username := event.Details["username"]
			email := event.Details["email"]

			// Create Keycloak-specific event (using local Event type)
			kcEvent := &domain.KeycloakEvent{
				TenantID:   m.tenantID,
				EventID:    eventID,
				Time:       eventTime,
				RealmID:    event.RealmID,
				RealmName:  realmName,
				ClientID:   event.ClientID,
				SessionID:  event.SessionID,
				IPAddress:  event.IPAddress,
				EventType:  event.Type,
				EventError: event.Error,
				UserID:     event.UserID,
				Username:   username,
				Email:      email,
				Details:    event.Details,
				Success:    event.Error == "",
			}
			keycloakEvents = append(keycloakEvents, kcEvent)

			// Create general event for dashboard visibility
			description := fmt.Sprintf("Keycloak %s event in realm %s", event.Type, realmName)
			if event.Error != "" {
				description += fmt.Sprintf(" (Error: %s)", event.Error)
			}

			severity := "info"
			if event.Error != "" {
				severity = "error"
			} else if event.Type == "LOGIN" || event.Type == "LOGOUT" {
				severity = "info"
			}

			generalEvent := &domain.Event{
				TenantID:     m.tenantID,
				EventID:      eventID,
				Timestamp:    eventTime,
				Type:         event.Type,
				Category:     "authentication",
				Severity:     severity,
				Description:  description,
				Source:       eventsource.SourceForKeycloakRealm(realmName),
				SourceIP:     event.IPAddress,
				SourceSystem: eventsource.SourceSystemKeycloak,
				UserID:       event.UserID,
				Username:     username,
				Email:        email,
				ClientID:     event.ClientID,
				Location:     "",
				RawData:      "",
				Status:       "processed",
				CreatedAt:    eventTime,
				UpdatedAt:    eventTime,
			}
			// The KMT adaptive-auth extension stamps these onto interactive
			// login events. amfa_event_id is the merge key with AMFA; risk_level
			// is already computed, so we surface it directly without an AMFA join.
			if amfaID := event.Details["amfa_event_id"]; amfaID != "" {
				generalEvent.AMFAEventID = &amfaID
			}
			if rl := event.Details["risk_level"]; rl != "" {
				if riskVal, perr := strconv.Atoi(rl); perr == nil {
					generalEvent.RiskLevel = &riskVal
				}
			}
			generalEvents = append(generalEvents, generalEvent)
		}

		// Save Keycloak events to database
		m.logger.Info("💾 [DEBUG] Saving Keycloak events to keycloak_events table",
			logger.Str("tenant_id", m.tenantID),
			logger.Str("realm", realmName),
			logger.Int("count", len(keycloakEvents)))

		if err := m.eventRepo.SaveEvents(ctx, keycloakEvents); err != nil {
			m.recorder().IncPollCycleError(m.tenantID, "events")
			m.logger.Error("❌ [DEBUG] Failed to save Keycloak events to database",
				logger.Str("tenant_id", m.tenantID),
				logger.Str("realm", realmName),
				logger.Int("count", len(keycloakEvents)),
				logger.Err(err))
			continue
		}

		m.recorder().AddEventsCollected(m.tenantID, realmName, len(keycloakEvents))

		m.logger.Info("✅ [DEBUG] Keycloak events saved successfully to keycloak_events table",
			logger.Str("tenant_id", m.tenantID),
			logger.Str("realm", realmName),
			logger.Int("count", len(keycloakEvents)))

		// Convert events to alerts
		if m.eventAlertConverter != nil {
			for _, kcEvent := range keycloakEvents {
				if err := m.eventAlertConverter.ConvertEvent(ctx, kcEvent); err != nil {
					m.logger.Error("Failed to convert event to alert",
						logger.Str("event_id", kcEvent.EventID),
						logger.Str("event_type", kcEvent.EventType),
						logger.Err(err))
					// Continue processing other events even if one fails
				}
			}
		}

		// Save general events to database for dashboard visibility
		m.logger.Info("💾 [DEBUG] Saving general events to events table",
			logger.Str("tenant_id", m.tenantID),
			logger.Str("realm", realmName),
			logger.Int("count", len(generalEvents)))

		savedCount := 0
		for i, generalEvent := range generalEvents {
			if err := m.generalEventRepo.MergeKeycloakEvent(ctx, generalEvent); err != nil {
				m.logger.Error("❌ [DEBUG] Failed to save general event to events table",
					logger.Str("tenant_id", m.tenantID),
					logger.Int("index", i),
					logger.Str("event_id", generalEvent.EventID),
					logger.Str("source", generalEvent.Source),
					logger.Err(err))
				// Continue with other events
			} else {
				savedCount++
			}
		}

		m.logger.Info("✅ [DEBUG] Event collection cycle completed",
			logger.Str("tenant_id", m.tenantID),
			logger.Str("realm", realmName),
			logger.Int("keycloak_events_saved", len(keycloakEvents)),
			logger.Int("general_events_saved", savedCount))
	}
}

// pollMetrics periodically fetches and stores Keycloak metrics
func (m *Monitor) pollMetrics(ctx context.Context) {
	ticker := time.NewTicker(m.instanceConfig.Polling.MetricsInterval)
	defer ticker.Stop()

	m.logger.Info("Starting metrics polling worker",
		logger.Str("interval", m.instanceConfig.Polling.MetricsInterval.String()))

	// Do first collection immediately
	m.collectMetrics(ctx)

	for {
		select {
		case <-ticker.C:
			m.collectMetrics(ctx)
		case <-m.workers.Stopping():
			m.logger.Info("Metrics polling worker stopped")
			return
		case <-ctx.Done():
			m.logger.Info("Metrics polling worker cancelled")
			return
		}
	}
}

// collectMetrics fetches and stores metrics from Keycloak
func (m *Monitor) collectMetrics(ctx context.Context) {
	start := time.Now()
	defer func() {
		m.recorder().ObservePollCycle(m.tenantID, "metrics", time.Since(start))
	}()

	m.logger.Debug("Collecting Keycloak metrics")

	realms := m.getRealmsToMonitor(ctx)
	if len(realms) == 0 {
		m.logger.Warn("No realms to monitor")
		return
	}

	now := time.Now()

	for _, realmName := range realms {
		// Get user counts. enabled/disabled come from COUNT-only queries
		// (/users/count?enabled=true|false); total is derived from their sum so
		// all three always reconcile and share one scope, while avoiding a
		// separate unfiltered /users/count round-trip.
		enabledUsers, disabledUsers, err := m.client.GetEnabledUsersCount(ctx, realmName)
		if err != nil {
			m.logger.Error("Failed to get users count",
				logger.Str("realm", realmName),
				logger.Err(err))
			continue
		}
		totalUsers := enabledUsers + disabledUsers

		// Get session counts
		activeSessions, offlineSessions, err := m.client.GetSessionsCount(ctx, realmName)
		if err != nil {
			m.logger.Warn("Failed to get sessions count",
				logger.Str("realm", realmName),
				logger.Err(err))
			// Continue with 0 values
		}

		// Get client count
		clients, err := m.client.GetClients(ctx, realmName)
		if err != nil {
			m.logger.Error("Failed to get clients",
				logger.Str("realm", realmName),
				logger.Err(err))
			continue
		}
		totalClients := len(clients)

		// Count recent events by type
		lookback := time.Now().Add(-m.instanceConfig.Polling.MetricsInterval)
		loginEvents, _ := m.eventRepo.CountEventsByType(ctx, m.tenantID, realmName, "LOGIN", lookback, now)
		logoutEvents, _ := m.eventRepo.CountEventsByType(ctx, m.tenantID, realmName, "LOGOUT", lookback, now)
		failedLoginEvents, _ := m.eventRepo.CountEventsByType(ctx, m.tenantID, realmName, "LOGIN_ERROR", lookback, now)
		registerEvents, _ := m.eventRepo.CountEventsByType(ctx, m.tenantID, realmName, "REGISTER", lookback, now)

		// Create metrics snapshot (using local Metrics type)
		metrics := &domain.KeycloakMetrics{
			TenantID:          m.tenantID,
			Time:              now,
			RealmName:         realmName,
			TotalUsers:        totalUsers,
			EnabledUsers:      enabledUsers,
			DisabledUsers:     disabledUsers,
			ActiveSessions:    activeSessions,
			OfflineSessions:   offlineSessions,
			TotalClients:      totalClients,
			LoginEvents:       int(loginEvents),
			LogoutEvents:      int(logoutEvents),
			FailedLoginEvents: int(failedLoginEvents),
			RegisterEvents:    int(registerEvents),
		}

		// Save metrics
		if err := m.metricsRepo.SaveMetrics(ctx, metrics); err != nil {
			m.logger.Error("Failed to save metrics",
				logger.Str("realm", realmName),
				logger.Err(err))
			continue
		}

		m.logger.Info("Collected metrics",
			logger.Str("realm", realmName),
			logger.Int("total_users", totalUsers),
			logger.Int("active_sessions", activeSessions),
			logger.Int("total_clients", totalClients))
	}
}

// pollHealth periodically checks Keycloak server health
func (m *Monitor) pollHealth(ctx context.Context) {
	ticker := time.NewTicker(m.instanceConfig.Polling.HealthInterval)
	defer ticker.Stop()

	m.logger.Info("Starting health polling worker",
		logger.Str("interval", m.instanceConfig.Polling.HealthInterval.String()))

	// Do first check immediately
	m.collectHealth(ctx)

	for {
		select {
		case <-ticker.C:
			m.collectHealth(ctx)
		case <-m.workers.Stopping():
			m.logger.Info("Health polling worker stopped")
			return
		case <-ctx.Done():
			m.logger.Info("Health polling worker cancelled")
			return
		}
	}
}

// collectHealth performs health check and stores result
func (m *Monitor) collectHealth(ctx context.Context) {
	start := time.Now()
	defer func() {
		m.recorder().ObservePollCycle(m.tenantID, "health", time.Since(start))
	}()

	m.logger.Debug("Performing Keycloak health check")

	healthMetrics := m.client.GetHealthMetrics(ctx)

	health := &domain.KeycloakHealth{
		TenantID:      m.tenantID,
		Time:          time.Now(),
		Status:        healthMetrics.Status,
		ResponseTime:  healthMetrics.ResponseTime,
		ServerVersion: healthMetrics.ServerVersion,
		UptimeMillis:  healthMetrics.UptimeMillis,
		MemoryUsed:    healthMetrics.MemoryUsed,
		MemoryMax:     healthMetrics.MemoryMax,
		MemoryFree:    healthMetrics.MemoryFree,
		ErrorMessage:  healthMetrics.Error,
	}

	if err := m.healthRepo.SaveHealth(ctx, health); err != nil {
		m.logger.Error("Failed to save health check", logger.Err(err))
		return
	}

	m.logger.Info("Health check completed",
		logger.Str("status", health.Status),
		logger.Int64("response_time_ms", health.ResponseTime),
		logger.Str("server_version", health.ServerVersion))

	// Process health status for alerting
	if m.healthAlertManager != nil {
		if err := m.healthAlertManager.ProcessHealthStatus(
			ctx,
			m.tenantID,
			m.tenantName,
			health.Status,
			health.ErrorMessage,
			int(health.ResponseTime),
		); err != nil {
			m.logger.Error("Failed to process health status for alerting",
				logger.Err(err),
				logger.Str("tenant_id", m.tenantID))
			// Don't return - health check was saved successfully
		}
	}

	// Keep the tenant record's health status current so the UI can distinguish
	// a broken connection from a genuinely empty Keycloak. Best-effort.
	if m.healthUpdater != nil {
		tenantStatus := mapHealthStatusToTenant(health.Status)
		message := "Keycloak reachable"
		if health.ErrorMessage != "" {
			message = health.ErrorMessage
		}
		if err := m.healthUpdater.UpdateHealth(ctx, m.tenantID, tenantStatus, message); err != nil {
			m.logger.Warn("Failed to update tenant health status",
				logger.Str("tenant_id", m.tenantID), logger.Err(err))
		}
		if health.Status == "DOWN" && health.ErrorMessage != "" {
			if err := m.healthUpdater.UpdateError(ctx, m.tenantID, health.ErrorMessage); err != nil {
				m.logger.Warn("Failed to update tenant last_error",
					logger.Str("tenant_id", m.tenantID), logger.Err(err))
			}
		} else if health.Status == "UP" {
			// Recovered: clear any stale error so the tenant is no longer flagged.
			if err := m.healthUpdater.UpdateError(ctx, m.tenantID, ""); err != nil {
				m.logger.Warn("Failed to clear tenant last_error",
					logger.Str("tenant_id", m.tenantID), logger.Err(err))
			}
		}
	}
}

// pollRealmInfo periodically updates realm information
func (m *Monitor) pollRealmInfo(ctx context.Context) {
	ticker := time.NewTicker(m.instanceConfig.Polling.RealmInfoInterval)
	defer ticker.Stop()

	m.logger.Info("Starting realm info polling worker",
		logger.Str("interval", m.instanceConfig.Polling.RealmInfoInterval.String()))

	// Do first collection immediately
	m.collectRealmInfo(ctx)

	for {
		select {
		case <-ticker.C:
			m.collectRealmInfo(ctx)
		case <-m.workers.Stopping():
			m.logger.Info("Realm info polling worker stopped")
			return
		case <-ctx.Done():
			m.logger.Info("Realm info polling worker cancelled")
			return
		}
	}
}

// collectRealmInfo fetches and stores realm information
func (m *Monitor) collectRealmInfo(ctx context.Context) {
	start := time.Now()
	defer func() {
		m.recorder().ObservePollCycle(m.tenantID, "realm_info", time.Since(start))
	}()

	m.logger.Debug("Collecting realm information")

	var realms []*keycloakadmin.RealmRepresentation
	var err error

	// Get realms to monitor
	if len(m.instanceConfig.Realms) > 0 {
		// Fetch specific realms
		for _, realmName := range m.instanceConfig.Realms {
			realm, err := m.client.GetRealmInfo(ctx, realmName)
			if err != nil {
				m.logger.Error("Failed to get realm info",
					logger.Str("realm", realmName),
					logger.Err(err))
				continue
			}
			realms = append(realms, realm)
		}
	} else {
		// Fetch all realms
		realms, err = m.client.GetAllRealms(ctx)
		if err != nil {
			m.logger.Error("Failed to get all realms", logger.Err(err))
			return
		}
	}

	now := time.Now()

	for _, realm := range realms {
		realmInfo := &domain.KeycloakRealmInfo{
			TenantID:                  m.tenantID,
			RealmID:                   realm.ID,
			RealmName:                 realm.Realm,
			DisplayName:               realm.DisplayName,
			Enabled:                   realm.Enabled,
			SslRequired:               realm.SslRequired,
			RegistrationAllowed:       realm.RegistrationAllowed,
			RememberMe:                realm.RememberMe,
			VerifyEmail:               realm.VerifyEmail,
			LoginWithEmailAllowed:     realm.LoginWithEmailAllowed,
			DuplicateEmailsAllowed:    realm.DuplicateEmailsAllowed,
			ResetPasswordAllowed:      realm.ResetPasswordAllowed,
			EditUsernameAllowed:       realm.EditUsernameAllowed,
			BruteForceProtected:       realm.BruteForceProtected,
			EventsEnabled:             realm.EventsEnabled,
			EventsListeners:           realm.EventsListeners,
			EnabledEventTypes:         realm.EnabledEventTypes,
			AdminEventsEnabled:        realm.AdminEventsEnabled,
			AdminEventsDetailsEnabled: realm.AdminEventsDetailsEnabled,
			LastChecked:               now,
			IsHealthy:                 realm.Enabled,
			HealthMessage:             "",
		}

		if !realm.Enabled {
			realmInfo.HealthMessage = "Realm is disabled"
		}

		// Check if events are properly configured
		if realm.Enabled && !realm.EventsEnabled {
			if realmInfo.HealthMessage != "" {
				realmInfo.HealthMessage += "; "
			}
			realmInfo.HealthMessage += "Events are not enabled"
		}
		if realm.Enabled && realm.EventsEnabled && len(realm.EventsListeners) == 0 {
			if realmInfo.HealthMessage != "" {
				realmInfo.HealthMessage += "; "
			}
			realmInfo.HealthMessage += "No event listeners configured"
		}

		if err := m.realmRepo.SaveRealm(ctx, realmInfo); err != nil {
			m.logger.Error("Failed to save realm info",
				logger.Str("tenant_id", m.tenantID),
				logger.Str("realm", realm.Realm),
				logger.Err(err))
			continue
		}

		m.logger.Info("Updated realm info",
			logger.Str("tenant_id", m.tenantID),
			logger.Str("realm", realm.Realm),
			logger.Bool("enabled", realm.Enabled))
	}
}

// getRealmsToMonitor returns the list of realms to monitor
func (m *Monitor) getRealmsToMonitor(ctx context.Context) []string {
	if len(m.instanceConfig.Realms) > 0 {
		return m.instanceConfig.Realms
	}

	// If no realms configured, try to fetch all realms from database
	realms, err := m.realmRepo.GetRealms(ctx, m.tenantID)
	if err != nil {
		m.logger.Warn("Failed to get realms from database", logger.Err(err))
		return []string{}
	}

	realmNames := make([]string, 0, len(realms))
	for _, realm := range realms {
		if realm.Enabled {
			realmNames = append(realmNames, realm.RealmName)
		}
	}

	return realmNames
}
