package keycloak

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/config"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/pkg/keycloakadmin"
	"github.com/rs/zerolog"
)

// ============================================================================
// TEST HELPERS
// ============================================================================

func newTestLogger() *logger.Logger {
	return logger.New(zerolog.New(io.Discard))
}

func newTestConfig() *config.KeycloakInstanceConfig {
	return &config.KeycloakInstanceConfig{
		Realms: []string{"test-realm"},
		Polling: config.KeycloakPollingConfig{
			MetricsInterval:   100 * time.Millisecond,
			EventsInterval:    100 * time.Millisecond,
			HealthInterval:    100 * time.Millisecond,
			RealmInfoInterval: 100 * time.Millisecond,
		},
	}
}

// ============================================================================
// MOCK IMPLEMENTATIONS
// ============================================================================

// mockAdminAPI implements AdminAPI interface
type mockAdminAPI struct {
	mu sync.Mutex

	// Function fields for customization
	CloseFunc                func() error
	IsHealthyFunc            func(ctx context.Context) bool
	GetRealmsFunc            func() []string
	GetAllRealmsFunc         func(ctx context.Context) ([]*keycloakadmin.RealmRepresentation, error)
	GetRealmInfoFunc         func(ctx context.Context, realmName string) (*keycloakadmin.RealmRepresentation, error)
	GetAuthFlowsFunc         func(ctx context.Context, realmName string) ([]keycloakadmin.AuthenticationFlowRepresentation, error)
	GetFlowExecutionsFunc    func(ctx context.Context, realmName, flowAlias string) ([]keycloakadmin.AuthenticationExecutionInfoRepresentation, error)
	GetRealmUsersFunc        func(ctx context.Context, realmName string, first, max int) ([]keycloakadmin.UserRepresentation, error)
	GetUsersFunc             func(ctx context.Context, realmName string, first, max int) ([]*keycloakadmin.UserRepresentation, error)
	GetUserByIDFunc          func(ctx context.Context, realmName, userID string) (*keycloakadmin.UserRepresentation, error)
	GetUserDetailsFunc       func(ctx context.Context, realmName, userID string) (*keycloakadmin.UserDetails, error)
	GetUserRoleMappingsFunc  func(ctx context.Context, realmName, userID string) (*keycloakadmin.RoleMappingsRepresentation, error)
	GetClientsFunc           func(ctx context.Context, realmName string) ([]*keycloakadmin.ClientRepresentation, error)
	GetUsersCountFunc        func(ctx context.Context, realmName string) (int, error)
	GetEnabledUsersCountFunc func(ctx context.Context, realmName string) (enabled, disabled int, err error)
	GetSessionsCountFunc     func(ctx context.Context, realmName string) (active, offline int, err error)
	GetEventsFunc            func(ctx context.Context, realmName string, options *keycloakadmin.EventQueryOptions) ([]*keycloakadmin.EventRepresentation, error)
	GetRecentEventsFunc      func(ctx context.Context, realmName string) ([]*keycloakadmin.EventRepresentation, error)
	GetHealthMetricsFunc     func(ctx context.Context) *keycloakadmin.HealthMetrics
	GetInfinispanMetricsFunc func(ctx context.Context) (*keycloakadmin.InfinispanMetrics, error)

	// Track calls
	calls map[string]int
}

func newMockAdminAPI() *mockAdminAPI {
	return &mockAdminAPI{
		calls: make(map[string]int),
	}
}

func (m *mockAdminAPI) trackCall(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls[name]++
}

func (m *mockAdminAPI) getCalls(name string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls[name]
}

func (m *mockAdminAPI) Close() error {
	m.trackCall("Close")
	if m.CloseFunc != nil {
		return m.CloseFunc()
	}
	return nil
}

func (m *mockAdminAPI) IsHealthy(ctx context.Context) bool {
	m.trackCall("IsHealthy")
	if m.IsHealthyFunc != nil {
		return m.IsHealthyFunc(ctx)
	}
	return true
}

func (m *mockAdminAPI) GetRealms() []string {
	m.trackCall("GetRealms")
	if m.GetRealmsFunc != nil {
		return m.GetRealmsFunc()
	}
	return []string{"test-realm"}
}

func (m *mockAdminAPI) GetAllRealms(ctx context.Context) ([]*keycloakadmin.RealmRepresentation, error) {
	m.trackCall("GetAllRealms")
	if m.GetAllRealmsFunc != nil {
		return m.GetAllRealmsFunc(ctx)
	}
	return []*keycloakadmin.RealmRepresentation{
		{ID: "realm-1", Realm: "test-realm", Enabled: true},
	}, nil
}

func (m *mockAdminAPI) GetAuthenticationFlows(ctx context.Context, realmName string) ([]keycloakadmin.AuthenticationFlowRepresentation, error) {
	m.trackCall("GetAuthenticationFlows")
	if m.GetAuthFlowsFunc != nil {
		return m.GetAuthFlowsFunc(ctx, realmName)
	}
	return nil, nil
}

func (m *mockAdminAPI) GetFlowExecutions(ctx context.Context, realmName, flowAlias string) ([]keycloakadmin.AuthenticationExecutionInfoRepresentation, error) {
	m.trackCall("GetFlowExecutions")
	if m.GetFlowExecutionsFunc != nil {
		return m.GetFlowExecutionsFunc(ctx, realmName, flowAlias)
	}
	return nil, nil
}

func (m *mockAdminAPI) GetRealmInfo(ctx context.Context, realmName string) (*keycloakadmin.RealmRepresentation, error) {
	m.trackCall("GetRealmInfo")
	if m.GetRealmInfoFunc != nil {
		return m.GetRealmInfoFunc(ctx, realmName)
	}
	return &keycloakadmin.RealmRepresentation{
		ID:              "realm-1",
		Realm:           realmName,
		DisplayName:     "Test Realm",
		Enabled:         true,
		EventsEnabled:   true,
		EventsListeners: []string{"jboss-logging"},
	}, nil
}

func (m *mockAdminAPI) GetRealmUsers(ctx context.Context, realmName string, first, max int) ([]keycloakadmin.UserRepresentation, error) {
	m.trackCall("GetRealmUsers")
	if m.GetRealmUsersFunc != nil {
		return m.GetRealmUsersFunc(ctx, realmName, first, max)
	}
	return []keycloakadmin.UserRepresentation{}, nil
}

func (m *mockAdminAPI) GetUsers(ctx context.Context, realmName string, first, max int) ([]*keycloakadmin.UserRepresentation, error) {
	m.trackCall("GetUsers")
	if m.GetUsersFunc != nil {
		return m.GetUsersFunc(ctx, realmName, first, max)
	}
	return []*keycloakadmin.UserRepresentation{}, nil
}

func (m *mockAdminAPI) GetUserByID(ctx context.Context, realmName, userID string) (*keycloakadmin.UserRepresentation, error) {
	m.trackCall("GetUserByID")
	if m.GetUserByIDFunc != nil {
		return m.GetUserByIDFunc(ctx, realmName, userID)
	}
	return &keycloakadmin.UserRepresentation{
		ID:       userID,
		Username: "testuser",
		Email:    "testuser@example.com",
	}, nil
}

func (m *mockAdminAPI) GetUserDetails(ctx context.Context, realmName, userID string) (*keycloakadmin.UserDetails, error) {
	m.trackCall("GetUserDetails")
	if m.GetUserDetailsFunc != nil {
		return m.GetUserDetailsFunc(ctx, realmName, userID)
	}
	return &keycloakadmin.UserDetails{}, nil
}

func (m *mockAdminAPI) GetUserRoleMappings(ctx context.Context, realmName, userID string) (*keycloakadmin.RoleMappingsRepresentation, error) {
	m.trackCall("GetUserRoleMappings")
	if m.GetUserRoleMappingsFunc != nil {
		return m.GetUserRoleMappingsFunc(ctx, realmName, userID)
	}
	return &keycloakadmin.RoleMappingsRepresentation{}, nil
}

func (m *mockAdminAPI) GetClients(ctx context.Context, realmName string) ([]*keycloakadmin.ClientRepresentation, error) {
	m.trackCall("GetClients")
	if m.GetClientsFunc != nil {
		return m.GetClientsFunc(ctx, realmName)
	}
	return []*keycloakadmin.ClientRepresentation{
		{ID: "client-1", ClientID: "test-client"},
	}, nil
}

