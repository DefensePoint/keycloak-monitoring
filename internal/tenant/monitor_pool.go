package tenant

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

// createClientTimeout bounds how long a single tenant's Keycloak client
// creation (including base-URL auto-detection) may take during startup.
// Fx's OnStart deadline (15s by default) is cumulative across every module
// in the app, not per-tenant, so this must stay well under that regardless
// of how many enabled tenants exist. A tenant that exceeds it is marked
// unhealthy and skipped rather than blocking the rest of the app from
// starting.
const createClientTimeout = 5 * time.Second

// MetricsRecorder is a local interface for recording monitor metrics.
// This decouples tenant from the concrete metrics implementation.
type MetricsRecorder interface {
	SetTenantMonitorUp(tenantID, tenantName string, up bool)
	SetTenantMonitorsActive(n int)
}

// Logger is a local interface for logging.
// This decouples tenant from the concrete logger implementation.
type Logger interface {
	Info(msg string, fields ...any)
	Error(msg string, fields ...any)
	Warn(msg string, fields ...any)
	Debug(msg string, fields ...any)
}

// monitorInstance represents a running monitor for a specific tenant
type monitorInstance struct {
	tenant  *domain.KeycloakTenant
	client  KeycloakClient
	monitor KeycloakMonitor
	cancel  context.CancelFunc
}

// MonitorPoolManager manages a pool of Keycloak monitors, one per tenant
type MonitorPoolManager struct {
	service             Service
	monitorFactory      KeycloakMonitorFactory
	globalConfig        any // Global default settings for all tenants (passed to factory)
	logger              Logger
	monitors            sync.Map // map[string]*monitorInstance (tenant_id -> instance)
	eventRepo           EventRepository
	generalEventRepo    GeneralEventRepository
	metricsRepo         MetricsRepository
	healthRepo          HealthRepository
	realmRepo           RealmRepository
	eventAlertConverter EventAlertConverter
	healthAlertManager  HealthAlertManager
	metrics             MetricsRecorder
	mu                  sync.RWMutex

	// Failed starts awaiting a retry (monitor_pool_retry.go).
	retryMu      sync.Mutex
	pending      map[string]*pendingStart
	retryWake    chan struct{}
	retryCancel  context.CancelFunc
	retryDone    chan struct{}
	retryInitial time.Duration
	retryMax     time.Duration
}

// SetMetrics attaches a metrics recorder for monitor up/down gauges.
func (m *MonitorPoolManager) SetMetrics(r MetricsRecorder) {
	m.metrics = r
}

// syncMonitorGauges refreshes the per-tenant gauge and the active-monitor total.
func (m *MonitorPoolManager) syncMonitorGauges(tenantID, tenantName string, up bool) {
	if m.metrics == nil {
		return
	}
	m.metrics.SetTenantMonitorUp(tenantID, tenantName, up)
	m.metrics.SetTenantMonitorsActive(m.GetActiveMonitorCount())
}

// NewMonitorPoolManager creates a new monitor pool manager
func NewMonitorPoolManager(
	service Service,
	monitorFactory KeycloakMonitorFactory,
	globalCfg any,
	log Logger,
	eventRepo EventRepository,
	generalEventRepo GeneralEventRepository,
	metricsRepo MetricsRepository,
	healthRepo HealthRepository,
	realmRepo RealmRepository,
	eventAlertConverter EventAlertConverter,
	healthAlertManager HealthAlertManager,
) *MonitorPoolManager {
	return &MonitorPoolManager{
		service:             service,
		monitorFactory:      monitorFactory,
		globalConfig:        globalCfg,
		logger:              log,
		eventRepo:           eventRepo,
		generalEventRepo:    generalEventRepo,
		metricsRepo:         metricsRepo,
		healthRepo:          healthRepo,
		realmRepo:           realmRepo,
		eventAlertConverter: eventAlertConverter,
		healthAlertManager:  healthAlertManager,
		pending:             map[string]*pendingStart{},
		retryWake:           make(chan struct{}, 1),
		retryInitial:        defaultRetryInitial,
		retryMax:            defaultRetryMax,
	}
}

