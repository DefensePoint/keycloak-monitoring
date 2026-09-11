package keycloak

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/pkg/keycloakadmin"
)

// Sentinel errors for service layer
var (
	// ErrClientNotAvailable indicates that the Keycloak client is not available for the tenant
	ErrClientNotAvailable = errors.New("keycloak client not available for this tenant")
	// ErrClientProviderNotConfigured indicates that the client provider is not configured
	ErrClientProviderNotConfigured = errors.New("keycloak client provider not configured")
)

// Service defines the business logic for Keycloak operations.
type Service interface {
	// Events
	ListEvents(ctx context.Context, tenantID string, limit, offset int) ([]*domain.KeycloakEvent, error)
	ListEventsByRealm(ctx context.Context, tenantID, realmName string, from, to time.Time, limit, offset int) ([]*domain.KeycloakEvent, error)
	ListEventsByType(ctx context.Context, tenantID, realmName, eventType string, from, to time.Time, limit, offset int) ([]*domain.KeycloakEvent, error)
	ListEventsByTimeRange(ctx context.Context, tenantID string, from, to time.Time, limit, offset int) ([]*domain.KeycloakEvent, error)
	CountEvents(ctx context.Context, tenantID string) (int64, error)
	CountEventsByType(ctx context.Context, tenantID, realmName, eventType string, from, to time.Time) (int64, error)
	GetEventStats(ctx context.Context, tenantID, realmName string, from, to time.Time) (*EventStats, error)

	// Metrics
	GetLatestMetrics(ctx context.Context, tenantID string) (*domain.KeycloakMetrics, error)
	GetLatestMetricsByRealm(ctx context.Context, tenantID, realmName string) (*domain.KeycloakMetrics, error)
	GetMetricsHistory(ctx context.Context, tenantID string, from, to time.Time) ([]*domain.KeycloakMetrics, error)
	GetMetricsHistoryByRealm(ctx context.Context, tenantID, realmName string, from, to time.Time) ([]*domain.KeycloakMetrics, error)
	GetAllRealmMetrics(ctx context.Context, tenantID string) (map[string]*domain.KeycloakMetrics, error)

	// Health
	GetLatestHealth(ctx context.Context, tenantID string) (*domain.KeycloakHealth, error)
	GetHealthHistory(ctx context.Context, tenantID string, from, to time.Time) ([]*domain.KeycloakHealth, error)

	// Realms
	ListRealms(ctx context.Context, tenantID string) ([]*domain.KeycloakRealmInfo, error)
	GetRealm(ctx context.Context, tenantID, realmName string) (*domain.KeycloakRealmInfo, error)

	// ListAmfaEnabledRealms returns the names of realms whose bound browser login
	// flow uses an Adaptive MFA (AMFA) authenticator. Result is cached
	// per tenant with a short TTL. Requires a live Keycloak client.
	ListAmfaEnabledRealms(ctx context.Context, tenantID string) ([]string, error)

	// Dashboard
	GetDashboard(ctx context.Context, tenantID string, realmName string) (*Dashboard, error)

	// Version
	GetVersionInfo(ctx context.Context, tenantID string) (*VersionCheckResult, error)

	// Live Keycloak data (requires client)
	GetUsers(ctx context.Context, tenantID, realmName string, first, max int) ([]keycloakadmin.UserRepresentation, error)
	GetClients(ctx context.Context, tenantID, realmName string) ([]*keycloakadmin.ClientRepresentation, error)
	GetUserDetails(ctx context.Context, tenantID, realmName, userID string) (*keycloakadmin.UserDetails, error)
	GetInfinispanMetrics(ctx context.Context, tenantID string) (*keycloakadmin.InfinispanMetrics, error)

	// Late binding setters (for DI when MonitorPoolManager is available)
	SetClientProvider(provider ClientProvider)
	SetVersionChecker(checker *VersionChecker)
}

// ClientProvider provides Keycloak clients for specific tenants.
type ClientProvider interface {
	GetClient(tenantID string) AdminAPI
}

