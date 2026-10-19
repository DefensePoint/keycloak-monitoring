package config

import (
	"fmt"
	"sort"

	"github.com/go-viper/mapstructure/v2"
)

// checkTenantOverrides rejects a tenant config: block that cannot be applied,
// as a typed field of the wrong type would be. Unknown keys are left for the
// monitor to report when it applies the overrides.
func checkTenantOverrides(kc *KeycloakConfig) error {
	ids := make([]string, 0, len(kc.Tenants))
	for id := range kc.Tenants {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if _, _, err := ApplyInstanceOverrides(kc.Global, kc.Tenants[id].Config); err != nil {
			return fmt.Errorf("keycloak.tenants.%s.config: %w", id, err)
		}
	}
	return nil
}

// ApplyInstanceOverrides returns base with a tenant's overrides decoded on
// top, using config.yaml's snake_case keys, duration strings such as "30s"
// and weak typing. Settings the overrides do not name keep their base value,
// and a list in them replaces the base list. base is not changed.
//
// Keys that match no setting are returned sorted, for the caller to report.
func ApplyInstanceOverrides(base KeycloakInstanceConfig, overrides map[string]any) (KeycloakInstanceConfig, []string, error) {
	merged := base
	var meta mapstructure.Metadata
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		// The same hooks and weak typing viper uses for config.yaml.
		DecodeHook: mapstructure.ComposeDecodeHookFunc(
			mapstructure.StringToTimeDurationHookFunc(),
			mapstructure.StringToSliceHookFunc(","),
		),
		WeaklyTypedInput: true,
		// A list in the overrides gets a new slice: without this the decoder
		// writes into base's backing array, patching the base list element by
		// element and changing it for every tenant.
		ZeroFields: true,
		Metadata:   &meta,
		Result:     &merged,
	})
	if err != nil {
		return KeycloakInstanceConfig{}, nil, err
	}
	if err := decoder.Decode(overrides); err != nil {
		return KeycloakInstanceConfig{}, nil, err
	}
	sort.Strings(meta.Unused)
	return merged, meta.Unused, nil
}
