package mcp

import (
	"errors"
	"testing"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/config"
)

func testMCPConfig() *config.MCPConfig {
	return &config.MCPConfig{DefaultPageSize: 50, MaxPageSize: 500}
}

func TestResolvePageSize(t *testing.T) {
	cases := []struct {
		name      string
		requested int
		cfg       *config.MCPConfig
		want      int
	}{
		{"zero gets default", 0, testMCPConfig(), 50},
		{"negative gets default", -5, testMCPConfig(), 50},
		{"in range passes through", 200, testMCPConfig(), 200},
		{"above max clamps", 10000, testMCPConfig(), 500},
		{"exactly max passes", 500, testMCPConfig(), 500},
		{"nil config falls back", 0, nil, 50},
		{"nil config clamps", 10000, nil, 500},
		{"default above max clamps to max", 0, &config.MCPConfig{DefaultPageSize: 900, MaxPageSize: 100}, 100},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ResolvePageSize(tc.requested, tc.cfg); got != tc.want {
				t.Fatalf("got %d, want %d", got, tc.want)
			}
		})
	}
}

func TestParseTimestampNormalizesToUTC(t *testing.T) {
	got, err := ParseTimestamp("from", "2026-01-02T15:04:05+02:00")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := time.Date(2026, 1, 2, 13, 4, 5, 0, time.UTC)
	if !got.Equal(want) || got.Location() != time.UTC {
		t.Fatalf("got %v (%v), want %v UTC", got, got.Location(), want)
	}
}

func TestParseTimestampRejectsMalformed(t *testing.T) {
	cases := []string{
		"",
		"yesterday",
		"2026-01-02",
		"2026-01-02 15:04:05",
		"2026-01-02T15:04:05",
		"1735822800",
		"2026-13-45T99:99:99Z",
	}
	for _, raw := range cases {
		t.Run(raw, func(t *testing.T) {
			_, err := ParseTimestamp("from", raw)
			if err == nil {
				t.Fatalf("expected error for %q", raw)
			}
			if !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("expected ErrInvalidInput classification, got %v", err)
			}
		})
	}
}

func TestResolveWindowDefaults(t *testing.T) {
	now := time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)

	from, to, err := ResolveWindow("", "", now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !to.Equal(now) {
		t.Fatalf("to = %v, want %v", to, now)
	}
	if !from.Equal(now.Add(-DefaultWindow)) {
		t.Fatalf("from = %v, want %v", from, now.Add(-DefaultWindow))
	}
}

func TestResolveWindowExplicitBounds(t *testing.T) {
	now := time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)

	from, to, err := ResolveWindow("2026-09-01T00:00:00Z", "2026-09-03T00:00:00Z", now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if from.Day() != 1 || to.Day() != 3 {
		t.Fatalf("unexpected window %v .. %v", from, to)
	}
}

func TestResolveWindowRejectsInvertedBounds(t *testing.T) {
	now := time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)

	_, _, err := ResolveWindow("2026-09-03T00:00:00Z", "2026-09-01T00:00:00Z", now)
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}

func TestResolveWindowRejectsOversizedWindow(t *testing.T) {
	now := time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)

	_, _, err := ResolveWindow("2026-01-01T00:00:00Z", "2026-09-01T00:00:00Z", now)
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput for >90 day window, got %v", err)
	}
}

func TestResolveWindowNinetyDaysExactlyAllowed(t *testing.T) {
	now := time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)
	to := now
	from := to.Add(-MaxWindow)

	_, _, err := ResolveWindow(from.Format(time.RFC3339), to.Format(time.RFC3339), now)
	if err != nil {
		t.Fatalf("90-day window should be allowed, got %v", err)
	}
}

func TestResolveWindowRejectsMalformedBound(t *testing.T) {
	now := time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)

	if _, _, err := ResolveWindow("not-a-time", "", now); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput for malformed from, got %v", err)
	}
	if _, _, err := ResolveWindow("", "not-a-time", now); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput for malformed to, got %v", err)
	}
}
