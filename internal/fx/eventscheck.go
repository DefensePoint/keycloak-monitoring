package fx

import (
	"time"

	"go.uber.org/fx"

	"github.com/DefensePoint/keycloak-monitoring/internal/eventscheck"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
)

// EventsCheckModule provides event check domain dependencies.
var EventsCheckModule = fx.Module("eventscheck",
	fx.Provide(
		provideEventsCheckLogger,
	),
)

// eventscheckLoggerAdapter adapts *logger.Logger to eventscheck.Logger.
type eventscheckLoggerAdapter struct {
	log *logger.Logger
}

func provideEventsCheckLogger(log *logger.Logger) eventscheck.Logger {
	return &eventscheckLoggerAdapter{log: log}
}

func (a *eventscheckLoggerAdapter) Info(msg string, args ...interface{}) {
	a.log.Info(msg)
}

func (a *eventscheckLoggerAdapter) Debug(msg string, args ...interface{}) {
	a.log.Debug(msg)
}

func (a *eventscheckLoggerAdapter) Warn(msg string, args ...interface{}) {
	a.log.Warn(msg)
}

func (a *eventscheckLoggerAdapter) Error(msg string, args ...interface{}) {
	a.log.Error(msg)
}

func (a *eventscheckLoggerAdapter) WithComponent(name string) eventscheck.Logger {
	return &eventscheckLoggerAdapter{log: a.log.WithComponent(name)}
}

// ProvideEventsCheckService creates the events check service with dependencies.
// This is a helper function that can be used to wire up the service with
// application-specific dependencies (AlertStore, NotificationService, Checks).
func ProvideEventsCheckService(
	checks []eventscheck.Check,
	alertStore eventscheck.AlertStore,
	notifier eventscheck.NotificationService,
	log eventscheck.Logger,
	pollInterval time.Duration,
) eventscheck.Service {
	return eventscheck.NewService(checks, alertStore, notifier, log, pollInterval)
}