func (m *mockAdminAPI) GetUsersCount(ctx context.Context, realmName string) (int, error) {
	m.trackCall("GetUsersCount")
	if m.GetUsersCountFunc != nil {
		return m.GetUsersCountFunc(ctx, realmName)
	}
	return 100, nil
}

func (m *mockAdminAPI) GetEnabledUsersCount(ctx context.Context, realmName string) (enabled, disabled int, err error) {
	m.trackCall("GetEnabledUsersCount")
	if m.GetEnabledUsersCountFunc != nil {
		return m.GetEnabledUsersCountFunc(ctx, realmName)
	}
	return 90, 10, nil
}

func (m *mockAdminAPI) GetSessionsCount(ctx context.Context, realmName string) (active, offline int, err error) {
	m.trackCall("GetSessionsCount")
	if m.GetSessionsCountFunc != nil {
		return m.GetSessionsCountFunc(ctx, realmName)
	}
	return 50, 20, nil
}

func (m *mockAdminAPI) GetEvents(ctx context.Context, realmName string, options *keycloakadmin.EventQueryOptions) ([]*keycloakadmin.EventRepresentation, error) {
	m.trackCall("GetEvents")
	if m.GetEventsFunc != nil {
		return m.GetEventsFunc(ctx, realmName, options)
	}
	return []*keycloakadmin.EventRepresentation{}, nil
}

func (m *mockAdminAPI) GetRecentEvents(ctx context.Context, realmName string) ([]*keycloakadmin.EventRepresentation, error) {
	m.trackCall("GetRecentEvents")
	if m.GetRecentEventsFunc != nil {
		return m.GetRecentEventsFunc(ctx, realmName)
	}
	return []*keycloakadmin.EventRepresentation{
		{
			Time:      time.Now().UnixMilli(),
			Type:      "LOGIN",
			RealmID:   "realm-1",
			ClientID:  "test-client",
			UserID:    "user-1",
			SessionID: "session-1",
			IPAddress: "192.168.1.1",
			Details:   map[string]string{"event_id": "evt-1"},
		},
	}, nil
}

func (m *mockAdminAPI) GetHealthMetrics(ctx context.Context) *keycloakadmin.HealthMetrics {
	m.trackCall("GetHealthMetrics")
	if m.GetHealthMetricsFunc != nil {
		return m.GetHealthMetricsFunc(ctx)
	}
	return &keycloakadmin.HealthMetrics{
		Status:        "healthy",
		ResponseTime:  50,
		ServerVersion: "21.0.0",
		UptimeMillis:  3600000,
		MemoryUsed:    500000000,
		MemoryMax:     1000000000,
		MemoryFree:    500000000,
	}
}

func (m *mockAdminAPI) GetInfinispanMetrics(ctx context.Context) (*keycloakadmin.InfinispanMetrics, error) {
	m.trackCall("GetInfinispanMetrics")
	if m.GetInfinispanMetricsFunc != nil {
		return m.GetInfinispanMetricsFunc(ctx)
	}
	return &keycloakadmin.InfinispanMetrics{}, nil
}

// mockEventRepository implements MonitorEventRepository interface
type mockEventRepository struct {
	mu sync.Mutex

	SaveEventsFunc        func(ctx context.Context, events []*domain.KeycloakEvent) error
	CountEventsByTypeFunc func(ctx context.Context, tenantID, realmName, eventType string, from, to time.Time) (int64, error)

	savedEvents []*domain.KeycloakEvent
	calls       map[string]int
}

func newMockEventRepository() *mockEventRepository {
	return &mockEventRepository{
		savedEvents: []*domain.KeycloakEvent{},
		calls:       make(map[string]int),
	}
}

func (m *mockEventRepository) SaveEvents(ctx context.Context, events []*domain.KeycloakEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls["SaveEvents"]++
	m.savedEvents = append(m.savedEvents, events...)
	if m.SaveEventsFunc != nil {
		return m.SaveEventsFunc(ctx, events)
	}
	return nil
}

func (m *mockEventRepository) CountEventsByType(ctx context.Context, tenantID, realmName, eventType string, from, to time.Time) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls["CountEventsByType"]++
	if m.CountEventsByTypeFunc != nil {
		return m.CountEventsByTypeFunc(ctx, tenantID, realmName, eventType, from, to)
	}
	return 10, nil
}

func (m *mockEventRepository) getSavedEvents() []*domain.KeycloakEvent {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.savedEvents
}

// mockGeneralEventRepository implements EventSaver interface
type mockGeneralEventRepository struct {
	mu sync.Mutex

	SaveFunc func(ctx context.Context, event *domain.Event) error

	savedEvents []*domain.Event
	calls       int
}

func newMockGeneralEventRepository() *mockGeneralEventRepository {
	return &mockGeneralEventRepository{
		savedEvents: []*domain.Event{},
	}
}

func (m *mockGeneralEventRepository) Save(ctx context.Context, event *domain.Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	m.savedEvents = append(m.savedEvents, event)
	if m.SaveFunc != nil {
		return m.SaveFunc(ctx, event)
	}
	return nil
}

// MergeKeycloakEvent behaves like Save in tests (the merge is a DB concern
// exercised in the repository integration tests).
func (m *mockGeneralEventRepository) MergeKeycloakEvent(ctx context.Context, event *domain.Event) error {
	return m.Save(ctx, event)
}

func (m *mockGeneralEventRepository) getSavedEvents() []*domain.Event {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.savedEvents
}

// mockMetricsRepository implements MonitorMetricsRepository interface
type mockMetricsRepository struct {
	mu sync.Mutex

	SaveMetricsFunc func(ctx context.Context, metrics *domain.KeycloakMetrics) error

	savedMetrics []*domain.KeycloakMetrics
	calls        int
}

func newMockMetricsRepository() *mockMetricsRepository {
	return &mockMetricsRepository{
		savedMetrics: []*domain.KeycloakMetrics{},
	}
}

func (m *mockMetricsRepository) SaveMetrics(ctx context.Context, metrics *domain.KeycloakMetrics) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	m.savedMetrics = append(m.savedMetrics, metrics)
	if m.SaveMetricsFunc != nil {
		return m.SaveMetricsFunc(ctx, metrics)
	}
	return nil
}

func (m *mockMetricsRepository) getSavedMetrics() []*domain.KeycloakMetrics {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.savedMetrics
}

// mockHealthRepository implements MonitorHealthRepository interface
type mockHealthRepository struct {
	mu sync.Mutex

	SaveHealthFunc func(ctx context.Context, health *domain.KeycloakHealth) error

	savedHealth []*domain.KeycloakHealth
	calls       int
}

func newMockHealthRepository() *mockHealthRepository {
	return &mockHealthRepository{
		savedHealth: []*domain.KeycloakHealth{},
	}
}

func (m *mockHealthRepository) SaveHealth(ctx context.Context, health *domain.KeycloakHealth) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	m.savedHealth = append(m.savedHealth, health)
	if m.SaveHealthFunc != nil {
		return m.SaveHealthFunc(ctx, health)
	}
	return nil
}

func (m *mockHealthRepository) getSavedHealth() []*domain.KeycloakHealth {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.savedHealth
}

// mockRealmRepository implements MonitorRealmRepository interface
type mockRealmRepository struct {
	mu sync.Mutex

	SaveRealmFunc func(ctx context.Context, realm *domain.KeycloakRealmInfo) error
	GetRealmsFunc func(ctx context.Context, tenantID string) ([]*domain.KeycloakRealmInfo, error)

	savedRealms []*domain.KeycloakRealmInfo
	calls       map[string]int
}

func newMockRealmRepository() *mockRealmRepository {
	return &mockRealmRepository{
		savedRealms: []*domain.KeycloakRealmInfo{},
		calls:       make(map[string]int),
	}
}

func (m *mockRealmRepository) SaveRealm(ctx context.Context, realm *domain.KeycloakRealmInfo) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls["SaveRealm"]++
	m.savedRealms = append(m.savedRealms, realm)
	if m.SaveRealmFunc != nil {
		return m.SaveRealmFunc(ctx, realm)
	}
	return nil
}

