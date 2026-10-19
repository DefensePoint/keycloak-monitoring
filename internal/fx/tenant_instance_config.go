package fx

import (
	"encoding/json"
	"fmt"

	"github.com/DefensePoint/keycloak-monitoring/internal/config"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
)

// TenantInstanceConfig applies a tenant's configuration overrides to the
// global Keycloak instance config. Overrides are JSON in the config file's
// own form: its snake_case keys, and durations such as "30s". Settings the
// override does not name keep their global value, and a list in it replaces
// the global list. The global config is not changed.
//
// Unknown keys are applied around and logged, so a misspelt setting is not
// dropped unnoticed; invalid JSON or a value of the wrong type is an error.
func (f *keycloakMonitorFactory) TenantInstanceConfig(tenantID string, global any, configuration string) (any, error) {
	base, ok := global.(*config.KeycloakInstanceConfig)
	if !ok {
		return nil, fmt.Errorf("global instance config is %T, not *config.KeycloakInstanceConfig", global)
	}

	var overrides map[string]any
	if err := json.Unmarshal([]byte(configuration), &overrides); err != nil {
		return nil, fmt.Errorf("configuration is not a JSON object: %w", err)
	}

	merged, unknown, err := config.ApplyInstanceOverrides(*base, overrides)
	if err != nil {
		return nil, fmt.Errorf("configuration does not match the Keycloak settings: %w", err)
	}

	if len(unknown) > 0 {
		f.log.Warn("Tenant configuration has unknown keys, ignoring them",
			logger.Str("tenant_id", tenantID),
			logger.Strs("unknown_keys", unknown))
	}
	return &merged, nil
}
