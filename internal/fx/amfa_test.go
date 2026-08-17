package fx

import (
	"testing"

	"repos.defensepoint.com/iam/oss-pmt/kmt/internal/config"
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

	reg := buildAmfaRegistry(nil, nil /* nil logger tolerated */)
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

	reg := buildAmfaRegistry(tenants, nil)
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

	reg := buildAmfaRegistry(tenants, nil)
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
	reg := buildAmfaRegistry(tenants, nil)
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

	reg := buildAmfaRegistry(tenants, nil)
	if ids := reg.TenantIDs(); len(ids) != 0 {
		t.Errorf("expected no tenants registered, got %v", ids)
	}
}