// Start initializes the monitor pool and starts monitoring all enabled tenants
func (m *MonitorPoolManager) Start(ctx context.Context) error {
	m.logger.Info("Starting Monitor Pool Manager")

	// Get all enabled tenants
	tenants, err := m.service.ListEnabledTenants(ctx)
	if err != nil {
		return fmt.Errorf("failed to list enabled tenants: %w", err)
	}

	// Start monitor for each enabled tenant, counting the ones that came up.
	// len(tenants) is what was attempted, not what is running, so reporting it
	// as active_monitors told an operator every monitor was live even when none
	// were: the per-tenant errors above were the only sign, and this is the line
	// people read to confirm startup.
	// A tenant that fails here is retried in the background rather than left
	// unmonitored until the next restart.
	m.startRetryLoop()
	started := 0
	for _, tenant := range tenants {
		if err := m.startMonitorForTenant(ctx, tenant); err != nil {
			m.logger.Error("Failed to start monitor for tenant",
				"tenant_id", tenant.TenantID,
				"error", err)
			m.startFailed(ctx, tenant, err)
			continue
		}
		started++
	}

	// Register callback for tenant changes
	m.service.RegisterCallback(m.handleTenantChange)

	switch {
	case started == len(tenants):
		m.logger.Info("Monitor Pool Manager started",
			"active_monitors", started)
	case started == 0 && len(tenants) > 0:
		m.logger.Error("Monitor Pool Manager started with no working monitors",
			"active_monitors", started,
			"requested_monitors", len(tenants))
	default:
		m.logger.Warn("Monitor Pool Manager started with some monitors unavailable",
			"active_monitors", started,
			"requested_monitors", len(tenants))
	}
	return nil
}

// Stop gracefully stops all monitors
func (m *MonitorPoolManager) Stop() error {
	m.logger.Info("Stopping Monitor Pool Manager")

	m.stopRetryLoop()

	var wg sync.WaitGroup
	m.monitors.Range(func(key, value interface{}) bool {
		tenantID := key.(string)
		instance := value.(*monitorInstance)

		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := m.stopMonitor(tenantID, instance); err != nil {
				m.logger.Error("Failed to stop monitor",
					"tenant_id", tenantID,
					"error", err)
			}
		}()
		return true
	})

	// Wait for all monitors to stop
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		m.logger.Info("All monitors stopped successfully")
	case <-time.After(30 * time.Second):
		m.logger.Warn("Timeout waiting for monitors to stop")
	}

	return nil
}

// handleTenantChange handles tenant change events
func (m *MonitorPoolManager) handleTenantChange(event Event, tenant *domain.KeycloakTenant) {
	ctx := context.Background()

	switch event {
	case EventAdded:
		m.logger.Info("Tenant added, starting monitor",
			"tenant_id", tenant.TenantID)
		if tenant.Enabled {
			m.cancelRetry(tenant.TenantID)
			if err := m.startMonitorForTenant(ctx, tenant); err != nil {
				m.logger.Error("Failed to start monitor for new tenant",
					"tenant_id", tenant.TenantID,
					"error", err)
				m.startFailed(ctx, tenant, err)
			}
		}

	case EventEnabled:
		m.logger.Info("Tenant enabled, starting monitor",
			"tenant_id", tenant.TenantID)
		m.cancelRetry(tenant.TenantID)
		if err := m.startMonitorForTenant(ctx, tenant); err != nil {
			m.logger.Error("Failed to start monitor for enabled tenant",
				"tenant_id", tenant.TenantID,
				"error", err)
			m.startFailed(ctx, tenant, err)
		}

	case EventDisabled:
		m.logger.Info("Tenant disabled, stopping monitor",
			"tenant_id", tenant.TenantID)
		m.cancelRetry(tenant.TenantID)
		if err := m.stopMonitorForTenant(tenant.TenantID); err != nil {
			m.logger.Error("Failed to stop monitor for disabled tenant",
				"tenant_id", tenant.TenantID,
				"error", err)
		}
		m.closeHealthAlerts(ctx, tenant, "disabled")

	case EventUpdated:
		m.logger.Info("Tenant updated, restarting monitor",
			"tenant_id", tenant.TenantID)
		if tenant.Enabled {
			// A changed tenant (a corrected secret, say) gets an immediate
			// attempt and a fresh backoff.
			m.cancelRetry(tenant.TenantID)
			if err := m.restartMonitorForTenant(ctx, tenant); err != nil {
				m.logger.Error("Failed to restart monitor for updated tenant",
					"tenant_id", tenant.TenantID,
					"error", err)
				m.startFailed(ctx, tenant, err)
			}
		}

	case EventRemoved:
		m.logger.Info("Tenant removed, stopping monitor",
			"tenant_id", tenant.TenantID)
		m.cancelRetry(tenant.TenantID)
		if err := m.stopMonitorForTenant(tenant.TenantID); err != nil {
			m.logger.Error("Failed to stop monitor for removed tenant",
				"tenant_id", tenant.TenantID,
				"error", err)
		}
		m.closeHealthAlerts(ctx, tenant, "removed")
	}
}