// Provider IDs of the Adaptive MFA (AMFA) authenticators, as registered by the
// Adaptive MFA Keycloak SPI in AdaptiveAuthAuthenticatorFactory.PROVIDER_ID
// and ConditionalUserConfiguredAdaptiveAuthAuthenticatorFactory.PROVIDER_ID. A
// realm "uses AMFA" when any of its top-level flows contains one of these
// executions and it is not DISABLED.
//
// Note "any top-level flow", not just the bound browser flow, which is what
// realmUsesAdaptiveAuth actually walks. That is deliberately the permissive
// reading: AMFA can be bound to bindings other than browser, and a realm with
// the flow configured but not yet bound is better shown with its AMFA
// sections than silently treated as a plain realm. False negatives here are
// the costlier failure, since the UI fail-closes on this endpoint and gives
// no indication that a realm was misjudged.
var amfaAuthenticatorProviderIDs = map[string]struct{}{
	"adaptive-auth":             {},
	"conditional-adaptive-auth": {},
}

// amfaRealmsCacheTTL bounds how often we walk each realm's Keycloak flow.
const amfaRealmsCacheTTL = 5 * time.Minute

type amfaRealmsCacheEntry struct {
	realms    []string
	expiresAt time.Time
}

// service implements the Service interface.
type service struct {
	eventRepo      EventRepository
	metricsRepo    MetricsRepository
	healthRepo     HealthRepository
	realmRepo      RealmRepository
	clientProvider ClientProvider
	versionChecker *VersionChecker

	amfaRealmsMu    sync.Mutex
	amfaRealmsCache map[string]amfaRealmsCacheEntry
}

// NewService creates a new keycloak service.
func NewService(
	eventRepo EventRepository,
	metricsRepo MetricsRepository,
	healthRepo HealthRepository,
	realmRepo RealmRepository,
	clientProvider ClientProvider,
	versionChecker *VersionChecker,
) Service {
	return &service{
		eventRepo:       eventRepo,
		metricsRepo:     metricsRepo,
		healthRepo:      healthRepo,
		realmRepo:       realmRepo,
		clientProvider:  clientProvider,
		versionChecker:  versionChecker,
		amfaRealmsCache: make(map[string]amfaRealmsCacheEntry),
	}
}

// SetClientProvider sets the client provider for live Keycloak access.
// This allows late binding when the MonitorPoolManager is available.
func (s *service) SetClientProvider(provider ClientProvider) {
	s.clientProvider = provider
}

// SetVersionChecker sets the version checker.
func (s *service) SetVersionChecker(checker *VersionChecker) {
	s.versionChecker = checker
}

// ListEvents returns events for a tenant.
func (s *service) ListEvents(ctx context.Context, tenantID string, limit, offset int) ([]*domain.KeycloakEvent, error) {
	return s.eventRepo.GetEvents(ctx, tenantID, limit, offset)
}

// ListEventsByRealm returns events for a specific realm.
func (s *service) ListEventsByRealm(ctx context.Context, tenantID, realmName string, from, to time.Time, limit, offset int) ([]*domain.KeycloakEvent, error) {
	return s.eventRepo.GetEventsByRealm(ctx, tenantID, realmName, from, to, limit, offset)
}

// ListEventsByType returns events of a specific type within one realm.
func (s *service) ListEventsByType(ctx context.Context, tenantID, realmName, eventType string, from, to time.Time, limit, offset int) ([]*domain.KeycloakEvent, error) {
	return s.eventRepo.GetEventsByType(ctx, tenantID, realmName, eventType, from, to, limit, offset)
}

// ListEventsByTimeRange returns events within a time range.
func (s *service) ListEventsByTimeRange(ctx context.Context, tenantID string, from, to time.Time, limit, offset int) ([]*domain.KeycloakEvent, error) {
	return s.eventRepo.GetEventsByTimeRange(ctx, tenantID, from, to, limit, offset)
}

// CountEvents returns the total count of events for a tenant.
func (s *service) CountEvents(ctx context.Context, tenantID string) (int64, error) {
	return s.eventRepo.CountEvents(ctx, tenantID)
}

// CountEventsByType returns the count of events by type for a realm.
func (s *service) CountEventsByType(ctx context.Context, tenantID, realmName, eventType string, from, to time.Time) (int64, error) {
	return s.eventRepo.CountEventsByType(ctx, tenantID, realmName, eventType, from, to)
}

