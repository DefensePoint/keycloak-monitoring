package notifications

import "github.com/DefensePoint/keycloak-monitoring/internal/config"

// productName is the name messages are signed with: branding.name, or the
// default name for a notifier built without one.
func productName(name string) string {
	if name == "" {
		return config.DefaultBrandingName
	}
	return name
}
