package config

import (
	"reflect"
	"testing"
)

// amfa_authenticator_ids is empty unless set, and takes the list set under
// keycloak.global or a comma-separated
// MONITORING_KEYCLOAK_GLOBAL_AMFA_AUTHENTICATOR_IDS.
func TestAmfaAuthenticatorIDs(t *testing.T) {
	cfg, err := LoadConfigQuiet(writeConfig(t, `
keycloak:
  global:
    realms: ["acme"]
`))
	if err != nil {
		t.Fatalf("LoadConfigQuiet: %v", err)
	}
	if got := cfg.Keycloak.Global.AmfaAuthenticatorIDs; len(got) != 0 {
		t.Errorf("default amfa_authenticator_ids = %v, want none", got)
	}

	cfg, err = LoadConfigQuiet(writeConfig(t, `
keycloak:
  global:
    amfa_authenticator_ids:
      - custom-adaptive-auth
      - custom-conditional-adaptive-auth
`))
	if err != nil {
		t.Fatalf("LoadConfigQuiet: %v", err)
	}
	want := []string{"custom-adaptive-auth", "custom-conditional-adaptive-auth"}
	if got := cfg.Keycloak.Global.AmfaAuthenticatorIDs; !reflect.DeepEqual(got, want) {
		t.Errorf("amfa_authenticator_ids = %v, want %v", got, want)
	}

	t.Setenv("MONITORING_KEYCLOAK_GLOBAL_AMFA_AUTHENTICATOR_IDS", "custom-adaptive-auth,custom-conditional-adaptive-auth")
	cfg, err = LoadConfigQuiet(writeConfig(t, `
keycloak:
  global:
    realms: ["acme"]
`))
	if err != nil {
		t.Fatalf("LoadConfigQuiet: %v", err)
	}
	if got := cfg.Keycloak.Global.AmfaAuthenticatorIDs; !reflect.DeepEqual(got, want) {
		t.Errorf("amfa_authenticator_ids from the environment = %v, want %v", got, want)
	}
}