func (m *mockRealmRepository) GetRealms(ctx context.Context, tenantID string) ([]*domain.KeycloakRealmInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls["GetRealms"]++
	if m.GetRealmsFunc != nil {
		return m.GetRealmsFunc(ctx, tenantID)
	}
	return []*domain.KeycloakRealmInfo{
		{TenantID: tenantID, RealmName: "test-realm", Enabled: true},
	}, nil
}

func (m *mockRealmRepository) getSavedRealms() []*domain.KeycloakRealmInfo {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.savedRealms
}

// mockEventAlertConverter implements EventAlertConverter interface
type mockEventAlertConverter struct {
	mu sync.Mutex

	ConvertEventFunc func(ctx context.Context, event *domain.KeycloakEvent) error

	convertedEvents []*domain.KeycloakEvent
	calls           int
}

func newMockEventAlertConverter() *mockEventAlertConverter {
	return &mockEventAlertConverter{
		convertedEvents: []*domain.KeycloakEvent{},
	}
}

func (m *mockEventAlertConverter) ConvertEvent(ctx context.Context, event *domain.KeycloakEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	m.convertedEvents = append(m.convertedEvents, event)
	if m.ConvertEventFunc != nil {
		return m.ConvertEventFunc(ctx, event)
	}
	return nil
}

func (m *mockEventAlertConverter) getConvertedEvents() []*domain.KeycloakEvent {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.convertedEvents
}

// mockHealthAlertManager implements HealthAlertManager interface
type mockHealthAlertManager struct {
	mu sync.Mutex

	ProcessHealthStatusFunc func(ctx context.Context, tenantID, tenantName, status, errorMessage string, responseTime int) error

	processedStatuses []healthStatusCall
	calls             int
}

type healthStatusCall struct {
	tenantID     string
	tenantName   string
	status       string
	errorMessage string
	responseTime int
}

func newMockHealthAlertManager() *mockHealthAlertManager {
	return &mockHealthAlertManager{
		processedStatuses: []healthStatusCall{},
	}
}

func (m *mockHealthAlertManager) ProcessHealthStatus(ctx context.Context, tenantID, tenantName, status, errorMessage string, responseTime int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	m.processedStatuses = append(m.processedStatuses, healthStatusCall{
		tenantID:     tenantID,
		tenantName:   tenantName,
		status:       status,
		errorMessage: errorMessage,
		responseTime: responseTime,
	})
	if m.ProcessHealthStatusFunc != nil {
		return m.ProcessHealthStatusFunc(ctx, tenantID, tenantName, status, errorMessage, responseTime)
	}
	return nil
}

func (m *mockHealthAlertManager) getProcessedStatuses() []healthStatusCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.processedStatuses
}

// ============================================================================
// TESTS
// ============================================================================

func TestNewMonitor(t *testing.T) {
	client := newMockAdminAPI()
	cfg := newTestConfig()
	log := newTestLogger()
	eventRepo := newMockEventRepository()
	generalEventRepo := newMockGeneralEventRepository()
	metricsRepo := newMockMetricsRepository()
	healthRepo := newMockHealthRepository()
	realmRepo := newMockRealmRepository()
	eventAlertConverter := newMockEventAlertConverter()
	healthAlertManager := newMockHealthAlertManager()

	monitor := NewMonitor(
		"tenant-1",
		"Test Tenant",
		client,
		cfg,
		log,
		eventRepo,
		generalEventRepo,
		metricsRepo,
		healthRepo,
		realmRepo,
		eventAlertConverter,
		healthAlertManager,
	)

	if monitor == nil {
		t.Fatal("NewMonitor returned nil")
		return
	}

	if monitor.tenantID != "tenant-1" {
		t.Errorf("tenantID = %s, want tenant-1", monitor.tenantID)
	}

	if monitor.tenantName != "Test Tenant" {
		t.Errorf("tenantName = %s, want Test Tenant", monitor.tenantName)
	}
}

func TestMonitor_StartStop(t *testing.T) {
	client := newMockAdminAPI()
	cfg := newTestConfig()
	log := newTestLogger()
	eventRepo := newMockEventRepository()
	generalEventRepo := newMockGeneralEventRepository()
	metricsRepo := newMockMetricsRepository()
	healthRepo := newMockHealthRepository()
	realmRepo := newMockRealmRepository()
	eventAlertConverter := newMockEventAlertConverter()
	healthAlertManager := newMockHealthAlertManager()

	monitor := NewMonitor(
		"tenant-1",
		"Test Tenant",
		client,
		cfg,
		log,
		eventRepo,
		generalEventRepo,
		metricsRepo,
		healthRepo,
		realmRepo,
		eventAlertConverter,
		healthAlertManager,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start should not return error
	err := monitor.Start(ctx)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Give some time for workers to start and do initial collection
	time.Sleep(200 * time.Millisecond)

	// Stop should not return error, and should not sit on its drain timeout.
	//
	// Stop used to wait on a done channel that nothing ever closed, so it
	// blocked the full 10s and then logged a spurious stop timeout on every
	// shutdown. This test passed throughout, because it only checked the
	// error, so assert the duration too: with one monitor per tenant and fx
	// running OnStop hooks sequentially, a 10s stall each blows the 15s fx
	// shutdown timeout.
	start := time.Now()
	err = monitor.Stop()
	if err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("Stop() took %s, want a prompt drain: the polling workers are not being awaited", elapsed)
	}
}

func TestMonitor_collectEvents(t *testing.T) {
	client := newMockAdminAPI()
	cfg := newTestConfig()
	log := newTestLogger()
	eventRepo := newMockEventRepository()
	generalEventRepo := newMockGeneralEventRepository()
	metricsRepo := newMockMetricsRepository()
	healthRepo := newMockHealthRepository()
	realmRepo := newMockRealmRepository()
	eventAlertConverter := newMockEventAlertConverter()
	healthAlertManager := newMockHealthAlertManager()

	// Setup mock to return events
	client.GetRecentEventsFunc = func(ctx context.Context, realmName string) ([]*keycloakadmin.EventRepresentation, error) {
		return []*keycloakadmin.EventRepresentation{
			{
				Time:      time.Now().UnixMilli(),
				Type:      "LOGIN",
				RealmID:   "realm-1",
				ClientID:  "test-client",
				UserID:    "user-1",
				SessionID: "session-1",
				IPAddress: "192.168.1.1",
				Details:   map[string]string{"event_id": "evt-1"},
			},
			{
				Time:      time.Now().UnixMilli(),
				Type:      "LOGIN_ERROR",
				RealmID:   "realm-1",
				ClientID:  "test-client",
				UserID:    "user-2",
				SessionID: "session-2",
				IPAddress: "192.168.1.2",
				Error:     "invalid_credentials",
				Details:   map[string]string{"event_id": "evt-2"},
			},
		}, nil
	}

	monitor := NewMonitor(
		"tenant-1",
		"Test Tenant",
		client,
		cfg,
		log,
		eventRepo,
		generalEventRepo,
		metricsRepo,
		healthRepo,
		realmRepo,
		eventAlertConverter,
		healthAlertManager,
	)

	ctx := context.Background()
	monitor.collectEvents(ctx)

	// Check that events were saved
	savedEvents := eventRepo.getSavedEvents()
	if len(savedEvents) != 2 {
		t.Errorf("SaveEvents called with %d events, want 2", len(savedEvents))
	}

	// Check event details
	if len(savedEvents) > 0 {
		if savedEvents[0].EventType != "LOGIN" {
			t.Errorf("First event type = %s, want LOGIN", savedEvents[0].EventType)
		}
		if savedEvents[0].TenantID != "tenant-1" {
			t.Errorf("First event tenantID = %s, want tenant-1", savedEvents[0].TenantID)
		}
	}

	// Check that general events were saved
	generalEvents := generalEventRepo.getSavedEvents()
	if len(generalEvents) != 2 {
		t.Errorf("General events saved = %d, want 2", len(generalEvents))
	}

	// Check that event converter was called
	convertedEvents := eventAlertConverter.getConvertedEvents()
	if len(convertedEvents) != 2 {
		t.Errorf("EventAlertConverter called %d times, want 2", len(convertedEvents))
	}
}

