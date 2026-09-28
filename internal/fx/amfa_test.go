package fx

import (
	"testing"

	"github.com/DefensePoint/keycloak-monitoring/internal/config"
)

// provideAmfaEnrichment tests
//
// These tests exercise the wiring decision (return nil vs return a real
// *amfa.Enrichment) without touching a live Keycloak or monitor pool. The
// nil-pool path is the operationally important one: it documents that the
// caller (amfa.Service) handles a nil enrichment by falling back to raw
// UUIDs, so deployments without a monitor pool still serve AMFA events.

func TestProvideAmfaEnrichment_ReturnsNilWhenPoolMissing(t *testing.T) {
	t.Parallel()

	// nil pool simulates a deployment where the MonitoringModule isn't
	// installed (e.g., a unit-test app graph). Must NOT return a real
	// Enrichment, because the underlying lookup would deref a nil pool.
	enr := provideAmfaEnrichment(nil, nil /* nil logger tolerated */)
	if enr != nil {
		t.Fatalf("expected nil Enrichment when pool is nil, got %+v", enr)
	}
}

func TestMonitorPoolKeycloakAdminLookup_NilPoolReturnsNil(t *testing.T) {
	t.Parallel()

	// Defensive: even if someone manually constructs the lookup with a nil
	// pool (it shouldn't happen in real wiring, but the type permits it),
	// resolution must return nil rather than panic on the pool deref.
	lookup := &monitorPoolKeycloakAdminLookup{pool: nil}
	if got := lookup.GetKeycloakAdmin("any-tenant"); got != nil {
		t.Errorf("expected nil admin for nil pool, got %+v", got)
	}
}

// buildAmfaRegistry is the focal point of these tests because it isolates
// the per-tenant iteration / validation / skip logic from the parts of
// provideAmfaRegistry that touch fx (cfg unmarshal) and a real Postgres.
// Each test asserts an empty registry: any non-empty result here would
// mean we accidentally opened a live database connection.

func TestBuildAmfaRegistry_EmptyWhenNoTenants(t *testing.T) {
	t.Parallel()

	reg := buildAmfaRegistry(&config.AppConfig{}, nil, nil /* nil logger tolerated */)
	if reg == nil {
		t.Fatal("buildAmfaRegistry returned nil; want empty *amfa.Registry")
	}
	if ids := reg.TenantIDs(); len(ids) != 0 {
		t.Errorf("want zero tenants, got %v", ids)
	}
}

func TestBuildAmfaRegistry_EmptyWhenNoTenantsHaveAmfa(t *testing.T) {
	t.Parallel()

	tenants := map[string]config.TenantConfig{
		"tenant-without-amfa": {
			Name:    "Tenant Without AMFA",
			Enabled: true,
			// No Amfa block set -> pointer is nil -> tenant should be skipped.
		},
	}

	reg := buildAmfaRegistry(&config.AppConfig{}, tenants, nil)
	if reg == nil {
		t.Fatal("buildAmfaRegistry returned nil; want empty *amfa.Registry")
	}
	if ids := reg.TenantIDs(); len(ids) != 0 {
		t.Errorf("want zero tenants, got %v", ids)
	}
}

func TestBuildAmfaRegistry_SkipsTenantWithDisabledAmfa(t *testing.T) {
	t.Parallel()

	tenants := map[string]config.TenantConfig{
		"tenant-with-disabled-amfa": {
			Name: "Disabled AMFA",
			Amfa: &config.AmfaTenantConfig{
				Enabled: false,
				Database: config.AmfaDatabaseConfig{
					// Fully populated, but Enabled=false means we never look.
					Host:     "amfa.example.com",
					Port:     5432,
					Database: "amfa",
					User:     "ro",
				},
			},
		},
	}

	reg := buildAmfaRegistry(&config.AppConfig{}, tenants, nil)
	if ids := reg.TenantIDs(); len(ids) != 0 {
		t.Errorf("want zero tenants for disabled AMFA, got %v", ids)
	}
}

func TestBuildAmfaRegistry_SkipsTenantWithInvalidAmfaConfig(t *testing.T) {
	t.Parallel()

	// Enabled but missing required Database.Host. Validate() must reject
	// this BEFORE NewClient is ever called, so the test runs without a
	// live Postgres and proves the early-skip path.
	tenants := map[string]config.TenantConfig{
		"tenant-with-bad-amfa": {
			Name: "Bad AMFA Config",
			Amfa: &config.AmfaTenantConfig{
				Enabled: true,
				Database: config.AmfaDatabaseConfig{
					// Host intentionally empty -> Validate() returns error.
					Database: "amfa",
					User:     "ro",
				},
			},
		},
	}

	// The call must not panic and must return an empty registry.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("buildAmfaRegistry panicked on invalid AMFA config: %v", r)
		}
	}()
	reg := buildAmfaRegistry(&config.AppConfig{}, tenants, nil)
	if reg == nil {
		t.Fatal("buildAmfaRegistry returned nil; want empty *amfa.Registry")
	}
	if ids := reg.TenantIDs(); len(ids) != 0 {
		t.Errorf("invalid-config tenant should be skipped; got %v", ids)
	}
}

