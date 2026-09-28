package fx

import (
	"errors"
	"net/http"

	"go.uber.org/fx"

	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
)

// serveOrShutdown runs the listener and, on a fatal error, asks fx to stop
// the whole process. Logging alone would leave a zombie process with no
// listener; exiting nonzero lets the container restart policy recover it.
func serveOrShutdown(name string, srv *http.Server, log *logger.Logger, sd fx.Shutdowner) {
	err := srv.ListenAndServe()
	if err == nil || errors.Is(err, http.ErrServerClosed) {
		return
	}
	log.Error(name+" listener failed, shutting down", logger.Err(err))
	if sdErr := sd.Shutdown(fx.ExitCode(1)); sdErr != nil {
		log.Error("Shutdown request failed", logger.Err(sdErr))
	}
}
