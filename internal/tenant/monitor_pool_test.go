package tenant

import (
	"context"
	"errors"
	"testing"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

// mockLogger is a simple mock logger for testing
type mockLogger struct{}

func (m *mockLogger) Info(msg string, fields ...any)  {}
func (m *mockLogger) Error(msg string, fields ...any) {}
func (m *mockLogger) Warn(msg string, fields ...any)  {}
func (m *mockLogger) Debug(msg string, fields ...any) {}

// mockService is a mock implementation of the Service interface for testing
type mockService struct {
	tenants   []*domain.KeycloakTenant
	callbacks []ChangeCallback
}

func (m *mockService) GetTenant(ctx context.Context, tenantID string) (*domain.KeycloakTenant, error) {
	for _, t := range m.tenants {
		if t.TenantID == tenantID {
			return t, nil
		}
	}
	return nil, nil
}

func (m *mockService) ListTenants(ctx context.Context) ([]*domain.KeycloakTenant, error) {
	return m.tenants, nil
}

func (m *mockService) ListEnabledTenants(ctx context.Context) ([]*domain.KeycloakTenant, error) {
	var enabled []*domain.KeycloakTenant
	for _, t := range m.tenants {
		if t.Enabled {
			enabled = append(enabled, t)
		}
	}
	return enabled, nil
}

func (m *mockService) CreateTenant(ctx context.Context, req *CreateRequest) (*domain.KeycloakTenant, error) {
	return nil, nil
}

func (m *mockService) UpdateTenant(ctx context.Context, tenantID string, req *UpdateRequest) (*domain.KeycloakTenant, error) {
	return nil, nil
}

func (m *mockService) DeleteTenant(ctx context.Context, tenantID string) error {
	return nil
}

func (m *mockService) SyncFromConfig(ctx context.Context, tenantID string, createReq *CreateRequest, updateReq *UpdateRequest) (*domain.KeycloakTenant, error) {
	return nil, nil
}

func (m *mockService) ListConfigDefinedTenants(ctx context.Context) ([]*domain.KeycloakTenant, error) {
	return nil, nil
}

func (m *mockService) UnmarkConfigDefined(ctx context.Context, tenantID string) error {
	return nil
}

func (m *mockService) UpdateHealth(ctx context.Context, tenantID, status, message string) error {
	return nil
}

func (m *mockService) LoadTenants(ctx context.Context) error {
	return nil
}

func (m *mockService) UpdateError(ctx context.Context, tenantID, errorMsg string) error {
	return nil
}

func (m *mockService) GetHealth(ctx context.Context, tenantID string) (*HealthStatus, error) {
	return nil, nil
}

func (m *mockService) RegisterCallback(callback ChangeCallback) {
	m.callbacks = append(m.callbacks, callback)
}

func (m *mockService) GetDefault(ctx context.Context) (*domain.KeycloakTenant, error) {
	return nil, nil
}

// mockMonitorFactory is a mock implementation of KeycloakMonitorFactory
type mockMonitorFactory struct {
	createClientCalled  bool
	createMonitorCalled bool
	// createClientErrFor fails CreateClient for the named tenants, so a test can
	// start a pool where some or all of the monitors never come up.
	createClientErrFor map[string]error

	// resolved and resolveErr are what TenantInstanceConfig answers; the
	// fields after them record what the pool passed in and on.
	resolved        any
	resolveErr      error
	resolveCalls    int
	resolvedFrom    string
	clientInstance  any
	monitorInstance any
}

func (m *mockMonitorFactory) TenantInstanceConfig(tenantID string, global any, configuration string) (any, error) {
	m.resolveCalls++
	m.resolvedFrom = configuration
	return m.resolved, m.resolveErr
}

func (m *mockMonitorFactory) CreateClient(ctx context.Context, cfg *MonitorConfig, instanceCfg any) (KeycloakClient, error) {
	m.createClientCalled = true
	m.clientInstance = instanceCfg
	if err := m.createClientErrFor[cfg.TenantID]; err != nil {
		return nil, err
	}
	return &mockKeycloakClient{}, nil
}

func (m *mockMonitorFactory) CreateMonitor(
	tenantID string,
	tenantName string,
	client KeycloakClient,
	instanceCfg any,
	eventRepo EventRepository,
	generalEventRepo GeneralEventRepository,
	metricsRepo MetricsRepository,
	healthRepo HealthRepository,
	realmRepo RealmRepository,
	eventAlertConverter EventAlertConverter,
	healthAlertManager HealthAlertManager,
) KeycloakMonitor {
	m.createMonitorCalled = true
	m.monitorInstance = instanceCfg
	return &mockKeycloakMonitor{}
}

// mockKeycloakClient is a mock Keycloak client
type mockKeycloakClient struct {
	healthy bool
}

func (m *mockKeycloakClient) IsHealthy(ctx context.Context) bool {
	return m.healthy
}

func (m *mockKeycloakClient) Close() error {
	return nil
}

// mockKeycloakMonitor is a mock Keycloak monitor
type mockKeycloakMonitor struct {
	started bool
	stopped bool
}

func (m *mockKeycloakMonitor) Start(ctx context.Context) error {
	m.started = true
	return nil
}

func (m *mockKeycloakMonitor) Stop() error {
	m.stopped = true
	return nil
}

func TestMonitorPoolManager_GetActiveMonitorCount(t *testing.T) {
	log := &mockLogger{}
	service := &mockService{}
	factory := &mockMonitorFactory{}

	manager := NewMonitorPoolManager(
		service,
		factory,
		nil, // globalConfig
		log,
		nil, // eventRepo
		nil, // generalEventRepo
		nil, // metricsRepo
		nil, // healthRepo
		nil, // realmRepo
		nil, // eventAlertConverter
		nil, // healthAlertManager
	)

	// Initially should have 0 monitors
	count := manager.GetActiveMonitorCount()
	if count != 0 {
		t.Errorf("Expected 0 active monitors initially, got %d", count)
	}
}

func TestMonitorPoolManager_CreateTenantConfig_NilGlobal(t *testing.T) {
	log := &mockLogger{}
	service := &mockService{}
	factory := &mockMonitorFactory{}

	manager := NewMonitorPoolManager(
		service,
		factory,
		nil, // globalConfig is nil
		log,
		nil, nil, nil, nil, nil, nil, nil,
	)

	tenant := &domain.KeycloakTenant{
		TenantID:   "test-tenant",
		Name:       "Test Tenant",
		ServerURL:  "https://keycloak.example.com",
		AdminRealm: "master",
		ClientID:   "admin-cli",
	}

	// Should return nil when globalConfig is nil
	instanceConfig := manager.createTenantConfig(tenant)
	if instanceConfig != nil {
		t.Error("Expected nil instance config when global config is nil")
	}
}

func TestMonitorPoolManager_CreateTenantConfig_NoOverrides(t *testing.T) {
	log := &mockLogger{}
	service := &mockService{}
	factory := &mockMonitorFactory{}

	globalConfig := map[string]any{
		"realms": []string{"master", "app"},
		"polling": map[string]any{
			"metrics_interval": "5m",
		},
	}

	manager := NewMonitorPoolManager(
		service,
		factory,
		globalConfig,
		log,
		nil, nil, nil, nil, nil, nil, nil,
	)

	// Tenant without configuration overrides
	tenant := &domain.KeycloakTenant{
		TenantID:      "test-tenant",
		Name:          "Test Tenant",
		ServerURL:     "https://keycloak.example.com",
		AdminRealm:    "master",
		Configuration: "", // No overrides
	}

	instanceConfig := manager.createTenantConfig(tenant)

	// When no overrides, should return global config as-is
	if instanceConfig == nil {
		t.Error("Expected non-nil instance config")
	}
}

func TestMonitorPoolManager_StartMonitorForTenant(t *testing.T) {
	log := &mockLogger{}
	service := &mockService{}
	factory := &mockMonitorFactory{}

	manager := NewMonitorPoolManager(
		service,
		factory,
		nil,
		log,
		nil, nil, nil, nil, nil, nil, nil,
	)

	tenant := &domain.KeycloakTenant{
		TenantID:   "test-tenant",
		Name:       "Test Tenant",
		ServerURL:  "https://keycloak.example.com",
		AdminRealm: "master",
		ClientID:   "admin-cli",
		Enabled:    true,
	}

	ctx := context.Background()
	err := manager.startMonitorForTenant(ctx, tenant)
	if err != nil {
		t.Fatalf("Failed to start monitor: %v", err)
	}

	// Verify factory methods were called
	if !factory.createClientCalled {
		t.Error("Expected CreateClient to be called")
	}
	if !factory.createMonitorCalled {
		t.Error("Expected CreateMonitor to be called")
	}

	// Verify monitor count
	count := manager.GetActiveMonitorCount()
	if count != 1 {
		t.Errorf("Expected 1 active monitor, got %d", count)
	}
}

func TestMonitorPoolManager_StopMonitorForTenant(t *testing.T) {
	log := &mockLogger{}
	service := &mockService{}
	factory := &mockMonitorFactory{}

	manager := NewMonitorPoolManager(
		service,
		factory,
		nil,
		log,
		nil, nil, nil, nil, nil, nil, nil,
	)

	tenant := &domain.KeycloakTenant{
		TenantID:   "test-tenant",
		Name:       "Test Tenant",
		ServerURL:  "https://keycloak.example.com",
		AdminRealm: "master",
		ClientID:   "admin-cli",
		Enabled:    true,
	}

	ctx := context.Background()

	// Start a monitor first
	err := manager.startMonitorForTenant(ctx, tenant)
	if err != nil {
		t.Fatalf("Failed to start monitor: %v", err)
	}

	// Verify it's running
	if manager.GetActiveMonitorCount() != 1 {
		t.Error("Expected 1 active monitor after start")
	}

	// Stop the monitor
	err = manager.stopMonitorForTenant(tenant.TenantID)
	if err != nil {
		t.Fatalf("Failed to stop monitor: %v", err)
	}

	// Verify it's stopped
	if manager.GetActiveMonitorCount() != 0 {
		t.Error("Expected 0 active monitors after stop")
	}
}

func TestMonitorPoolManager_HandleTenantChange(t *testing.T) {
	log := &mockLogger{}
	service := &mockService{}
	factory := &mockMonitorFactory{}

	manager := NewMonitorPoolManager(
		service,
		factory,
		nil,
		log,
		nil, nil, nil, nil, nil, nil, nil,
	)

	tenant := &domain.KeycloakTenant{
		TenantID:   "test-tenant",
		Name:       "Test Tenant",
		ServerURL:  "https://keycloak.example.com",
		AdminRealm: "master",
		Enabled:    true,
	}

	// Test EventAdded with enabled tenant
	manager.handleTenantChange(EventAdded, tenant)
	if manager.GetActiveMonitorCount() != 1 {
		t.Error("Expected monitor to be started for added enabled tenant")
	}

	// Test EventDisabled
	manager.handleTenantChange(EventDisabled, tenant)
	if manager.GetActiveMonitorCount() != 0 {
		t.Error("Expected monitor to be stopped for disabled tenant")
	}

	// Test EventEnabled
	manager.handleTenantChange(EventEnabled, tenant)
	if manager.GetActiveMonitorCount() != 1 {
		t.Error("Expected monitor to be started for enabled tenant")
	}

	// Test EventRemoved
	manager.handleTenantChange(EventRemoved, tenant)
	if manager.GetActiveMonitorCount() != 0 {
		t.Error("Expected monitor to be stopped for removed tenant")
	}
}

func TestMonitorPoolManager_GetMonitorStatus(t *testing.T) {
	log := &mockLogger{}
	service := &mockService{}
	factory := &mockMonitorFactory{}

	manager := NewMonitorPoolManager(
		service,
		factory,
		nil,
		log,
		nil, nil, nil, nil, nil, nil, nil,
	)

	// Initially empty
	status := manager.GetMonitorStatus()
	if len(status) != 0 {
		t.Error("Expected empty status map initially")
	}

	// Start a monitor
	tenant := &domain.KeycloakTenant{
		TenantID:   "test-tenant",
		Name:       "Test Tenant",
		ServerURL:  "https://keycloak.example.com",
		AdminRealm: "master",
		Enabled:    true,
	}

	ctx := context.Background()
	_ = manager.startMonitorForTenant(ctx, tenant)

	// Should have one entry
	status = manager.GetMonitorStatus()
	if len(status) != 1 {
		t.Errorf("Expected 1 status entry, got %d", len(status))
	}

	// Check status (mock client returns false for IsHealthy by default)
	if status["test-tenant"] != "unhealthy" {
		t.Errorf("Expected 'unhealthy' status, got %s", status["test-tenant"])
	}
}

func TestMonitorPoolManager_GetKeycloakClient(t *testing.T) {
	log := &mockLogger{}
	service := &mockService{}
	factory := &mockMonitorFactory{}

	manager := NewMonitorPoolManager(
		service,
		factory,
		nil,
		log,
		nil, nil, nil, nil, nil, nil, nil,
	)

	// Non-existent tenant should return nil
	client := manager.GetKeycloakClient("non-existent")
	if client != nil {
		t.Error("Expected nil client for non-existent tenant")
	}

	// Start a monitor
	tenant := &domain.KeycloakTenant{
		TenantID:   "test-tenant",
		Name:       "Test Tenant",
		ServerURL:  "https://keycloak.example.com",
		AdminRealm: "master",
		Enabled:    true,
	}

	ctx := context.Background()
	_ = manager.startMonitorForTenant(ctx, tenant)

	// Should return client
	client = manager.GetKeycloakClient("test-tenant")
	if client == nil {
		t.Error("Expected non-nil client for existing tenant")
	}
}

// recordingLogger captures what was logged, so a test can assert the line an
// operator actually reads rather than only that Start returned nil.
type recordingLogger struct {
	entries []logEntry
}

type logEntry struct {
	level  string
	msg    string
	fields []any
}

// field reads a key from the alternating key/value pairs the Logger interface
// takes.
func (e logEntry) field(name string) (any, bool) {
	for i := 0; i+1 < len(e.fields); i += 2 {
		if k, ok := e.fields[i].(string); ok && k == name {
			return e.fields[i+1], true
		}
	}
	return nil, false
}

func (l *recordingLogger) record(level, msg string, fields []any) {
	l.entries = append(l.entries, logEntry{level: level, msg: msg, fields: fields})
}

func (l *recordingLogger) Info(msg string, fields ...any)  { l.record("info", msg, fields) }
func (l *recordingLogger) Error(msg string, fields ...any) { l.record("error", msg, fields) }
func (l *recordingLogger) Warn(msg string, fields ...any)  { l.record("warn", msg, fields) }
func (l *recordingLogger) Debug(msg string, fields ...any) { l.record("debug", msg, fields) }

// startupSummary returns the entry carrying active_monitors, which is the line
// Start ends on and the one an operator greps for.
func (l *recordingLogger) startupSummary(t *testing.T) logEntry {
	t.Helper()
	for _, e := range l.entries {
		if _, ok := e.field("active_monitors"); ok {
			return e
		}
	}
	t.Fatalf("Start logged no summary carrying active_monitors; entries: %+v", l.entries)
	return logEntry{}
}

// Start used to report len(tenants) as active_monitors, so a pool where every
// monitor failed to come up still logged "started successfully" with a non-zero
// count. The summary has to count monitors that actually started.
func TestMonitorPoolManager_StartReportsMonitorsThatActuallyStarted(t *testing.T) {
	newTenants := func() []*domain.KeycloakTenant {
		return []*domain.KeycloakTenant{
			{TenantID: "tenant-a", Name: "A", ServerURL: "https://a.example.invalid", AdminRealm: "master", ClientID: "admin-cli", Enabled: true},
			{TenantID: "tenant-b", Name: "B", ServerURL: "https://b.example.invalid", AdminRealm: "master", ClientID: "admin-cli", Enabled: true},
		}
	}

	unreachable := errors.New("keycloak unreachable")

	tests := []struct {
		name       string
		failFor    map[string]error
		wantLevel  string
		wantActive int
	}{
		{
			name:       "every monitor starts",
			wantLevel:  "info",
			wantActive: 2,
		},
		{
			name:       "no monitor starts",
			failFor:    map[string]error{"tenant-a": unreachable, "tenant-b": unreachable},
			wantLevel:  "error",
			wantActive: 0,
		},
		{
			name:       "one monitor of two starts",
			failFor:    map[string]error{"tenant-b": unreachable},
			wantLevel:  "warn",
			wantActive: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tenants := newTenants()
			log := &recordingLogger{}
			manager := NewMonitorPoolManager(
				&mockService{tenants: tenants},
				&mockMonitorFactory{createClientErrFor: tt.failFor},
				nil, // globalConfig
				log,
				nil, nil, nil, nil, nil, nil, nil,
			)

			if err := manager.Start(context.Background()); err != nil {
				t.Fatalf("Start: %v", err)
			}

			got := log.startupSummary(t)

			if got.level != tt.wantLevel {
				t.Errorf("summary level = %q, want %q (message %q)", got.level, tt.wantLevel, got.msg)
			}

			active, _ := got.field("active_monitors")
			if active != tt.wantActive {
				t.Errorf("active_monitors = %v, want %d: startup reported monitors it did not start", active, tt.wantActive)
			}

			// A degraded start has to carry both numbers, or the count alone
			// cannot be read as good or bad.
			if tt.wantActive != len(tenants) {
				requested, ok := got.field("requested_monitors")
				if !ok {
					t.Error("a degraded startup must also report requested_monitors")
				} else if requested != len(tenants) {
					t.Errorf("requested_monitors = %v, want %d", requested, len(tenants))
				}
			}
		})
	}
}