func TestBuildAmfaRegistry_MultipleTenantsMixed(t *testing.T) {
	t.Parallel()

	// Combine "no amfa", "disabled amfa", and "invalid amfa" tenants. All
	// three skip paths must coexist without affecting each other and
	// without touching a real database.
	tenants := map[string]config.TenantConfig{
		"a-no-amfa": {Name: "A"},
		"b-disabled": {
			Name: "B",
			Amfa: &config.AmfaTenantConfig{Enabled: false},
		},
		"c-invalid": {
			Name: "C",
			Amfa: &config.AmfaTenantConfig{
				Enabled: true,
				// Missing Host -> Validate() errors -> skip.
				Database: config.AmfaDatabaseConfig{
					Database: "amfa",
					User:     "ro",
				},
			},
		},
	}

	reg := buildAmfaRegistry(&config.AppConfig{}, tenants, nil)
	if ids := reg.TenantIDs(); len(ids) != 0 {
		t.Errorf("expected no tenants registered, got %v", ids)
	}
}

// --- transport selection --------------------------------------------------
//
// These exercise newAmfaRepository rather than buildAmfaRegistry so the chosen
// transport is observable. The API path is the interesting one: unlike the
// database path it constructs without any network access, so it can be asserted
// on without a live AMFA.

func amfaAPITenant(baseURL string) config.TenantConfig {
	return config.TenantConfig{
		Name:         "API tenant",
		ServerURL:    "https://keycloak.example.com",
		AdminRealm:   "master",
		ClientID:     "monitoring-service",
		ClientSecret: "secret",
		Enabled:      true,
		Amfa: &config.AmfaTenantConfig{
			Enabled: true,
			API:     config.AmfaAPIConfig{BaseURL: baseURL},
		},
	}
}

func TestNewAmfaRepository_PrefersAPIWhenConfigured(t *testing.T) {
	// Both transports present: the API must win. It needs no database
	// credentials and does not couple this platform to AMFA's schema.
	tcfg := amfaAPITenant("https://amfa.internal")
	tcfg.Amfa.Database = config.AmfaDatabaseConfig{
		Host: "amfa-db", Database: "adaptive_mfa", User: "kmt_ro",
	}
	if err := tcfg.Amfa.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	repo, transport, err := newAmfaRepository("t1", tcfg, &config.AppConfig{}, nil)
	if err != nil {
		t.Fatalf("newAmfaRepository: %v", err)
	}
	if transport != "api" {
		t.Errorf("transport = %q, want api to take precedence", transport)
	}
	if repo == nil {
		t.Error("repository is nil")
	}
}

func TestNewAmfaRepository_APIPathNeedsNoNetworkAtStartup(t *testing.T) {
	// Construction must not probe AMFA. Failing startup on a momentarily
	// unreachable AMFA would take the tenant's endpoints down for the whole
	// outage; request-time failures surface as 503 instead.
	tcfg := amfaAPITenant("https://amfa-that-does-not-resolve.invalid")
	if err := tcfg.Amfa.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	repo, transport, err := newAmfaRepository("t1", tcfg, &config.AppConfig{}, nil)
	if err != nil {
		t.Fatalf("construction should not require reachability: %v", err)
	}
	if transport != "api" || repo == nil {
		t.Errorf("transport = %q, repo nil = %v", transport, repo == nil)
	}
}

func TestNewAmfaRepository_RegistersTenantOverAPI(t *testing.T) {
	// The end-to-end path through buildAmfaRegistry: an API-only tenant reaches
	// the registry, which is what makes its endpoints answer.
	tenants := map[string]config.TenantConfig{
		"api-tenant": amfaAPITenant("https://amfa.internal"),
	}

	reg := buildAmfaRegistry(&config.AppConfig{}, tenants, nil)
	if got := reg.TenantIDs(); len(got) != 1 || got[0] != "api-tenant" {
		t.Errorf("registered tenants = %v, want [api-tenant]", got)
	}
	if _, err := reg.RepositoryFor("api-tenant"); err != nil {
		t.Errorf("RepositoryFor: %v", err)
	}
}

func TestBuildAmfaRegistry_SkipsTenantWithNeitherTransport(t *testing.T) {
	// Enabled but unconfigured is a config error, and it must skip the tenant
	// rather than abort startup for every other tenant.
	tenants := map[string]config.TenantConfig{
		"broken": {
			Name:    "Broken",
			Enabled: true,
			Amfa:    &config.AmfaTenantConfig{Enabled: true},
		},
	}

	reg := buildAmfaRegistry(&config.AppConfig{}, tenants, nil)
	if got := reg.TenantIDs(); len(got) != 0 {
		t.Errorf("registered %v, want none", got)
	}
}

func TestAmfaAuthRealm_DefaultsToMaster(t *testing.T) {
	// The token comes from the realm holding the confidential client. An empty
	// admin_realm would otherwise build a token URL with an empty path segment
	// and fail at every request.
	if got := amfaAuthRealm(config.TenantConfig{}); got != "master" {
		t.Errorf("amfaAuthRealm = %q, want master", got)
	}
	if got := amfaAuthRealm(config.TenantConfig{AdminRealm: "ops"}); got != "ops" {
		t.Errorf("amfaAuthRealm = %q, want ops", got)
	}
}
