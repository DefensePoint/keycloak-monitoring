package chi_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DefensePoint/keycloak-monitoring/internal/config"
	chihttp "github.com/DefensePoint/keycloak-monitoring/internal/http/chi"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/internal/metrics"
)

func newRouter(t *testing.T, cfg config.MetricsConfig) *chihttp.Router {
	t.Helper()

	r := chihttp.NewRouter(logger.NewNoop(), nil, nil)
	r.SetMetrics(metrics.New(), cfg)
	r.RegisterAllRoutes()
	return r
}

func TestMetricsEndpointServesExposition(t *testing.T) {
	r := newRouter(t, config.MetricsConfig{Enabled: true})

	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "pmp_") {
		t.Fatal("expected pmp_ metrics in response body")
	}
}

func TestMetricsEndpointDisabled(t *testing.T) {
	r := newRouter(t, config.MetricsConfig{Enabled: false})

	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 when disabled, got %d", rec.Code)
	}
}

func TestMetricsEndpointRequiresToken(t *testing.T) {
	r := newRouter(t, config.MetricsConfig{Enabled: true, AuthToken: "s3cret"})

	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without token, got %d", rec.Code)
	}

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.Header.Set("Authorization", "Bearer s3cret")
	rec = httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 with valid token, got %d", rec.Code)
	}
}

func TestMetricsMiddlewareUsesRoutePattern(t *testing.T) {
	r := newRouter(t, config.MetricsConfig{Enabled: true})

	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))

	rec = httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	body := rec.Body.String()
	if !strings.Contains(body, `route="/health"`) {
		t.Fatalf("expected route pattern label for /health, got:\n%s", body)
	}
}
