package fx

import (
	"go.uber.org/fx"

	"repos.defensepoint.com/iam/oss-pmt/kmt/internal/config"
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