// startMonitorForTenant starts a monitor for a specific tenant
func (m *MonitorPoolManager) startMonitorForTenant(ctx context.Context, tenant *domain.KeycloakTenant) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if monitor already exists
	if _, exists := m.monitors.Load(tenant.TenantID); exists {
		return fmt.Errorf("monitor for tenant %s already exists", tenant.TenantID)
	}

	m.logger.Info("Starting monitor for tenant",
		"tenant_id", tenant.TenantID,
		"name", tenant.Name,
		"server_url", tenant.ServerURL)

	// Create instance config for this tenant (merging global + tenant-specific settings)
	instanceCfg := m.createTenantConfig(tenant)

	// Create monitor config
	monitorCfg := &MonitorConfig{
		TenantID:      tenant.TenantID,
		TenantName:    tenant.Name,
		ServerURL:     tenant.ServerURL,
		AdminRealm:    tenant.AdminRealm,
		ClientID:      tenant.ClientID,
		ClientSecret:  tenant.ClientSecret,
		Configuration: tenant.Configuration,
	}

	// Create Keycloak client via factory. Bounded to a fraction of the Fx
	// start budget so one unresponsive tenant can't consume the entire
	// startup window and abort the process; the tenant is marked unhealthy
	// and skipped instead of blocking boot for everyone after it.
	createCtx, cancelCreate := context.WithTimeout(ctx, createClientTimeout)
	client, err := m.monitorFactory.CreateClient(createCtx, monitorCfg, instanceCfg)
	cancelCreate()
	if err != nil {
		// Update tenant health status
		_ = m.service.UpdateHealth(ctx, tenant.TenantID, "unhealthy",
			fmt.Sprintf("Failed to create Keycloak client: %v", err))
		_ = m.service.UpdateError(ctx, tenant.TenantID, err.Error())
		return fmt.Errorf("failed to create Keycloak client: %w", err)
	}

	// Create tenant-aware monitor via factory
	monitor := m.monitorFactory.CreateMonitor(
		tenant.TenantID,
		tenant.Name,
		client,
		instanceCfg,
		m.eventRepo,
		m.generalEventRepo,
		m.metricsRepo,
		m.healthRepo,
		m.realmRepo,
		m.eventAlertConverter,
		m.healthAlertManager,
	)

	// Create context for this monitor
	// Use context.Background() instead of ctx because ctx comes from Fx OnStart
	// which has a short timeout. The monitor should run independently and be
	// cancelled via instance.cancel() when Stop() is called.
	monitorCtx, cancel := context.WithCancel(context.Background())

	// Start the monitor
	if err := monitor.Start(monitorCtx); err != nil {
		cancel()
		_ = client.Close()
		_ = m.service.UpdateHealth(ctx, tenant.TenantID, "unhealthy",
			fmt.Sprintf("Failed to start monitor: %v", err))
		_ = m.service.UpdateError(ctx, tenant.TenantID, err.Error())
		return fmt.Errorf("failed to start monitor: %w", err)
	}

	// Store monitor instance
	instance := &monitorInstance{
		tenant:  tenant,
		client:  client,
		monitor: monitor,
		cancel:  cancel,
	}
	m.monitors.Store(tenant.TenantID, instance)
	m.syncMonitorGauges(tenant.TenantID, tenant.Name, true)

	// Update tenant health status
	_ = m.service.UpdateHealth(ctx, tenant.TenantID, "healthy", "Monitor started successfully")

	m.logger.Info("Monitor started successfully for tenant",
		"tenant_id", tenant.TenantID)
	return nil
}

