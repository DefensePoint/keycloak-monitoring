package fx

import (
	"testing"

	"github.com/DefensePoint/keycloak-monitoring/internal/config"
)

// A database from a single-tenant release is given to the one configured tenant, and to
// nobody when there are zero or several: the database layer then refuses it.
func TestPreTenantOwner(t *testing.T) {
	for _, tc := range []struct {
		tenants []string
		want    string
	}{
		{nil, ""},
		{[]string{"acme"}, "acme"},
		{[]string{"acme", "beta"}, ""},
	} {
		cfg := &config.AppConfig{}
		cfg.Keycloak.Tenants = map[string]config.TenantConfig{}
		for _, id := range tc.tenants {
			cfg.Keycloak.Tenants[id] = config.TenantConfig{}
		}
		if got := preTenantOwner(cfg); got != tc.want {
			t.Errorf("tenants %v: owner %q, want %q", tc.tenants, got, tc.want)
		}
	}
}
