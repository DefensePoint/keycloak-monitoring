package fx

import (
	"reflect"
	"testing"
)

// A tenant's amfa_authenticator_ids is reported as ignored, like the other
// settings only read from keycloak.global.
func TestGlobalOnlyOverrides_AmfaAuthenticatorIDs(t *testing.T) {
	got := globalOnlyOverrides(map[string]any{
		"realms":                 []any{"acme"},
		"amfa_authenticator_ids": []any{"custom-adaptive-auth"},
		"amfa_mirror":            map[string]any{"enabled": true},
	})
	want := []string{"amfa_authenticator_ids", "amfa_mirror"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("globalOnlyOverrides = %v, want %v", got, want)
	}
}