// stopMonitorForTenant stops a monitor for a specific tenant
func (m *MonitorPoolManager) stopMonitorForTenant(tenantID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	value, exists := m.monitors.Load(tenantID)
	if !exists {
		return fmt.Errorf("monitor for tenant %s not found", tenantID)
	}

	instance := value.(*monitorInstance)
	return m.stopMonitor(tenantID, instance)
}

// stopMonitor stops a monitor instance
func (m *MonitorPoolManager) stopMonitor(tenantID string, instance *monitorInstance) error {
	m.logger.Info("Stopping monitor for tenant", "tenant_id", tenantID)

	// Cancel monitor context
	if instance.cancel != nil {
		instance.cancel()
	}

	// Stop monitor
	if instance.monitor != nil {
		if err := instance.monitor.Stop(); err != nil {
			m.logger.Warn("Error stopping monitor",
				"tenant_id", tenantID,
				"error", err)
		}
	}

	// Close client
	if instance.client != nil {
		if err := instance.client.Close(); err != nil {
			m.logger.Warn("Error closing Keycloak client",
				"tenant_id", tenantID,
				"error", err)
		}
	}

	// Remove from map
	m.monitors.Delete(tenantID)
	m.syncMonitorGauges(tenantID, instance.tenant.Name, false)

	m.logger.Info("Monitor stopped for tenant", "tenant_id", tenantID)
	return nil
}

// restartMonitorForTenant restarts a monitor for a specific tenant
func (m *MonitorPoolManager) restartMonitorForTenant(ctx context.Context, tenant *domain.KeycloakTenant) error {
	m.logger.Info("Restarting monitor for tenant", "tenant_id", tenant.TenantID)

	// Stop existing monitor
	if err := m.stopMonitorForTenant(tenant.TenantID); err != nil {
		m.logger.Warn("Failed to stop monitor during restart",
			"tenant_id", tenant.TenantID,
			"error", err)
		// Continue anyway
	}

	// Start new monitor
	return m.startMonitorForTenant(ctx, tenant)
}

// GetMonitorStatus returns the status of all monitors
func (m *MonitorPoolManager) GetMonitorStatus() map[string]string {
	status := make(map[string]string)
	m.monitors.Range(func(key, value interface{}) bool {
		tenantID := key.(string)
		instance := value.(*monitorInstance)
		if instance.client.IsHealthy(context.Background()) {
			status[tenantID] = "healthy"
		} else {
			status[tenantID] = "unhealthy"
		}
		return true
	})
	return status
}

// GetActiveMonitorCount returns the number of active monitors
func (m *MonitorPoolManager) GetActiveMonitorCount() int {
	count := 0
	m.monitors.Range(func(key, value interface{}) bool {
		count++
		return true
	})
	return count
}

// GetKeycloakClient returns the Keycloak client for a specific tenant
// Returns nil if the tenant doesn't have an active monitor
func (m *MonitorPoolManager) GetKeycloakClient(tenantID string) KeycloakClient {
	value, exists := m.monitors.Load(tenantID)
	if !exists {
		return nil
	}
	instance := value.(*monitorInstance)
	return instance.client
}

// createTenantConfig returns the instance config for a tenant: the global
// config with the tenant's configuration overrides applied. The factory does
// the merge because it owns the config type. An override it cannot use is
// reported and the tenant keeps the global config, never the factory's
// built-in defaults.
func (m *MonitorPoolManager) createTenantConfig(tenant *domain.KeycloakTenant) any {
	if m.globalConfig == nil {
		return nil
	}
	if tenant.Configuration == "" {
		return m.globalConfig
	}

	merged, err := m.monitorFactory.TenantInstanceConfig(tenant.TenantID, m.globalConfig, tenant.Configuration)
	if err != nil {
		m.logger.Warn("Tenant configuration overrides not applied, using the global configuration",
			"tenant_id", tenant.TenantID,
			"error", err)
		return m.globalConfig
	}

	m.logger.Debug("Applied tenant-specific configuration overrides",
		"tenant_id", tenant.TenantID)
	return merged
}
