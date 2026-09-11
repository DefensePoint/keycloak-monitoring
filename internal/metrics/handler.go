package metrics

import (
	"crypto/subtle"
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/DefensePoint/keycloak-monitoring/internal/config"
)

// GuardedHandler serves the Prometheus exposition endpoint for the registry,
// optionally behind the bearer token from cfg.
func GuardedHandler(m *Registry, cfg config.MetricsConfig) http.Handler {
	handler := promhttp.HandlerFor(m.Gatherer(), promhttp.HandlerOpts{})

	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if token := cfg.AuthToken; token != "" {
			provided := req.Header.Get("Authorization")
			if subtle.ConstantTimeCompare([]byte(provided), []byte("Bearer "+token)) != 1 {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
		}
		handler.ServeHTTP(w, req)
	})
}