// GetEventStats returns aggregated event statistics for a realm.
func (s *service) GetEventStats(ctx context.Context, tenantID, realmName string, from, to time.Time) (*EventStats, error) {
	loginCount, _ := s.eventRepo.CountEventsByType(ctx, tenantID, realmName, "LOGIN", from, to)
	loginErrorCount, _ := s.eventRepo.CountEventsByType(ctx, tenantID, realmName, "LOGIN_ERROR", from, to)
	logoutCount, _ := s.eventRepo.CountEventsByType(ctx, tenantID, realmName, "LOGOUT", from, to)
	registerCount, _ := s.eventRepo.CountEventsByType(ctx, tenantID, realmName, "REGISTER", from, to)
	codeToTokenCount, _ := s.eventRepo.CountEventsByType(ctx, tenantID, realmName, "CODE_TO_TOKEN", from, to)

	return &EventStats{
		Realm:            realmName,
		Start:            from,
		End:              to,
		LoginCount:       loginCount,
		LoginErrorCount:  loginErrorCount,
		LogoutCount:      logoutCount,
		RegisterCount:    registerCount,
		CodeToTokenCount: codeToTokenCount,
		TotalEvents:      loginCount + loginErrorCount + logoutCount + registerCount + codeToTokenCount,
	}, nil
}

// GetLatestMetrics returns the latest metrics for a tenant.
func (s *service) GetLatestMetrics(ctx context.Context, tenantID string) (*domain.KeycloakMetrics, error) {
	return s.metricsRepo.GetLatestMetrics(ctx, tenantID)
}

// GetLatestMetricsByRealm returns the latest metrics for a specific realm.
func (s *service) GetLatestMetricsByRealm(ctx context.Context, tenantID, realmName string) (*domain.KeycloakMetrics, error) {
	return s.metricsRepo.GetLatestMetricsByRealm(ctx, tenantID, realmName)
}

// GetMetricsHistory returns metrics history for a tenant.
func (s *service) GetMetricsHistory(ctx context.Context, tenantID string, from, to time.Time) ([]*domain.KeycloakMetrics, error) {
	return s.metricsRepo.GetMetricsHistory(ctx, tenantID, from, to)
}

// GetMetricsHistoryByRealm returns metrics history for a specific realm.
func (s *service) GetMetricsHistoryByRealm(ctx context.Context, tenantID, realmName string, from, to time.Time) ([]*domain.KeycloakMetrics, error) {
	return s.metricsRepo.GetMetricsHistoryByRealm(ctx, tenantID, realmName, from, to)
}

