package tenant

import (
	"context"
	"encoding/json"
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

	// Start monitor for each enabled tenant
	for _, tenant := range tenants {
		if err := m.startMonitorForTenant(ctx, tenant); err != nil {
			m.logger.Error("Failed to start monitor for tenant",
				"tenant_id", tenant.TenantID,
				"error", err)
			// Continue with other tenants even if one fails
			continue
		}
	}

	// Register callback for tenant changes
	m.service.RegisterCallback(m.handleTenantChange)

	m.logger.Info("Monitor Pool Manager started successfully",
		"active_monitors", len(tenants))
	return nil
}

// Stop gracefully stops all monitors
func (m *MonitorPoolManager) Stop() error {
	m.logger.Info("Stopping Monitor Pool Manager")

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
			if err := m.startMonitorForTenant(ctx, tenant); err != nil {
				m.logger.Error("Failed to start monitor for new tenant",
					"tenant_id", tenant.TenantID,
					"error", err)
			}
		}

	case EventEnabled:
		m.logger.Info("Tenant enabled, starting monitor",
			"tenant_id", tenant.TenantID)
		if err := m.startMonitorForTenant(ctx, tenant); err != nil {
			m.logger.Error("Failed to start monitor for enabled tenant",
				"tenant_id", tenant.TenantID,
				"error", err)
		}

	case EventDisabled:
		m.logger.Info("Tenant disabled, stopping monitor",
			"tenant_id", tenant.TenantID)
		if err := m.stopMonitorForTenant(tenant.TenantID); err != nil {
			m.logger.Error("Failed to stop monitor for disabled tenant",
				"tenant_id", tenant.TenantID,
				"error", err)
		}

	case EventUpdated:
		m.logger.Info("Tenant updated, restarting monitor",
			"tenant_id", tenant.TenantID)
		if tenant.Enabled {
			if err := m.restartMonitorForTenant(ctx, tenant); err != nil {
				m.logger.Error("Failed to restart monitor for updated tenant",
					"tenant_id", tenant.TenantID,
					"error", err)
			}
		}

	case EventRemoved:
		m.logger.Info("Tenant removed, stopping monitor",
			"tenant_id", tenant.TenantID)
		if err := m.stopMonitorForTenant(tenant.TenantID); err != nil {
			m.logger.Error("Failed to stop monitor for removed tenant",
				"tenant_id", tenant.TenantID,
				"error", err)
		}
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

// createTenantConfig creates an instance config for a specific tenant
// Merges global defaults with tenant-specific configuration overrides
func (m *MonitorPoolManager) createTenantConfig(tenant *domain.KeycloakTenant) any {
	// If global config is nil, return nil
	if m.globalConfig == nil {
		return nil
	}

	// Try to deep copy and merge configuration
	// The actual implementation depends on the config type
	// For now, we'll use JSON marshal/unmarshal for a generic deep copy
	globalBytes, err := json.Marshal(m.globalConfig)
	if err != nil {
		m.logger.Warn("Failed to marshal global config, using as-is",
			"tenant_id", tenant.TenantID,
			"error", err)
		return m.globalConfig
	}

	// If tenant has no configuration overrides, return global config
	if tenant.Configuration == "" {
		return m.globalConfig
	}

	// Create a map to hold merged config
	var configMap map[string]any
	if err := json.Unmarshal(globalBytes, &configMap); err != nil {
		m.logger.Warn("Failed to unmarshal global config to map",
			"tenant_id", tenant.TenantID,
			"error", err)
		return m.globalConfig
	}

	// Parse tenant overrides
	var tenantOverrides map[string]any
	if err := json.Unmarshal([]byte(tenant.Configuration), &tenantOverrides); err != nil {
		m.logger.Warn("Failed to parse tenant configuration JSON, using global config",
			"tenant_id", tenant.TenantID,
			"error", err)
		return m.globalConfig
	}

	// Merge tenant overrides into config map
	mergeConfig(configMap, tenantOverrides)

	m.logger.Debug("Applied tenant-specific configuration overrides",
		"tenant_id", tenant.TenantID)

	return configMap
}

// mergeConfig recursively merges src into dst
func mergeConfig(dst, src map[string]any) {
	for key, srcVal := range src {
		if dstVal, exists := dst[key]; exists {
			// If both are maps, merge recursively
			srcMap, srcIsMap := srcVal.(map[string]any)
			dstMap, dstIsMap := dstVal.(map[string]any)
			if srcIsMap && dstIsMap {
				mergeConfig(dstMap, srcMap)
				continue
			}
		}
		// Otherwise, override with src value
		dst[key] = srcVal
	}
}
