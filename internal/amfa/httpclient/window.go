package httpclient

import "time"

// Lookback bounds, matching amfa/postgres. Both implementations must window
// identically or the same request would return different data depending on
// which one is wired up — and the parity test comparing them would be
// meaningless.
const (
	defaultLookbackDays = 30
	maxLookbackDays     = 90
)

// normalizeTimeWindow fills in absent bounds and caps the span:
//   - no end means now (UTC)
//   - no start means end minus lookbackDays
//   - a span wider than maxLookbackDays is trimmed from the start
//
// Identical to the postgres implementation, deliberately: an unbounded window
// would let one request read a realm's entire history.
func normalizeTimeWindow(start, end *time.Time, lookbackDays int) (time.Time, time.Time) {
	if lookbackDays <= 0 {
		lookbackDays = defaultLookbackDays
	}
	if lookbackDays > maxLookbackDays {
		lookbackDays = maxLookbackDays
	}
	if end == nil {
		now := time.Now().UTC()
		end = &now
	}
	if start == nil {
		s := end.Add(-time.Duration(lookbackDays) * 24 * time.Hour)
		start = &s
	}
	if end.Sub(*start) > time.Duration(maxLookbackDays)*24*time.Hour {
		s := end.Add(-time.Duration(maxLookbackDays) * 24 * time.Hour)
		start = &s
	}
	return *start, *end
}
