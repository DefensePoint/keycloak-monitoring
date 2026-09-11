package chi

import (
	"net/http"
	"strconv"
	"time"

	httputil "github.com/DefensePoint/keycloak-monitoring/internal/http"
)

// Constants for pagination limits
const (
	DefaultLimit = httputil.DefaultPageLimit
	MaxLimit     = httputil.MaxPageLimit
)

// parseIntParam extracts an integer query parameter with a default value.
func parseIntParam(r *http.Request, key string, defaultValue int) int {
	return httputil.GetIntQuery(r, key, defaultValue)
}

// parseHoursParam parses hours query parameter and returns from/to time range.
// If hours is not specified, defaults to the provided defaultHours.
// Validates bounds: minimum 1 hour, maximum 168 hours (7 days).
func parseHoursParam(r *http.Request, defaultHours int) (time.Time, time.Time) {
	hoursStr := r.URL.Query().Get("hours")
	hours := defaultHours

	if hoursStr != "" {
		if h, err := strconv.Atoi(hoursStr); err == nil && h > 0 {
			hours = h
		}
	}

	// Apply bounds: min 1 hour, max 168 hours (7 days)
	if hours < 1 {
		hours = 1
	}
	if hours > 168 {
		hours = 168
	}

	to := time.Now()
	from := to.Add(-time.Duration(hours) * time.Hour)

	return from, to
}
