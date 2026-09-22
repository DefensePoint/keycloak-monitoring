package fx

import (
	"time"

	"go.uber.org/fx"

	"github.com/DefensePoint/keycloak-monitoring/internal/configcheck"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
)

// ConfigCheckModule provides configuration check domain dependencies.
var ConfigCheckModule = fx.Module("configcheck")

// Logger is used directly - no adapter needed (stable infrastructure)

// ProvideConfigCheckService creates the config check service with dependencies.
// This is a helper function that can be used to wire up the service with
// application-specific dependencies (AlertStore, NotificationService, Checks).
func ProvideConfigCheckService(
	checks []configcheck.Check,
	alertStore configcheck.AlertStore,
	notifier configcheck.NotificationService,
	log *logger.Logger,
	pollInterval time.Duration,
) configcheck.Service {
	return configcheck.NewService(checks, alertStore, notifier, log, pollInterval)
}