// GetAllRealmMetrics returns metrics for all realms of a tenant.
func (s *service) GetAllRealmMetrics(ctx context.Context, tenantID string) (map[string]*domain.KeycloakMetrics, error) {
	realms, err := s.realmRepo.GetRealms(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	result := make(map[string]*domain.KeycloakMetrics)
	for _, realm := range realms {
		metrics, err := s.metricsRepo.GetLatestMetricsByRealm(ctx, tenantID, realm.RealmName)
		if err == nil && metrics != nil {
			result[realm.RealmName] = metrics
		}
	}

	return result, nil
}

// GetLatestHealth returns the latest health status for a tenant.
func (s *service) GetLatestHealth(ctx context.Context, tenantID string) (*domain.KeycloakHealth, error) {
	return s.healthRepo.GetLatestHealth(ctx, tenantID)
}

// GetHealthHistory returns health status history for a tenant.
func (s *service) GetHealthHistory(ctx context.Context, tenantID string, from, to time.Time) ([]*domain.KeycloakHealth, error) {
	return s.healthRepo.GetHealthHistory(ctx, tenantID, from, to)
}

// ListRealms returns all realms for a tenant.
func (s *service) ListRealms(ctx context.Context, tenantID string) ([]*domain.KeycloakRealmInfo, error) {
	return s.realmRepo.GetRealms(ctx, tenantID)
}

// GetRealm returns a specific realm by name.
func (s *service) GetRealm(ctx context.Context, tenantID, realmName string) (*domain.KeycloakRealmInfo, error) {
	return s.realmRepo.GetRealmByName(ctx, tenantID, realmName)
}

// ListAmfaEnabledRealms returns the names of realms whose bound browser login
// flow uses an Adaptive MFA (AMFA) authenticator. Results are cached per
// tenant for amfaRealmsCacheTTL; on a Keycloak error a still-usable stale cache
// entry is returned in preference to failing.
func (s *service) ListAmfaEnabledRealms(ctx context.Context, tenantID string) ([]string, error) {
	if cached, ok := s.cachedAmfaRealms(tenantID, false); ok {
		return cached, nil
	}

	if s.clientProvider == nil {
		return nil, ErrClientProviderNotConfigured
	}
	client := s.clientProvider.GetClient(tenantID)
	if client == nil {
		return nil, ErrClientNotAvailable
	}

	realms, err := client.GetAllRealms(ctx)
	if err != nil {
		if stale, ok := s.cachedAmfaRealms(tenantID, true); ok {
			return stale, nil
		}
		return nil, err
	}

	enabled := make([]string, 0)
	for _, realm := range realms {
		if realm == nil || realm.Realm == "" {
			continue
		}
		if s.realmUsesAdaptiveAuth(ctx, client, realm.Realm) {
			enabled = append(enabled, realm.Realm)
		}
	}

	s.amfaRealmsMu.Lock()
	s.amfaRealmsCache[tenantID] = amfaRealmsCacheEntry{
		realms:    enabled,
		expiresAt: time.Now().Add(amfaRealmsCacheTTL),
	}
	s.amfaRealmsMu.Unlock()

	return enabled, nil
}

// cachedAmfaRealms returns the cached AMFA-enabled realms for a tenant. When
// allowStale is false only unexpired entries hit; when true any present entry
// (even expired) is returned, used as a fallback when Keycloak is unreachable.
func (s *service) cachedAmfaRealms(tenantID string, allowStale bool) ([]string, bool) {
	s.amfaRealmsMu.Lock()
	defer s.amfaRealmsMu.Unlock()
	entry, ok := s.amfaRealmsCache[tenantID]
	if !ok {
		return nil, false
	}
	if !allowStale && time.Now().After(entry.expiresAt) {
		return nil, false
	}
	return entry.realms, true
}

// realmUsesAdaptiveAuth reports whether any of a realm's top-level authentication
// flows contains an enabled Adaptive MFA authenticator. We scan every
// top-level flow (not just the realm's bound browserFlow) because AMFA is often
// bound at the client level via an authentication-flow override to a custom flow
// (e.g. "hybrid-flow") rather than as the realm-wide browser flow. A flow's
// executions listing already includes its nested subflows, so top-level flows
// are sufficient. Errors inspecting a realm are treated as "not AMFA".
func (s *service) realmUsesAdaptiveAuth(ctx context.Context, client AdminAPI, realmName string) bool {
	flows, err := client.GetAuthenticationFlows(ctx, realmName)
	if err != nil {
		return false
	}
	for _, flow := range flows {
		if !flow.TopLevel {
			continue
		}
		execs, err := client.GetFlowExecutions(ctx, realmName, flow.Alias)
		if err != nil {
			continue
		}
		if flowUsesAdaptiveAuth(execs) {
			return true
		}
	}
	return false
}

// flowUsesAdaptiveAuth reports whether a flow's executions include an enabled
// Adaptive MFA authenticator.
func flowUsesAdaptiveAuth(execs []keycloakadmin.AuthenticationExecutionInfoRepresentation) bool {
	for _, e := range execs {
		if e.Requirement == "DISABLED" {
			continue
		}
		if _, ok := amfaAuthenticatorProviderIDs[e.ProviderID]; ok {
			return true
		}
	}
	return false
}

// GetDashboard returns aggregated dashboard data for a tenant.
func (s *service) GetDashboard(ctx context.Context, tenantID string, realmName string) (*Dashboard, error) {
	dashboard := &Dashboard{}

	// Get health
	health, _ := s.healthRepo.GetLatestHealth(ctx, tenantID)
	dashboard.Health = health

	// Get version info if health has version
	if health != nil && health.ServerVersion != "" && s.versionChecker != nil {
		versionInfo, err := s.GetVersionInfo(ctx, tenantID)
		if err == nil {
			dashboard.VersionInfo = versionInfo
		}
	}

	// If realm specified, get detailed data for that realm
	if realmName != "" {
		dashboard.Realm = realmName

		metrics, _ := s.metricsRepo.GetLatestMetricsByRealm(ctx, tenantID, realmName)
		dashboard.Metrics = metrics

		realmInfo, _ := s.realmRepo.GetRealmByName(ctx, tenantID, realmName)
		dashboard.RealmInfo = realmInfo

		// Get recent event stats (last hour)
		end := time.Now()
		start := end.Add(-1 * time.Hour)
		loginCount, _ := s.eventRepo.CountEventsByType(ctx, tenantID, realmName, "LOGIN", start, end)
		loginErrorCount, _ := s.eventRepo.CountEventsByType(ctx, tenantID, realmName, "LOGIN_ERROR", start, end)

		dashboard.RecentEvents = &RecentEventStats{
			Logins:      loginCount,
			LoginErrors: loginErrorCount,
			TimePeriod:  "last_hour",
		}

		return dashboard, nil
	}

	// Get all realms summary
	realms, _ := s.realmRepo.GetRealms(ctx, tenantID)
	dashboard.TotalRealms = len(realms)

	realmsSummary := make([]*RealmSummary, 0, len(realms))
	for _, realm := range realms {
		metrics, _ := s.metricsRepo.GetLatestMetricsByRealm(ctx, tenantID, realm.RealmName)

		summary := &RealmSummary{
			RealmName:       realm.RealmName,
			Enabled:         realm.Enabled,
			IsHealthy:       realm.IsHealthy,
			EventsEnabled:   realm.EventsEnabled,
			EventsListeners: realm.EventsListeners,
			Metrics:         metrics,
		}
		realmsSummary = append(realmsSummary, summary)
	}
	dashboard.Realms = realmsSummary

	return dashboard, nil
}

// GetVersionInfo returns version check information.
func (s *service) GetVersionInfo(ctx context.Context, tenantID string) (*VersionCheckResult, error) {
	if s.versionChecker == nil {
		return nil, nil
	}

	// Get current version from health data
	health, err := s.healthRepo.GetLatestHealth(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	currentVersion := ""
	if health != nil {
		currentVersion = health.ServerVersion
	}

	// Fetch latest versions
	latestVersions, err := s.versionChecker.GetLatestVersions(ctx)
	if err != nil {
		return nil, err
	}

	isKeycloakOutdated := false
	if currentVersion != "" && latestVersions != nil && latestVersions.LatestKeycloakVersion != "" {
		isKeycloakOutdated = IsVersionOutdated(currentVersion, latestVersions.LatestKeycloakVersion)
	}

	return &VersionCheckResult{
		CurrentVersion:        currentVersion,
		LatestKeycloakVersion: latestVersions.LatestKeycloakVersion,
		KeycloakReleaseURL:    latestVersions.KeycloakReleaseURL,
		IsKeycloakOutdated:    isKeycloakOutdated,
		HasSecurityUpdates:    latestVersions.HasSecurityUpdates,
		LastChecked:           latestVersions.LastChecked,
	}, nil
}

// GetUsers returns users from a Keycloak realm.
func (s *service) GetUsers(ctx context.Context, tenantID, realmName string, first, max int) ([]keycloakadmin.UserRepresentation, error) {
	if s.clientProvider == nil {
		return nil, ErrClientProviderNotConfigured
	}

	client := s.clientProvider.GetClient(tenantID)
	if client == nil {
		return nil, ErrClientNotAvailable
	}

	return client.GetRealmUsers(ctx, realmName, first, max)
}

// GetClients returns clients from a Keycloak realm.
func (s *service) GetClients(ctx context.Context, tenantID, realmName string) ([]*keycloakadmin.ClientRepresentation, error) {
	if s.clientProvider == nil {
		return nil, ErrClientProviderNotConfigured
	}

	client := s.clientProvider.GetClient(tenantID)
	if client == nil {
		return nil, ErrClientNotAvailable
	}

	return client.GetClients(ctx, realmName)
}

// GetUserDetails returns complete user information including groups and roles.
func (s *service) GetUserDetails(ctx context.Context, tenantID, realmName, userID string) (*keycloakadmin.UserDetails, error) {
	if s.clientProvider == nil {
		return nil, ErrClientProviderNotConfigured
	}

	client := s.clientProvider.GetClient(tenantID)
	if client == nil {
		return nil, ErrClientNotAvailable
	}

	return client.GetUserDetails(ctx, realmName, userID)
}

// GetInfinispanMetrics returns Infinispan metrics from Keycloak.
func (s *service) GetInfinispanMetrics(ctx context.Context, tenantID string) (*keycloakadmin.InfinispanMetrics, error) {
	if s.clientProvider == nil {
		return nil, ErrClientProviderNotConfigured
	}

	client := s.clientProvider.GetClient(tenantID)
	if client == nil {
		return nil, ErrClientNotAvailable
	}

	return client.GetInfinispanMetrics(ctx)
}
