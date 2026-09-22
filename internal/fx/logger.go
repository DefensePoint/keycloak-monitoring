package fx

import (
	"go.uber.org/fx"

	"github.com/DefensePoint/keycloak-monitoring/internal/config"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
)

// LoggerModule provides logger
var LoggerModule = fx.Module("logger",
	fx.Provide(
		provideLogger,
	),
)

func provideLogger(cfg *config.AppConfig) *logger.Logger {
	return logger.NewConsole(cfg.Logging.Level, cfg.Logging.Output.Pretty)
}