func TestMonitor_collectEvents_noRealms(t *testing.T) {
	client := newMockAdminAPI()
	cfg := &config.KeycloakInstanceConfig{
		Realms: []string{}, // No realms configured
		Polling: config.KeycloakPollingConfig{
			EventsInterval: 100 * time.Millisecond,
		},
	}
	log := newTestLogger()
	eventRepo := newMockEventRepository()
	generalEventRepo := newMockGeneralEventRepository()
	metricsRepo := newMockMetricsRepository()
	healthRepo := newMockHealthRepository()
	realmRepo := newMockRealmRepository()

	// Return empty realms from repo
	realmRepo.GetRealmsFunc = func(ctx context.Context, tenantID string) ([]*domain.KeycloakRealmInfo, error) {
		return []*domain.KeycloakRealmInfo{}, nil
	}

	monitor := NewMonitor(
		"tenant-1",
		"Test Tenant",
		client,
		cfg,
		log,
		eventRepo,
		generalEventRepo,
		metricsRepo,
		healthRepo,
		realmRepo,
		nil,
		nil,
	)

	ctx := context.Background()
	monitor.collectEvents(ctx)

	// No events should be saved when there are no realms
	savedEvents := eventRepo.getSavedEvents()
	if len(savedEvents) != 0 {
		t.Errorf("SaveEvents called with %d events, want 0", len(savedEvents))
	}
}

func TestMonitor_collectEvents_apiError(t *testing.T) {
	client := newMockAdminAPI()
	cfg := newTestConfig()
	log := newTestLogger()
	eventRepo := newMockEventRepository()
	generalEventRepo := newMockGeneralEventRepository()
	metricsRepo := newMockMetricsRepository()
	healthRepo := newMockHealthRepository()
	realmRepo := newMockRealmRepository()

	// Setup mock to return error
	client.GetRecentEventsFunc = func(ctx context.Context, realmName string) ([]*keycloakadmin.EventRepresentation, error) {
		return nil, errors.New("connection refused")
	}

	monitor := NewMonitor(
		"tenant-1",
		"Test Tenant",
		client,
		cfg,
		log,
		eventRepo,
		generalEventRepo,
		metricsRepo,
		healthRepo,
		realmRepo,
		nil,
		nil,
	)

	ctx := context.Background()
	monitor.collectEvents(ctx)

	// No events should be saved when API returns error
	savedEvents := eventRepo.getSavedEvents()
	if len(savedEvents) != 0 {
		t.Errorf("SaveEvents called with %d events, want 0", len(savedEvents))
	}
}

func TestMonitor_collectMetrics(t *testing.T) {
	client := newMockAdminAPI()
	cfg := newTestConfig()
	log := newTestLogger()
	eventRepo := newMockEventRepository()
	generalEventRepo := newMockGeneralEventRepository()
	metricsRepo := newMockMetricsRepository()
	healthRepo := newMockHealthRepository()
	realmRepo := newMockRealmRepository()

	monitor := NewMonitor(
		"tenant-1",
		"Test Tenant",
		client,
		cfg,
		log,
		eventRepo,
		generalEventRepo,
		metricsRepo,
		healthRepo,
		realmRepo,
		nil,
		nil,
	)

	ctx := context.Background()
	monitor.collectMetrics(ctx)

	// Check that metrics were saved
	savedMetrics := metricsRepo.getSavedMetrics()
	if len(savedMetrics) != 1 {
		t.Errorf("SaveMetrics called %d times, want 1", len(savedMetrics))
	}

	// Check metrics values
	if len(savedMetrics) > 0 {
		m := savedMetrics[0]
		if m.TenantID != "tenant-1" {
			t.Errorf("Metrics tenantID = %s, want tenant-1", m.TenantID)
		}
		if m.RealmName != "test-realm" {
			t.Errorf("Metrics realmName = %s, want test-realm", m.RealmName)
		}
		if m.TotalUsers != 100 {
			t.Errorf("Metrics totalUsers = %d, want 100", m.TotalUsers)
		}
		if m.EnabledUsers != 90 {
			t.Errorf("Metrics enabledUsers = %d, want 90", m.EnabledUsers)
		}
		if m.DisabledUsers != 10 {
			t.Errorf("Metrics disabledUsers = %d, want 10", m.DisabledUsers)
		}
		if m.ActiveSessions != 50 {
			t.Errorf("Metrics activeSessions = %d, want 50", m.ActiveSessions)
		}
		if m.OfflineSessions != 20 {
			t.Errorf("Metrics offlineSessions = %d, want 20", m.OfflineSessions)
		}
		if m.TotalClients != 1 {
			t.Errorf("Metrics totalClients = %d, want 1", m.TotalClients)
		}
	}

	// Check that API was called. total_users is now derived from
	// enabled+disabled, so the unfiltered GetUsersCount is no longer called.
	if client.getCalls("GetUsersCount") != 0 {
		t.Errorf("GetUsersCount called %d times, want 0", client.getCalls("GetUsersCount"))
	}
	if client.getCalls("GetEnabledUsersCount") != 1 {
		t.Errorf("GetEnabledUsersCount called %d times, want 1", client.getCalls("GetEnabledUsersCount"))
	}
	if client.getCalls("GetSessionsCount") != 1 {
		t.Errorf("GetSessionsCount called %d times, want 1", client.getCalls("GetSessionsCount"))
	}
	if client.getCalls("GetClients") != 1 {
		t.Errorf("GetClients called %d times, want 1", client.getCalls("GetClients"))
	}
}

func TestMonitor_collectMetrics_apiError(t *testing.T) {
	client := newMockAdminAPI()
	cfg := newTestConfig()
	log := newTestLogger()
	eventRepo := newMockEventRepository()
	generalEventRepo := newMockGeneralEventRepository()
	metricsRepo := newMockMetricsRepository()
	healthRepo := newMockHealthRepository()
	realmRepo := newMockRealmRepository()

	// Setup mock to return error for users count. total_users is derived from
	// the enabled/disabled counts, so that call is the one that must fail.
	client.GetEnabledUsersCountFunc = func(ctx context.Context, realmName string) (int, int, error) {
		return 0, 0, errors.New("connection refused")
	}

	monitor := NewMonitor(
		"tenant-1",
		"Test Tenant",
		client,
		cfg,
		log,
		eventRepo,
		generalEventRepo,
		metricsRepo,
		healthRepo,
		realmRepo,
		nil,
		nil,
	)

	ctx := context.Background()
	monitor.collectMetrics(ctx)

	// No metrics should be saved when API returns error
	savedMetrics := metricsRepo.getSavedMetrics()
	if len(savedMetrics) != 0 {
		t.Errorf("SaveMetrics called %d times, want 0", len(savedMetrics))
	}
}

func TestMonitor_collectHealth(t *testing.T) {
	client := newMockAdminAPI()
	cfg := newTestConfig()
	log := newTestLogger()
	eventRepo := newMockEventRepository()
	generalEventRepo := newMockGeneralEventRepository()
	metricsRepo := newMockMetricsRepository()
	healthRepo := newMockHealthRepository()
	realmRepo := newMockRealmRepository()
	healthAlertManager := newMockHealthAlertManager()

	monitor := NewMonitor(
		"tenant-1",
		"Test Tenant",
		client,
		cfg,
		log,
		eventRepo,
		generalEventRepo,
		metricsRepo,
		healthRepo,
		realmRepo,
		nil,
		healthAlertManager,
	)

	ctx := context.Background()
	monitor.collectHealth(ctx)

	// Check that health was saved
	savedHealth := healthRepo.getSavedHealth()
	if len(savedHealth) != 1 {
		t.Errorf("SaveHealth called %d times, want 1", len(savedHealth))
	}

	// Check health values
	if len(savedHealth) > 0 {
		h := savedHealth[0]
		if h.TenantID != "tenant-1" {
			t.Errorf("Health tenantID = %s, want tenant-1", h.TenantID)
		}
		if h.Status != "healthy" {
			t.Errorf("Health status = %s, want healthy", h.Status)
		}
		if h.ResponseTime != 50 {
			t.Errorf("Health responseTime = %d, want 50", h.ResponseTime)
		}
		if h.ServerVersion != "21.0.0" {
			t.Errorf("Health serverVersion = %s, want 21.0.0", h.ServerVersion)
		}
	}

	// Check that health alert manager was called
	processedStatuses := healthAlertManager.getProcessedStatuses()
	if len(processedStatuses) != 1 {
		t.Errorf("ProcessHealthStatus called %d times, want 1", len(processedStatuses))
	}

	if len(processedStatuses) > 0 {
		status := processedStatuses[0]
		if status.tenantID != "tenant-1" {
			t.Errorf("ProcessHealthStatus tenantID = %s, want tenant-1", status.tenantID)
		}
		if status.status != "healthy" {
			t.Errorf("ProcessHealthStatus status = %s, want healthy", status.status)
		}
	}
}

