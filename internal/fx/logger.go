package fx

import (
	"go.uber.org/fx"

	"repos.defensepoint.com/iam/oss-pmt/kmt/internal/config"
	"repos.defensepoint.com/iam/oss-pmt/kmt/internal/logger"
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
