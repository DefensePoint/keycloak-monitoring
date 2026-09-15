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

// parseTimeWindow returns the window a read should cover.
//
// An explicit RFC3339 start/end pair wins, because that is what the dashboard's
// range picker sends; hours stays the fallback for the callers that use it. A
// malformed or inverted pair falls back rather than erroring, so a bad query
// string degrades to the default window instead of blanking the panel.
//
// parseHoursParam alone could not serve the picker: it caps at 168 hours and
// never reads start/end, so "Last 7 days" silently returned one day.
func parseTimeWindow(r *http.Request, defaultHours int) (time.Time, time.Time) {
	startStr := r.URL.Query().Get("start")
	endStr := r.URL.Query().Get("end")

	if startStr != "" && endStr != "" {
		start, startErr := time.Parse(time.RFC3339, startStr)
		end, endErr := time.Parse(time.RFC3339, endStr)
		if startErr == nil && endErr == nil && end.After(start) {
			return start, end
		}
	}

	return parseHoursParam(r, defaultHours)
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
