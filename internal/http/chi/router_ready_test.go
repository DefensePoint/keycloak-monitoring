package chi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
)

func routerStatus(t *testing.T, r *Router, path string) int {
	t.Helper()
	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec.Code
}

// /ready follows the readiness check; /health is liveness and stays 200, so an
// unreachable database takes the instance out of traffic without restarting it.
func TestRouter_ReadyFollowsTheCheckAndHealthDoesNot(t *testing.T) {
	r := NewRouter(logger.NewNoop(), nil, nil)
	var dbErr error
	r.SetReadinessCheck(func(context.Context) error { return dbErr })
	r.RegisterAllRoutes()

	if got := routerStatus(t, r, "/ready"); got != http.StatusOK {
		t.Errorf("/ready with the database up = %d, want 200", got)
	}

	dbErr = errors.New("connection refused")
	if got := routerStatus(t, r, "/ready"); got != http.StatusServiceUnavailable {
		t.Errorf("/ready with the database down = %d, want 503", got)
	}
	if got := routerStatus(t, r, "/health"); got != http.StatusOK {
		t.Errorf("/health with the database down = %d, want 200", got)
	}

	dbErr = nil
	if got := routerStatus(t, r, "/ready"); got != http.StatusOK {
		t.Errorf("/ready after the database came back = %d, want 200", got)
	}
}
