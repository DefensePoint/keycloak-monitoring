package rbac

import (
	"slices"
	"testing"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

// The one table defining what every shape of a tenant_policies row means.
//
// Two resolvers used to answer these questions independently, the web
// middleware and the MCP authorizer, and disagreed on three of them. Both now
// call ResolveRealmScope, so this table is the whole contract and a divergence
// has nowhere left to hide.
func TestResolveRealmScope(t *testing.T) {
	const tenant = "tenant-a"

	tests := []struct {
		name       string
		policies   []*domain.TenantPolicy
		wantRealms []string
		wantAll    bool
	}{
		{
			// tenant_policies is an opt-in restriction, not a grant.
			name:     "no policy row at all is unrestricted",
			policies: nil,
			wantAll:  true,
		},
		{
			name:     "a row for another tenant does not restrict this one",
			policies: []*domain.TenantPolicy{{TenantID: "tenant-other", AllowedRealms: []string{"realmB"}}},
			wantAll:  true,
		},
		{
			// The shape MCP used to deny while the web allowed it.
			name:     "allowed_realms holding no list is unrestricted",
			policies: []*domain.TenantPolicy{{TenantID: tenant, RealmsUnrestricted: true}},
			wantAll:  true,
		},
		{
			name:     "a stored empty list denies every realm",
			policies: []*domain.TenantPolicy{{TenantID: tenant, AllowedRealms: []string{}}},
			wantAll:  false,
		},
		{
			name:       "a named realm restricts to exactly it",
			policies:   []*domain.TenantPolicy{{TenantID: tenant, AllowedRealms: []string{"realmB"}}},
			wantRealms: []string{"realmB"},
		},
		{
			// The shape MCP used to answer with only the first row.
			name: "every matching row contributes, because (user_id, tenant_id) is not unique",
			policies: []*domain.TenantPolicy{
				{TenantID: tenant, AllowedRealms: []string{"realmA"}},
				{TenantID: tenant, AllowedRealms: []string{"realmB"}},
			},
			wantRealms: []string{"realmA", "realmB"},
		},
		{
			name: "duplicate realms across rows appear once",
			policies: []*domain.TenantPolicy{
				{TenantID: tenant, AllowedRealms: []string{"realmA", "realmB"}},
				{TenantID: tenant, AllowedRealms: []string{"realmB"}},
			},
			wantRealms: []string{"realmA", "realmB"},
		},
		{
			name: "an unrestricted row wins over a restricting one",
			policies: []*domain.TenantPolicy{
				{TenantID: tenant, AllowedRealms: []string{"realmA"}},
				{TenantID: tenant, RealmsUnrestricted: true},
			},
			wantAll: true,
		},
		{
			// The fail-open shape. Blank names no realm, but left in a scope it
			// reads as "no filter" downstream and widens to the whole tenant.
			name:     "a single blank realm denies every realm",
			policies: []*domain.TenantPolicy{{TenantID: tenant, AllowedRealms: []string{""}}},
			wantAll:  false,
		},
		{
			name:     "nothing but blank realms denies every realm",
			policies: []*domain.TenantPolicy{{TenantID: tenant, AllowedRealms: []string{"", "  ", "\t"}}},
			wantAll:  false,
		},
		{
			name:       "a blank beside a real realm keeps only the real one",
			policies:   []*domain.TenantPolicy{{TenantID: tenant, AllowedRealms: []string{"", "realmB"}}},
			wantRealms: []string{"realmB"},
		},
		{
			// Realms are matched by exact equality, so padding is kept rather
			// than trimmed into the realm it resembles.
			name:       "a padded realm is kept verbatim, not trimmed into a grant",
			policies:   []*domain.TenantPolicy{{TenantID: tenant, AllowedRealms: []string{" realmB "}}},
			wantRealms: []string{" realmB "},
		},
		{
			name:     "a nil row in the slice is skipped, not treated as unrestricted",
			policies: []*domain.TenantPolicy{nil, {TenantID: tenant, AllowedRealms: []string{"realmB"}}},

			wantRealms: []string{"realmB"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			realms, all := ResolveRealmScope(tt.policies, tenant)

			if all != tt.wantAll {
				t.Fatalf("all = %v, want %v (realms = %q)", all, tt.wantAll, realms)
			}
			if all {
				if realms != nil {
					t.Errorf("realms = %q with all = true, want nil: unrestricted is not a realm list", realms)
				}
				return
			}
			// %q, not %v: a scope of [""] prints as [] under %v, which is
			// indistinguishable from the empty scope it must not be confused with.
			if !slices.Equal(realms, tt.wantRealms) {
				t.Errorf("realms = %q, want %q", realms, tt.wantRealms)
			}
		})
	}
}

// The distinction every caller depends on, stated as its own test because
// reading an empty restricted scope as "no restriction" is what turned a blank
// realm into a tenant-wide read.
func TestResolveRealmScope_EmptyScopeIsNotUnrestricted(t *testing.T) {
	for _, policies := range [][]*domain.TenantPolicy{
		{{TenantID: "tenant-a", AllowedRealms: []string{}}},
		{{TenantID: "tenant-a", AllowedRealms: []string{""}}},
		{{TenantID: "tenant-a", AllowedRealms: []string{"", " "}}},
	} {
		realms, all := ResolveRealmScope(policies, "tenant-a")
		if all {
			t.Errorf("policies %+v resolved to unrestricted; a row that grants nothing must stay fail-closed",
				policies[0].AllowedRealms)
		}
		if len(realms) != 0 {
			t.Errorf("realms = %q, want none", realms)
		}
	}
}
