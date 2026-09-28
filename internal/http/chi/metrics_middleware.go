package chi

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/DefensePoint/keycloak-monitoring/internal/metrics"
)

// metricsResponseWriter captures the status code written by downstream handlers.
type metricsResponseWriter struct {
	http.ResponseWriter
	status int
}

func (w *metricsResponseWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *metricsResponseWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(b)
}

// MetricsMiddleware records request counts, latency and in-flight requests.
// Routes are labelled with the Chi route pattern rather than the raw path so
// that per-tenant URLs do not create a new time series each.
func MetricsMiddleware(m *metrics.Registry) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			m.HTTPRequestsInFlight.Inc()
			defer m.HTTPRequestsInFlight.Dec()

			start := time.Now()
			rw := &metricsResponseWriter{ResponseWriter: w}

			next.ServeHTTP(rw, req)

			route := chi.RouteContext(req.Context()).RoutePattern()
			if route == "" {
				route = "unmatched"
			}

			if rw.status == 0 {
				rw.status = http.StatusOK
			}

			m.HTTPRequestsTotal.WithLabelValues(route, req.Method, strconv.Itoa(rw.status)).Inc()
			m.HTTPRequestDuration.WithLabelValues(route, req.Method).Observe(time.Since(start).Seconds())
		})
	}
}