func TestMonitor_collectHealth_unhealthy(t *testing.T) {
	client := newMockAdminAPI()
	cfg := newTestConfig()
	log := newTestLogger()
	eventRepo := newMockEventRepository()
	generalEventRepo := newMockGeneralEventRepository()
	metricsRepo := newMockMetricsRepository()
	healthRepo := newMockHealthRepository()
	realmRepo := newMockRealmRepository()
	healthAlertManager := newMockHealthAlertManager()

	// Setup mock to return unhealthy status
	client.GetHealthMetricsFunc = func(ctx context.Context) *keycloakadmin.HealthMetrics {
		return &keycloakadmin.HealthMetrics{
			Status: "unhealthy",
			Error:  "connection timeout",
		}
	}

	monitor := NewMonitor(
		"tenant-1",
		"Test Tenant",
		client,
		cfg,
		log,
		eventRepo,
		generalEventRepo,
		metricsRepo,
		healthRepo,
		realmRepo,
		nil,
		healthAlertManager,
	)

	ctx := context.Background()
	monitor.collectHealth(ctx)

	// Check that health was saved with error
	savedHealth := healthRepo.getSavedHealth()
	if len(savedHealth) != 1 {
		t.Fatalf("SaveHealth called %d times, want 1", len(savedHealth))
	}

	h := savedHealth[0]
	if h.Status != "unhealthy" {
		t.Errorf("Health status = %s, want unhealthy", h.Status)
	}
	if h.ErrorMessage != "connection timeout" {
		t.Errorf("Health errorMessage = %s, want 'connection timeout'", h.ErrorMessage)
	}

	// Check that health alert manager was called with error
	processedStatuses := healthAlertManager.getProcessedStatuses()
	if len(processedStatuses) != 1 {
		t.Fatalf("ProcessHealthStatus called %d times, want 1", len(processedStatuses))
	}

	status := processedStatuses[0]
	if status.status != "unhealthy" {
		t.Errorf("ProcessHealthStatus status = %s, want unhealthy", status.status)
	}
	if status.errorMessage != "connection timeout" {
		t.Errorf("ProcessHealthStatus errorMessage = %s, want 'connection timeout'", status.errorMessage)
	}
}

func TestMonitor_collectRealmInfo(t *testing.T) {
	client := newMockAdminAPI()
	cfg := newTestConfig()
	log := newTestLogger()
	eventRepo := newMockEventRepository()
	generalEventRepo := newMockGeneralEventRepository()
	metricsRepo := newMockMetricsRepository()
	healthRepo := newMockHealthRepository()
	realmRepo := newMockRealmRepository()

	monitor := NewMonitor(
		"tenant-1",
		"Test Tenant",
		client,
		cfg,
		log,
		eventRepo,
		generalEventRepo,
		metricsRepo,
		healthRepo,
		realmRepo,
		nil,
		nil,
	)

	ctx := context.Background()
	monitor.collectRealmInfo(ctx)

	// Check that realm info was saved
	savedRealms := realmRepo.getSavedRealms()
	if len(savedRealms) != 1 {
		t.Errorf("SaveRealm called %d times, want 1", len(savedRealms))
	}

	// Check realm values
	if len(savedRealms) > 0 {
		r := savedRealms[0]
		if r.TenantID != "tenant-1" {
			t.Errorf("Realm tenantID = %s, want tenant-1", r.TenantID)
		}
		if r.RealmName != "test-realm" {
			t.Errorf("Realm name = %s, want test-realm", r.RealmName)
		}
		if !r.Enabled {
			t.Error("Realm should be enabled")
		}
		if !r.IsHealthy {
			t.Error("Realm should be healthy")
		}
	}

	// Check that API was called
	if client.getCalls("GetRealmInfo") != 1 {
		t.Errorf("GetRealmInfo called %d times, want 1", client.getCalls("GetRealmInfo"))
	}
}

func TestMonitor_collectRealmInfo_disabledRealm(t *testing.T) {
	client := newMockAdminAPI()
	cfg := newTestConfig()
	log := newTestLogger()
	eventRepo := newMockEventRepository()
	generalEventRepo := newMockGeneralEventRepository()
	metricsRepo := newMockMetricsRepository()
	healthRepo := newMockHealthRepository()
	realmRepo := newMockRealmRepository()

	// Setup mock to return disabled realm
	client.GetRealmInfoFunc = func(ctx context.Context, realmName string) (*keycloakadmin.RealmRepresentation, error) {
		return &keycloakadmin.RealmRepresentation{
			ID:      "realm-1",
			Realm:   realmName,
			Enabled: false,
		}, nil
	}

	monitor := NewMonitor(
		"tenant-1",
		"Test Tenant",
		client,
		cfg,
		log,
		eventRepo,
		generalEventRepo,
		metricsRepo,
		healthRepo,
		realmRepo,
		nil,
		nil,
	)

	ctx := context.Background()
	monitor.collectRealmInfo(ctx)

	// Check that realm info was saved with health message
	savedRealms := realmRepo.getSavedRealms()
	if len(savedRealms) != 1 {
		t.Fatalf("SaveRealm called %d times, want 1", len(savedRealms))
	}

	r := savedRealms[0]
	if r.Enabled {
		t.Error("Realm should be disabled")
	}
	if r.IsHealthy {
		t.Error("Disabled realm should not be healthy")
	}
	if r.HealthMessage != "Realm is disabled" {
		t.Errorf("Health message = %s, want 'Realm is disabled'", r.HealthMessage)
	}
}

func TestMonitor_collectRealmInfo_eventsNotEnabled(t *testing.T) {
	client := newMockAdminAPI()
	cfg := newTestConfig()
	log := newTestLogger()
	eventRepo := newMockEventRepository()
	generalEventRepo := newMockGeneralEventRepository()
	metricsRepo := newMockMetricsRepository()
	healthRepo := newMockHealthRepository()
	realmRepo := newMockRealmRepository()

	// Setup mock to return realm with events disabled
	client.GetRealmInfoFunc = func(ctx context.Context, realmName string) (*keycloakadmin.RealmRepresentation, error) {
		return &keycloakadmin.RealmRepresentation{
			ID:            "realm-1",
			Realm:         realmName,
			Enabled:       true,
			EventsEnabled: false,
		}, nil
	}

	monitor := NewMonitor(
		"tenant-1",
		"Test Tenant",
		client,
		cfg,
		log,
		eventRepo,
		generalEventRepo,
		metricsRepo,
		healthRepo,
		realmRepo,
		nil,
		nil,
	)

	ctx := context.Background()
	monitor.collectRealmInfo(ctx)

	// Check health message mentions events not enabled
	savedRealms := realmRepo.getSavedRealms()
	if len(savedRealms) != 1 {
		t.Fatalf("SaveRealm called %d times, want 1", len(savedRealms))
	}

	r := savedRealms[0]
	if r.HealthMessage != "Events are not enabled" {
		t.Errorf("Health message = %s, want 'Events are not enabled'", r.HealthMessage)
	}
}

