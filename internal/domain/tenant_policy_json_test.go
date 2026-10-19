package domain

import (
	"encoding/json"
	"testing"
)

// The web client decides whether a caller's realm access is limited from
// tenant_policies. allowed_realms is omitted both for an unrestricted row
// (NULL column) and for a deny-all row (empty list), so without
// realms_unrestricted the two are indistinguishable in the payload.
func TestTenantPolicy_JSONReportsWhetherRealmsAreUnrestricted(t *testing.T) {
	cases := []struct {
		name   string
		policy TenantPolicy
		want   bool
	}{
		{"unrestricted row", TenantPolicy{TenantID: "t", RealmsUnrestricted: true}, true},
		{"restricted to named realms", TenantPolicy{TenantID: "t", AllowedRealms: []string{"a"}}, false},
		{"deny-all empty list", TenantPolicy{TenantID: "t", AllowedRealms: []string{}}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := json.Marshal(tc.policy)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			var got map[string]any
			if err := json.Unmarshal(raw, &got); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			v, ok := got["realms_unrestricted"]
			if !ok {
				t.Fatalf("realms_unrestricted missing from %s", raw)
			}
			if v != tc.want {
				t.Errorf("realms_unrestricted = %v, want %v (%s)", v, tc.want, raw)
			}
		})
	}
}
