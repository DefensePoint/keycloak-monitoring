package config

import (
	"strings"
	"testing"
	"time"

	"github.com/go-viper/mapstructure/v2"
)

func secretExpiryYAML(entries string) string {
	return `
keycloak:
  global:
    config_checker:
      checks:
        identity_provider_secret_expiry:
          secrets:
` + entries
}

func TestIdentityProviderSecretExpiry_Defaults(t *testing.T) {
	cfg, err := LoadConfigQuiet(writeConfig(t, ""))
	if err != nil {
		t.Fatalf("LoadConfigQuiet: %v", err)
	}
	c := cfg.Keycloak.Global.ConfigChecker.Checks.IdentityProviderSecretExpiry
	if !c.Enabled {
		t.Error("the check is off by default; it should be on, and do nothing until a secret is listed")
	}
	if c.WarningDays != 30 {
		t.Errorf("warning_days = %d, want 30", c.WarningDays)
	}
	if len(c.Secrets) != 0 {
		t.Errorf("secrets = %v, want none", c.Secrets)
	}
}

// YAML reads an unquoted date as a timestamp, not a string; both spellings
// must give the same day.
func TestIdentityProviderSecretExpiry_ReadsQuotedAndUnquotedDates(t *testing.T) {
	cfg, err := LoadConfigQuiet(writeConfig(t, secretExpiryYAML(`
            - tenant: prod
              realm: customers
              identity_provider: entra
              expires_on: 2027-03-31
            - tenant: prod
              realm: staff
              identity_provider: okta
              expires_on: "2027-03-31"
`)))
	if err != nil {
		t.Fatalf("LoadConfigQuiet: %v", err)
	}
	secrets := cfg.Keycloak.Global.ConfigChecker.Checks.IdentityProviderSecretExpiry.Secrets
	if len(secrets) != 2 {
		t.Fatalf("secrets = %d, want 2", len(secrets))
	}
	want := time.Date(2027, 3, 31, 0, 0, 0, 0, time.UTC)
	for _, s := range secrets {
		got, err := s.ExpiryDate()
		if err != nil {
			t.Fatalf("%s/%s: ExpiryDate: %v", s.Realm, s.IdentityProvider, err)
		}
		if !got.Equal(want) {
			t.Errorf("%s/%s: expires %v, want %v", s.Realm, s.IdentityProvider, got, want)
		}
	}
	if secrets[0].Tenant != "prod" || secrets[0].Realm != "customers" || secrets[0].IdentityProvider != "entra" {
		t.Errorf("first entry = %+v", secrets[0])
	}
}

func TestIdentityProviderSecretExpiry_InvalidEntriesStopStartup(t *testing.T) {
	for _, tc := range []struct {
		name, entries, want string
	}{
		{name: "date in another format", want: "expires_on", entries: `
            - tenant: prod
              realm: customers
              identity_provider: entra
              expires_on: 31.03.2027
`},
		{name: "no date", want: "expires_on", entries: `
            - tenant: prod
              realm: customers
              identity_provider: entra
`},
		{name: "no tenant", want: "tenant", entries: `
            - realm: customers
              identity_provider: entra
              expires_on: 2027-03-31
`},
		{name: "no realm", want: "realm", entries: `
            - tenant: prod
              identity_provider: entra
              expires_on: 2027-03-31
`},
		{name: "no identity provider", want: "identity_provider", entries: `
            - tenant: prod
              realm: customers
              expires_on: 2027-03-31
`},
		{name: "same provider twice", want: "more than once", entries: `
            - tenant: prod
              realm: customers
              identity_provider: entra
              expires_on: 2027-03-31
            - tenant: prod
              realm: customers
              identity_provider: entra
              expires_on: 2028-03-31
`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := LoadConfigQuiet(writeConfig(t, secretExpiryYAML(tc.entries)))
			if err == nil {
				t.Fatal("LoadConfigQuiet accepted the entry")
			}
			if !strings.Contains(err.Error(), "identity_provider_secret_expiry") || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q does not name the setting and %q", err, tc.want)
			}
		})
	}
}

func TestIdentityProviderSecretExpiry_NonPositiveWarningDaysStopsStartup(t *testing.T) {
	_, err := LoadConfigQuiet(writeConfig(t, `
keycloak:
  global:
    config_checker:
      checks:
        identity_provider_secret_expiry:
          warning_days: 0
`))
	if err == nil || !strings.Contains(err.Error(), "warning_days") {
		t.Fatalf("error = %v, want one about warning_days", err)
	}
}

// The loader keeps viper's own hooks: its comma split fills a slice of any
// type, where mapstructure's fills only []string. The date hook is the only
// addition.
func TestDecodeHooks_KeepViperDefaults(t *testing.T) {
	var out struct {
		Ports   []int         `mapstructure:"ports"`
		Names   []string      `mapstructure:"names"`
		Timeout time.Duration `mapstructure:"timeout"`
		Day     string        `mapstructure:"day"`
	}
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		DecodeHook:       decodeHooks(),
		WeaklyTypedInput: true,
		Result:           &out,
	})
	if err != nil {
		t.Fatalf("NewDecoder: %v", err)
	}
	in := map[string]any{"ports": "80,443", "names": "a,b", "timeout": "30s",
		"day": time.Date(2027, 3, 31, 0, 0, 0, 0, time.UTC)}
	if err := decoder.Decode(in); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if len(out.Ports) != 2 || out.Ports[0] != 80 || out.Ports[1] != 443 {
		t.Errorf("ports = %v, want [80 443]", out.Ports)
	}
	if len(out.Names) != 2 || out.Timeout != 30*time.Second || out.Day != "2027-03-31" {
		t.Errorf("names %v, timeout %v, day %q", out.Names, out.Timeout, out.Day)
	}
}