func TestMonitor_collectRealmInfo_noEventListeners(t *testing.T) {
	client := newMockAdminAPI()
	cfg := newTestConfig()
	log := newTestLogger()
	eventRepo := newMockEventRepository()
	generalEventRepo := newMockGeneralEventRepository()
	metricsRepo := newMockMetricsRepository()
	healthRepo := newMockHealthRepository()
	realmRepo := newMockRealmRepository()

	// Setup mock to return realm with events enabled but no listeners
	client.GetRealmInfoFunc = func(ctx context.Context, realmName string) (*keycloakadmin.RealmRepresentation, error) {
		return &keycloakadmin.RealmRepresentation{
			ID:              "realm-1",
			Realm:           realmName,
			Enabled:         true,
			EventsEnabled:   true,
			EventsListeners: []string{},
		}, nil
	}

	monitor := NewMonitor(
		"tenant-1",
		"Test Tenant",
		client,
		cfg,
		log,
		eventRepo,
		generalEventRepo,
		metricsRepo,
		healthRepo,
		realmRepo,
		nil,
		nil,
	)

	ctx := context.Background()
	monitor.collectRealmInfo(ctx)

	// Check health message mentions no event listeners
	savedRealms := realmRepo.getSavedRealms()
	if len(savedRealms) != 1 {
		t.Fatalf("SaveRealm called %d times, want 1", len(savedRealms))
	}

	r := savedRealms[0]
	if r.HealthMessage != "No event listeners configured" {
		t.Errorf("Health message = %s, want 'No event listeners configured'", r.HealthMessage)
	}
}

func TestMonitor_collectRealmInfo_allRealms(t *testing.T) {
	client := newMockAdminAPI()
	// Config with no specific realms - should fetch all
	cfg := &config.KeycloakInstanceConfig{
		Realms: []string{},
		Polling: config.KeycloakPollingConfig{
			RealmInfoInterval: 100 * time.Millisecond,
		},
	}
	log := newTestLogger()
	eventRepo := newMockEventRepository()
	generalEventRepo := newMockGeneralEventRepository()
	metricsRepo := newMockMetricsRepository()
	healthRepo := newMockHealthRepository()
	realmRepo := newMockRealmRepository()

	// Setup mock to return multiple realms
	client.GetAllRealmsFunc = func(ctx context.Context) ([]*keycloakadmin.RealmRepresentation, error) {
		return []*keycloakadmin.RealmRepresentation{
			{ID: "realm-1", Realm: "realm-one", Enabled: true, EventsEnabled: true, EventsListeners: []string{"jboss-logging"}},
			{ID: "realm-2", Realm: "realm-two", Enabled: true, EventsEnabled: true, EventsListeners: []string{"jboss-logging"}},
			{ID: "realm-3", Realm: "realm-three", Enabled: false},
		}, nil
	}

	monitor := NewMonitor(
		"tenant-1",
		"Test Tenant",
		client,
		cfg,
		log,
		eventRepo,
		generalEventRepo,
		metricsRepo,
		healthRepo,
		realmRepo,
		nil,
		nil,
	)

	ctx := context.Background()
	monitor.collectRealmInfo(ctx)

	// Check that all realms were saved
	savedRealms := realmRepo.getSavedRealms()
	if len(savedRealms) != 3 {
		t.Errorf("SaveRealm called %d times, want 3", len(savedRealms))
	}

	// Check that GetAllRealms was called
	if client.getCalls("GetAllRealms") != 1 {
		t.Errorf("GetAllRealms called %d times, want 1", client.getCalls("GetAllRealms"))
	}
}

func TestMonitor_getRealmsToMonitor_fromConfig(t *testing.T) {
	client := newMockAdminAPI()
	cfg := &config.KeycloakInstanceConfig{
		Realms: []string{"realm-a", "realm-b"},
		Polling: config.KeycloakPollingConfig{
			EventsInterval: 100 * time.Millisecond,
		},
	}
	log := newTestLogger()
	eventRepo := newMockEventRepository()
	generalEventRepo := newMockGeneralEventRepository()
	metricsRepo := newMockMetricsRepository()
	healthRepo := newMockHealthRepository()
	realmRepo := newMockRealmRepository()

	monitor := NewMonitor(
		"tenant-1",
		"Test Tenant",
		client,
		cfg,
		log,
		eventRepo,
		generalEventRepo,
		metricsRepo,
		healthRepo,
		realmRepo,
		nil,
		nil,
	)

	ctx := context.Background()
	realms := monitor.getRealmsToMonitor(ctx)

	if len(realms) != 2 {
		t.Errorf("getRealmsToMonitor returned %d realms, want 2", len(realms))
	}

	if realms[0] != "realm-a" || realms[1] != "realm-b" {
		t.Errorf("getRealmsToMonitor returned %v, want [realm-a, realm-b]", realms)
	}
}

func TestMonitor_getRealmsToMonitor_fromDatabase(t *testing.T) {
	client := newMockAdminAPI()
	cfg := &config.KeycloakInstanceConfig{
		Realms: []string{}, // No realms configured
		Polling: config.KeycloakPollingConfig{
			EventsInterval: 100 * time.Millisecond,
		},
	}
	log := newTestLogger()
	eventRepo := newMockEventRepository()
	generalEventRepo := newMockGeneralEventRepository()
	metricsRepo := newMockMetricsRepository()
	healthRepo := newMockHealthRepository()
	realmRepo := newMockRealmRepository()

	// Setup mock to return realms from database
	realmRepo.GetRealmsFunc = func(ctx context.Context, tenantID string) ([]*domain.KeycloakRealmInfo, error) {
		return []*domain.KeycloakRealmInfo{
			{TenantID: tenantID, RealmName: "db-realm-1", Enabled: true},
			{TenantID: tenantID, RealmName: "db-realm-2", Enabled: true},
			{TenantID: tenantID, RealmName: "db-realm-3", Enabled: false}, // Disabled, should be excluded
		}, nil
	}

	monitor := NewMonitor(
		"tenant-1",
		"Test Tenant",
		client,
		cfg,
		log,
		eventRepo,
		generalEventRepo,
		metricsRepo,
		healthRepo,
		realmRepo,
		nil,
		nil,
	)

	ctx := context.Background()
	realms := monitor.getRealmsToMonitor(ctx)

	// Should only return enabled realms
	if len(realms) != 2 {
		t.Errorf("getRealmsToMonitor returned %d realms, want 2", len(realms))
	}

	expectedRealms := map[string]bool{"db-realm-1": true, "db-realm-2": true}
	for _, r := range realms {
		if !expectedRealms[r] {
			t.Errorf("Unexpected realm %s in result", r)
		}
	}
}

func TestMonitor_getRealmsToMonitor_databaseError(t *testing.T) {
	client := newMockAdminAPI()
	cfg := &config.KeycloakInstanceConfig{
		Realms: []string{}, // No realms configured
		Polling: config.KeycloakPollingConfig{
			EventsInterval: 100 * time.Millisecond,
		},
	}
	log := newTestLogger()
	eventRepo := newMockEventRepository()
	generalEventRepo := newMockGeneralEventRepository()
	metricsRepo := newMockMetricsRepository()
	healthRepo := newMockHealthRepository()
	realmRepo := newMockRealmRepository()

	// Setup mock to return error
	realmRepo.GetRealmsFunc = func(ctx context.Context, tenantID string) ([]*domain.KeycloakRealmInfo, error) {
		return nil, errors.New("database connection failed")
	}

	monitor := NewMonitor(
		"tenant-1",
		"Test Tenant",
		client,
		cfg,
		log,
		eventRepo,
		generalEventRepo,
		metricsRepo,
		healthRepo,
		realmRepo,
		nil,
		nil,
	)

	ctx := context.Background()
	realms := monitor.getRealmsToMonitor(ctx)

	// Should return empty list on error
	if len(realms) != 0 {
		t.Errorf("getRealmsToMonitor returned %d realms, want 0 on error", len(realms))
	}
}

func TestMonitor_collectEvents_repoError(t *testing.T) {
	client := newMockAdminAPI()
	cfg := newTestConfig()
	log := newTestLogger()
	eventRepo := newMockEventRepository()
	generalEventRepo := newMockGeneralEventRepository()
	metricsRepo := newMockMetricsRepository()
	healthRepo := newMockHealthRepository()
	realmRepo := newMockRealmRepository()

	// Setup mock to return error on save
	eventRepo.SaveEventsFunc = func(ctx context.Context, events []*domain.KeycloakEvent) error {
		return errors.New("database error")
	}

	monitor := NewMonitor(
		"tenant-1",
		"Test Tenant",
		client,
		cfg,
		log,
		eventRepo,
		generalEventRepo,
		metricsRepo,
		healthRepo,
		realmRepo,
		nil,
		nil,
	)

	ctx := context.Background()
	// Should not panic, just log error
	monitor.collectEvents(ctx)

	// General events should not be saved if keycloak events failed
	generalEvents := generalEventRepo.getSavedEvents()
	if len(generalEvents) != 0 {
		t.Errorf("General events saved = %d, want 0 after keycloak events error", len(generalEvents))
	}
}

