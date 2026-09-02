package chi

import (
	"context"
	"reflect"
	"testing"

	"repos.defensepoint.com/iam/oss-pmt/kmt/internal/domain"
)

type mockRealmLister struct {
	realms []*domain.KeycloakRealmInfo
	err    error
}

func (m *mockRealmLister) ListRealms(_ context.Context, _ string) ([]*domain.KeycloakRealmInfo, error) {
	return m.realms, m.err
}

func realms(names ...string) []*domain.KeycloakRealmInfo {
	out := make([]*domain.KeycloakRealmInfo, 0, len(names))
	for _, n := range names {
		out = append(out, &domain.KeycloakRealmInfo{RealmName: n})
	}
	return out
}

func TestTenantEventSources(t *testing.T) {
	tests := []struct {
		name            string
		realms          []*domain.KeycloakRealmInfo
		requestedSource string
		want            []string
	}{
		{
			name:   "all sources for the tenant's realms",
			realms: realms("realmA", "realmB"),
			want:   []string{"keycloak:realmA", "amfa:realmA", "keycloak:realmB", "amfa:realmB"},
		},
		{
			name:            "narrow to one exact source within the tenant",
			realms:          realms("realmA", "realmB"),
			requestedSource: "keycloak:realmA",
			want:            []string{"keycloak:realmA"},
		},
		{
			name:            "narrow by realm name matches both feeds",
			realms:          realms("realmA", "realmB"),
			requestedSource: "realmB",
			want:            []string{"keycloak:realmB", "amfa:realmB"},
		},
		{
			name:            "cross-tenant source is denied",
			realms:          realms("realmA"),
			requestedSource: "keycloak:realmB",
			want:            []string{},
		},
		{
			name:   "tenant with no realms yields no sources",
			realms: nil,
			want:   []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &SystemHandlers{realmLister: &mockRealmLister{realms: tt.realms}}

			got, err := h.tenantEventSources(context.Background(), "tenant-a", tt.requestedSource)
			if err != nil {
				t.Fatalf("tenantEventSources() error = %v", err)
			}
			if len(got) == 0 && len(tt.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("tenantEventSources() = %v, want %v", got, tt.want)
			}
		})
	}
}
