package healthcheck

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
)

// readinessTimeout bounds one readiness check. It stays under the 3s the
// -healthcheck probe waits, so a slow check answers 503 rather than leaving
// the probe to time out.
const readinessTimeout = 2 * time.Second

// Readiness answers /ready: 200 while its check passes, 503 while it fails or
// does not finish in time. It is the readiness probe, not the liveness one: a
// failing check takes the instance out of traffic, where a failing /health
// would restart it, and a restart does not bring a database back.
//
// /ready is served without authentication, so the reply says only whether the
// instance is ready; the reason goes to the log, once per change of state, so
// a probe every few seconds through an outage does not flood it.
type Readiness struct {
	log     *logger.Logger
	timeout time.Duration

	mu      sync.Mutex
	check   func(context.Context) error
	failing bool
}

// NewReadiness returns a Readiness with no check, which is always ready.
func NewReadiness(log *logger.Logger) *Readiness {
	if log == nil {
		log = logger.NewNoop()
	}
	return &Readiness{log: log, timeout: readinessTimeout}
}

// SetCheck sets what must pass for the instance to be ready.
func (r *Readiness) SetCheck(check func(context.Context) error) {
	r.mu.Lock()
	r.check = check
	r.mu.Unlock()
}

func (r *Readiness) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.mu.Lock()
	check := r.check
	r.mu.Unlock()

	var err error
	if check != nil {
		ctx, cancel := context.WithTimeout(req.Context(), r.timeout)
		err = check(ctx)
		cancel()
	}
	r.record(err)

	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"status":"not ready"}`))
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ready"}`))
}

func (r *Readiness) record(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	switch {
	case err != nil && !r.failing:
		r.failing = true
		r.log.Warn("Not ready: the database did not answer; /ready returns 503 until it does", logger.Err(err))
	case err == nil && r.failing:
		r.failing = false
		r.log.Info("Ready again: the database answers")
	}
}