func TestMonitor_collectEvents_userEnrichment(t *testing.T) {
	client := newMockAdminAPI()
	cfg := newTestConfig()
	log := newTestLogger()
	eventRepo := newMockEventRepository()
	generalEventRepo := newMockGeneralEventRepository()
	metricsRepo := newMockMetricsRepository()
	healthRepo := newMockHealthRepository()
	realmRepo := newMockRealmRepository()

	// Events without a username in Details (e.g. REFRESH_TOKEN, CODE_TO_TOKEN).
	// The old code would call GetUserByID to enrich these; the new code does not,
	// because that call triggers UNKNOWN 4 WARNs in federated Keycloak realms.
	client.GetRecentEventsFunc = func(ctx context.Context, realmName string) ([]*keycloakadmin.EventRepresentation, error) {
		return []*keycloakadmin.EventRepresentation{
			{
				Time:    time.Now().UnixMilli(),
				Type:    "REFRESH_TOKEN",
				RealmID: "realm-1",
				UserID:  "user-1",
				Details: map[string]string{"event_id": "evt-1"}, // no username key
			},
			{
				Time:    time.Now().UnixMilli(),
				Type:    "CODE_TO_TOKEN",
				RealmID: "realm-1",
				UserID:  "user-1",
				Details: map[string]string{"event_id": "evt-2"}, // no username key, same user
			},
			{
				Time:    time.Now().UnixMilli(),
				Type:    "REFRESH_TOKEN",
				RealmID: "realm-1",
				UserID:  "user-2",
				Details: map[string]string{"event_id": "evt-3"}, // no username key
			},
		}, nil
	}

	lookups := 0
	client.GetUserByIDFunc = func(ctx context.Context, realmName, userID string) (*keycloakadmin.UserRepresentation, error) {
		lookups++
		return &keycloakadmin.UserRepresentation{ID: userID, Username: "should-not-be-used"}, nil
	}

	monitor := NewMonitor(
		"tenant-1",
		"Test Tenant",
		client,
		cfg,
		log,
		eventRepo,
		generalEventRepo,
		metricsRepo,
		healthRepo,
		realmRepo,
		nil,
		nil,
	)

	monitor.collectEvents(context.Background())

	// GetUserByID must never be called — it triggers UNKNOWN 4 in federated realms.
	if lookups != 0 {
		t.Errorf("GetUserByID called %d times; want 0 (no Admin API user lookup)", lookups)
	}
	if c := client.getCalls("GetUserByID"); c != 0 {
		t.Errorf("GetUserByID tracked %d calls; want 0", c)
	}

	// All three events should be saved; username will be empty for events that
	// don't carry it in their Details payload — this is the accepted trade-off.
	savedEvents := eventRepo.getSavedEvents()
	if len(savedEvents) != 3 {
		t.Fatalf("SaveEvents called with %d events, want 3", len(savedEvents))
	}
	for _, e := range savedEvents {
		if e.Username != "" {
			t.Errorf("Event %s: expected empty username (no Details key), got %q", e.EventID, e.Username)
		}
		if e.UserID == "" {
			t.Errorf("Event %s: UserID must be preserved even without username", e.EventID)
		}
	}
}

func TestMonitor_collectEvents_userEnrichmentFromDetails(t *testing.T) {
	client := newMockAdminAPI()
	cfg := newTestConfig()
	log := newTestLogger()
	eventRepo := newMockEventRepository()
	generalEventRepo := newMockGeneralEventRepository()
	metricsRepo := newMockMetricsRepository()
	healthRepo := newMockHealthRepository()
	realmRepo := newMockRealmRepository()

	// Events carry the username (and sometimes email) in their details, as
	// Keycloak does for user events. Enrichment should use those directly and
	// NOT call the (expensive) Admin API user lookup.
	client.GetRecentEventsFunc = func(ctx context.Context, realmName string) ([]*keycloakadmin.EventRepresentation, error) {
		return []*keycloakadmin.EventRepresentation{
			{
				Time:    time.Now().UnixMilli(),
				Type:    "LOGIN",
				RealmID: "realm-1",
				UserID:  "user-1",
				Details: map[string]string{"event_id": "evt-1", "username": "alice", "email": "alice@example.com"},
			},
			{
				Time:    time.Now().UnixMilli(),
				Type:    "LOGIN_ERROR",
				RealmID: "realm-1",
				UserID:  "user-2",
				// username present, no email -> username used, email stays empty (no lookup)
				Details: map[string]string{"event_id": "evt-2", "username": "bob"},
			},
		}, nil
	}

	lookups := 0
	client.GetUserByIDFunc = func(ctx context.Context, realmName, userID string) (*keycloakadmin.UserRepresentation, error) {
		lookups++
		return &keycloakadmin.UserRepresentation{ID: userID, Username: "should-not-be-used", Email: "nope@example.com"}, nil
	}

	monitor := NewMonitor(
		"tenant-1", "Test Tenant", client, cfg, log,
		eventRepo, generalEventRepo, metricsRepo, healthRepo, realmRepo, nil, nil,
	)

	monitor.collectEvents(context.Background())

	if lookups != 0 {
		t.Errorf("GetUserByID called %d times; want 0 (username present in event details)", lookups)
	}
	if c := client.getCalls("GetUserByID"); c != 0 {
		t.Errorf("GetUserByID tracked %d calls; want 0", c)
	}

	saved := eventRepo.getSavedEvents()
	if len(saved) != 2 {
		t.Fatalf("SaveEvents called with %d events, want 2", len(saved))
	}
	for _, e := range saved {
		switch e.EventID {
		case "evt-1":
			if e.Username != "alice" || e.Email != "alice@example.com" {
				t.Errorf("evt-1 = username=%q email=%q, want alice / alice@example.com", e.Username, e.Email)
			}
		case "evt-2":
			if e.Username != "bob" || e.Email != "" {
				t.Errorf("evt-2 = username=%q email=%q, want bob / <empty>", e.Username, e.Email)
			}
		}
	}
}

func TestMonitor_collectEvents_withoutEventAlertConverter(t *testing.T) {
	client := newMockAdminAPI()
	cfg := newTestConfig()
	log := newTestLogger()
	eventRepo := newMockEventRepository()
	generalEventRepo := newMockGeneralEventRepository()
	metricsRepo := newMockMetricsRepository()
	healthRepo := newMockHealthRepository()
	realmRepo := newMockRealmRepository()

	// Create monitor without event alert converter (nil)
	monitor := NewMonitor(
		"tenant-1",
		"Test Tenant",
		client,
		cfg,
		log,
		eventRepo,
		generalEventRepo,
		metricsRepo,
		healthRepo,
		realmRepo,
		nil, // No event alert converter
		nil,
	)

	ctx := context.Background()
	// Should not panic when eventAlertConverter is nil
	monitor.collectEvents(ctx)

	// Events should still be saved
	savedEvents := eventRepo.getSavedEvents()
	if len(savedEvents) == 0 {
		t.Error("Events should be saved even without event alert converter")
	}
}

func TestMonitor_collectHealth_withoutHealthAlertManager(t *testing.T) {
	client := newMockAdminAPI()
	cfg := newTestConfig()
	log := newTestLogger()
	eventRepo := newMockEventRepository()
	generalEventRepo := newMockGeneralEventRepository()
	metricsRepo := newMockMetricsRepository()
	healthRepo := newMockHealthRepository()
	realmRepo := newMockRealmRepository()

	// Create monitor without health alert manager (nil)
	monitor := NewMonitor(
		"tenant-1",
		"Test Tenant",
		client,
		cfg,
		log,
		eventRepo,
		generalEventRepo,
		metricsRepo,
		healthRepo,
		realmRepo,
		nil,
		nil, // No health alert manager
	)

	ctx := context.Background()
	// Should not panic when healthAlertManager is nil
	monitor.collectHealth(ctx)

	// Health should still be saved
	savedHealth := healthRepo.getSavedHealth()
	if len(savedHealth) == 0 {
		t.Error("Health should be saved even without health alert manager")
	}
}

