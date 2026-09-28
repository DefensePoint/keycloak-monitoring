package postgres

import (
	"reflect"
	"testing"

	"github.com/DefensePoint/keycloak-monitoring/pkg/database"
)

func TestConvertPolicy_AllowedRealms(t *testing.T) {
	tests := []struct {
		name             string
		column           []byte
		wantRealms       []string
		wantUnrestricted bool
	}{
		{
			name:             "SQL NULL column is unrestricted",
			column:           nil,
			wantUnrestricted: true,
		},
		{
			name:   "stored empty list allows no realm",
			column: []byte(`[]`),
		},
		{
			// json.Marshal of a nil slice: no list stored, so no restriction.
			name:             "stored JSON null is unrestricted",
			column:           []byte(`null`),
			wantUnrestricted: true,
		},
		{
			name:       "stored list allows exactly its realms",
			column:     []byte(`["realmA","realmB"]`),
			wantRealms: []string{"realmA", "realmB"},
		},
		{
			name:   "unreadable column fails closed",
			column: []byte(`not json`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convertPolicy(&database.TenantPolicy{TenantID: "tenant-a", AllowedRealms: tt.column})

			if got.RealmsUnrestricted != tt.wantUnrestricted {
				t.Errorf("RealmsUnrestricted = %v, want %v", got.RealmsUnrestricted, tt.wantUnrestricted)
			}
			if got.RealmsUnrestricted != realmsUnrestricted(tt.column) {
				t.Errorf("RealmsUnrestricted = %v, disagrees with the realm check on the same column", got.RealmsUnrestricted)
			}
			if len(got.AllowedRealms) == 0 && len(tt.wantRealms) == 0 {
				return
			}
			if !reflect.DeepEqual(got.AllowedRealms, tt.wantRealms) {
				t.Errorf("AllowedRealms = %v, want %v", got.AllowedRealms, tt.wantRealms)
			}
		})
	}
}

func TestMarshalAllowedRealms(t *testing.T) {
	tests := []struct {
		name             string
		realms           []string
		want             []byte
		wantUnrestricted bool
	}{
		{
			name:             "a nil list stores SQL NULL, not the null literal",
			realms:           nil,
			want:             nil,
			wantUnrestricted: true,
		},
		{
			name:   "an explicit empty list stores [] and stays fail-closed",
			realms: []string{},
			want:   []byte(`[]`),
		},
		{
			name:   "a populated list stores its realms",
			realms: []string{"realmA"},
			want:   []byte(`["realmA"]`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := marshalAllowedRealms(tt.realms)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("marshalAllowedRealms() = %s, want %s", got, tt.want)
			}
			if realmsUnrestricted(got) != tt.wantUnrestricted {
				t.Errorf("the column just written reads back unrestricted = %v, want %v",
					realmsUnrestricted(got), tt.wantUnrestricted)
			}
		})
	}
}
