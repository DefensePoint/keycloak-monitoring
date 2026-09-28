package mcp

import (
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/config"
)

const (
	fallbackDefaultPageSize = 50
	fallbackMaxPageSize     = 500

	// DefaultWindow is applied when a tool call omits both from and to.
	DefaultWindow = 24 * time.Hour

	// MaxWindow caps the span of any requested time window.
	MaxWindow = 90 * 24 * time.Hour
)

// ResolvePageSize turns a requested page size into the one actually applied:
// zero or negative requests get the configured default, and anything larger
// than the configured maximum is capped at it. The two are separate settings,
// and a default configured above the maximum is itself capped.
func ResolvePageSize(requested int, cfg *config.MCPConfig) int {
	defaultSize, maxSize := fallbackDefaultPageSize, fallbackMaxPageSize
	if cfg != nil {
		if cfg.DefaultPageSize > 0 {
			defaultSize = cfg.DefaultPageSize
		}
		if cfg.MaxPageSize > 0 {
			maxSize = cfg.MaxPageSize
		}
	}
	if defaultSize > maxSize {
		defaultSize = maxSize
	}
	switch {
	case requested <= 0:
		return defaultSize
	case requested > maxSize:
		return maxSize
	default:
		return requested
	}
}

// ParseTimestamp parses a strict RFC3339 timestamp and normalizes it to UTC.
// Malformed input, including timestamps without an offset, is rejected.
func ParseTimestamp(field, value string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, Invalidf("%s must be an RFC3339 timestamp, e.g. 2026-01-02T15:04:05Z", field)
	}
	return t.UTC(), nil
}

// ResolveWindow turns optional from/to strings into a concrete UTC window:
// a missing to means now, a missing from means to minus DefaultWindow, and
// the resulting span must be positive and at most MaxWindow.
func ResolveWindow(fromRaw, toRaw string, now time.Time) (from, to time.Time, err error) {
	to = now.UTC()
	if toRaw != "" {
		if to, err = ParseTimestamp("to", toRaw); err != nil {
			return time.Time{}, time.Time{}, err
		}
	}

	from = to.Add(-DefaultWindow)
	if fromRaw != "" {
		if from, err = ParseTimestamp("from", fromRaw); err != nil {
			return time.Time{}, time.Time{}, err
		}
	}

	if !from.Before(to) {
		return time.Time{}, time.Time{}, Invalidf("from must be before to")
	}
	if to.Sub(from) > MaxWindow {
		return time.Time{}, time.Time{}, Invalidf("time window must not exceed 90 days")
	}
	return from, to, nil
}