func TestMonitor_collectHealth_repoError(t *testing.T) {
	client := newMockAdminAPI()
	cfg := newTestConfig()
	log := newTestLogger()
	eventRepo := newMockEventRepository()
	generalEventRepo := newMockGeneralEventRepository()
	metricsRepo := newMockMetricsRepository()
	healthRepo := newMockHealthRepository()
	realmRepo := newMockRealmRepository()
	healthAlertManager := newMockHealthAlertManager()

	// Setup mock to return error on save
	healthRepo.SaveHealthFunc = func(ctx context.Context, health *domain.KeycloakHealth) error {
		return errors.New("database error")
	}

	monitor := NewMonitor(
		"tenant-1",
		"Test Tenant",
		client,
		cfg,
		log,
		eventRepo,
		generalEventRepo,
		metricsRepo,
		healthRepo,
		realmRepo,
		nil,
		healthAlertManager,
	)

	ctx := context.Background()
	// Should not panic, just log error
	monitor.collectHealth(ctx)

	// Health alert manager should NOT be called if save failed
	processedStatuses := healthAlertManager.getProcessedStatuses()
	if len(processedStatuses) != 0 {
		t.Errorf("ProcessHealthStatus called %d times, want 0 after save error", len(processedStatuses))
	}
}

// mockTenantHealthUpdater records the last health/error update it received.
type mockTenantHealthUpdater struct {
	lastStatus  string
	lastMessage string
	lastError   string
	healthCalls int
	errorCalls  int
}

func (m *mockTenantHealthUpdater) UpdateHealth(_ context.Context, _, status, message string) error {
	m.healthCalls++
	m.lastStatus = status
	m.lastMessage = message
	return nil
}

func (m *mockTenantHealthUpdater) UpdateError(_ context.Context, _, errorMsg string) error {
	m.errorCalls++
	m.lastError = errorMsg
	return nil
}

func TestCollectHealth_UpdatesTenantHealthStatus(t *testing.T) {
	cfg := newTestConfig()
	log := newTestLogger()

	t.Run("DOWN maps to unhealthy and records error", func(t *testing.T) {
		client := newMockAdminAPI()
		client.GetHealthMetricsFunc = func(ctx context.Context) *keycloakadmin.HealthMetrics {
			return &keycloakadmin.HealthMetrics{Status: "DOWN", Error: "connection refused"}
		}
		updater := &mockTenantHealthUpdater{}

		monitor := NewMonitor(
			"tenant-1", "Test Tenant", client, cfg, log,
			newMockEventRepository(), newMockGeneralEventRepository(),
			newMockMetricsRepository(), newMockHealthRepository(), newMockRealmRepository(),
			newMockEventAlertConverter(), newMockHealthAlertManager(),
			WithTenantHealthUpdater(updater),
		)

		monitor.collectHealth(context.Background())

		if updater.lastStatus != "unhealthy" {
			t.Errorf("health_status = %q, want unhealthy", updater.lastStatus)
		}
		if updater.errorCalls == 0 || updater.lastError != "connection refused" {
			t.Errorf("expected UpdateError with %q, got calls=%d value=%q", "connection refused", updater.errorCalls, updater.lastError)
		}
	})

	t.Run("UP maps to healthy", func(t *testing.T) {
		client := newMockAdminAPI()
		client.GetHealthMetricsFunc = func(ctx context.Context) *keycloakadmin.HealthMetrics {
			return &keycloakadmin.HealthMetrics{Status: "UP", ServerVersion: "26.0"}
		}
		updater := &mockTenantHealthUpdater{}

		monitor := NewMonitor(
			"tenant-1", "Test Tenant", client, cfg, log,
			newMockEventRepository(), newMockGeneralEventRepository(),
			newMockMetricsRepository(), newMockHealthRepository(), newMockRealmRepository(),
			newMockEventAlertConverter(), newMockHealthAlertManager(),
			WithTenantHealthUpdater(updater),
		)

		monitor.collectHealth(context.Background())

		if updater.lastStatus != "healthy" {
			t.Errorf("health_status = %q, want healthy", updater.lastStatus)
		}
		if updater.errorCalls == 0 {
			t.Errorf("expected UpdateError to be called to clear the error on UP")
		}
		if updater.lastError != "" {
			t.Errorf("expected last_error cleared (empty) on UP, got %q", updater.lastError)
		}
	})
}

// Cancelling the run context must let Stop drain promptly, even when a
// collector is blocked on a slow Keycloak call.
//
// This is the sequence fx uses: OnStop cancels the run context, then calls
// Stop. It matters here because pollEvents runs collectEvents once before
// entering its select loop, so Stop waits on that first collection no matter
// what the ticker is doing. The admin client builds its requests with
// http.NewRequestWithContext, so a cancelled context aborts the in-flight
// call; this test is what keeps that true. With one monitor per tenant and fx
// running OnStop hooks sequentially, a stalled drain here would blow the 15s
// fx shutdown timeout.
func TestMonitor_StopDrainsPromptlyWhenContextCancelled(t *testing.T) {
	client := newMockAdminAPI()

	var once sync.Once
	entered := make(chan struct{})
	// Stand in for a slow Keycloak round trip that honours cancellation, the
	// way http.NewRequestWithContext does. The 30s arm is far longer than
	// Stop's 10s drain, so this cannot pass by simply waiting the call out.
	client.GetRecentEventsFunc = func(ctx context.Context, realmName string) ([]*keycloakadmin.EventRepresentation, error) {
		once.Do(func() { close(entered) })
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(30 * time.Second):
			return nil, nil
		}
	}

	monitor := NewMonitor(
		"tenant-1",
		"Test Tenant",
		client,
		newTestConfig(),
		newTestLogger(),
		newMockEventRepository(),
		newMockGeneralEventRepository(),
		newMockMetricsRepository(),
		newMockHealthRepository(),
		newMockRealmRepository(),
		newMockEventAlertConverter(),
		newMockHealthAlertManager(),
	)

	ctx, cancel := context.WithCancel(context.Background())
	if err := monitor.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Wait until a collector is genuinely blocked in the client before
	// shutting down, rather than sleeping and hoping.
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		cancel()
		t.Fatal("GetRecentEvents was never called; the collector never started")
	}

	start := time.Now()
	cancel()
	if err := monitor.Stop(); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("Stop() took %s after cancellation, want a prompt drain: "+
			"an in-flight collector is not honouring ctx", elapsed)
	}
}

// Stop must not return while a polling worker is still in flight.
//
// The other two lifecycle tests here assert that Stop is prompt, which an
// untracked worker also satisfies: dropping the tracking makes Stop faster,
// not slower, so neither test could tell the difference. This one gates a
// collector open and asserts Stop stays blocked until it finishes, which is
// what actually pins the drain.
func TestMonitor_StopWaitsForWorkers(t *testing.T) {
	client := newMockAdminAPI()

	var once sync.Once
	entered := make(chan struct{})
	release := make(chan struct{})
	client.GetRecentEventsFunc = func(ctx context.Context, realmName string) ([]*keycloakadmin.EventRepresentation, error) {
		once.Do(func() { close(entered) })
		<-release
		return nil, nil
	}

	monitor := NewMonitor(
		"tenant-1",
		"Test Tenant",
		client,
		newTestConfig(),
		newTestLogger(),
		newMockEventRepository(),
		newMockGeneralEventRepository(),
		newMockMetricsRepository(),
		newMockHealthRepository(),
		newMockRealmRepository(),
		newMockEventAlertConverter(),
		newMockHealthAlertManager(),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := monitor.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		close(release)
		t.Fatal("the events collector never ran")
	}

	stopped := make(chan struct{})
	go func() {
		_ = monitor.Stop()
		close(stopped)
	}()

	// Stop must still be blocked, because the collector has not been released.
	select {
	case <-stopped:
		close(release)
		t.Fatal("Stop() returned while a polling worker was still in flight")
	case <-time.After(100 * time.Millisecond):
	}

	close(release)

	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("Stop() did not return after the worker finished")
	}
}
