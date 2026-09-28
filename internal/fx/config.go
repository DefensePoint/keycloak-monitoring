package fx

import (
	"go.uber.org/fx"

	"github.com/DefensePoint/keycloak-monitoring/internal/config"
)

// ConfigModule provides configuration loading
var ConfigModule = fx.Module("config",
	fx.Provide(
		provideAppConfig,
	),
)

// ConfigPath is used to inject the config file path
type ConfigPath string

func provideAppConfig(path ConfigPath) (*config.AppConfig, error) {
	return config.LoadConfig(string(path))
}
