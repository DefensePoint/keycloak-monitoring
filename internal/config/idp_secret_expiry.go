package config

import (
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/go-viper/mapstructure/v2"
)

// decodeHooks are the hooks config.yaml is decoded with: viper's defaults,
// plus yamlDateToString. Passing hooks to viper replaces its defaults, so
// they are repeated here.
func decodeHooks() mapstructure.DecodeHookFunc {
	return mapstructure.ComposeDecodeHookFunc(
		mapstructure.StringToTimeDurationHookFunc(),
		stringToWeakSlice(","),
		yamlDateToString,
	)
}

// stringToWeakSlice splits a string into a slice of any type, as viper's
// default does (an environment variable "80,443" fills a []int).
// mapstructure.StringToSliceHookFunc fills only a []string.
func stringToWeakSlice(sep string) mapstructure.DecodeHookFunc {
	return func(from reflect.Type, to reflect.Type, data any) (any, error) {
		if from.Kind() != reflect.String || to.Kind() != reflect.Slice {
			return data, nil
		}
		raw := data.(string)
		if raw == "" {
			return []string{}, nil
		}
		return strings.Split(raw, sep), nil
	}
}

// yamlDateToString turns a timestamp into the text it was written as, for a
// string setting: YAML reads an unquoted 2027-03-31 as a timestamp, which
// would otherwise fail to decode.
func yamlDateToString(_ reflect.Type, to reflect.Type, data any) (any, error) {
	t, ok := data.(time.Time)
	if !ok || to.Kind() != reflect.String {
		return data, nil
	}
	if t.Equal(t.Truncate(24 * time.Hour)) {
		return t.UTC().Format(time.DateOnly), nil
	}
	return t.Format(time.RFC3339), nil
}

// Validate rejects an entry that names no tenant, realm, identity provider or
// date, a date not written as YYYY-MM-DD, and a provider listed twice.
func (c IdentityProviderSecretExpiryCheckConfig) Validate() error {
	const key = "keycloak.global.config_checker.checks.identity_provider_secret_expiry"
	if c.WarningDays <= 0 {
		return fmt.Errorf("%s.warning_days must be a positive number of days, got %d", key, c.WarningDays)
	}
	seen := make(map[IdentityProviderSecret]bool, len(c.Secrets))
	for i, s := range c.Secrets {
		where := fmt.Sprintf("%s.secrets[%d]", key, i)
		switch {
		case s.Tenant == "":
			return fmt.Errorf("%s: tenant is required", where)
		case s.Realm == "":
			return fmt.Errorf("%s: realm is required", where)
		case s.IdentityProvider == "":
			return fmt.Errorf("%s: identity_provider is required", where)
		case s.ExpiresOn == "":
			return fmt.Errorf("%s: expires_on is required, as YYYY-MM-DD", where)
		}
		if _, err := s.ExpiryDate(); err != nil {
			return fmt.Errorf("%s: expires_on %q is not a date written as YYYY-MM-DD", where, s.ExpiresOn)
		}
		id := IdentityProviderSecret{Tenant: s.Tenant, Realm: s.Realm, IdentityProvider: s.IdentityProvider}
		if seen[id] {
			return fmt.Errorf("%s: identity provider %q of tenant %q realm %q is listed more than once",
				where, s.IdentityProvider, s.Tenant, s.Realm)
		}
		seen[id] = true
	}
	return nil
}
