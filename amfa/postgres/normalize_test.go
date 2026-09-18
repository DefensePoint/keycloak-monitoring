package postgres

import (
	"testing"
	"time"
)

func TestNormalizeTimeWindow_DefaultsTo30DaysWhenStartNil(t *testing.T) {
	end := time.Now()
	start, gotEnd := normalizeTimeWindow(nil, &end, defaultLookbackDays)
	if !gotEnd.Equal(end) {
		t.Errorf("end time was modified: want %v, got %v", end, gotEnd)
	}
	delta := end.Sub(start)
	if delta < 29*24*time.Hour || delta > 31*24*time.Hour {
		t.Errorf("expected ~30 day window, got %v", delta)
	}
}

func TestNormalizeTimeWindow_CapsAt90Days(t *testing.T) {
	end := time.Now()
	start := end.Add(-365 * 24 * time.Hour)
	s, _ := normalizeTimeWindow(&start, &end, defaultLookbackDays)
	if end.Sub(s) > 90*24*time.Hour+time.Minute {
		t.Errorf("expected window capped at 90 days, got %v", end.Sub(s))
	}
}

func TestNormalizeTimeWindow_DefaultsEndToNowWhenNil(t *testing.T) {
	before := time.Now().UTC()
	_, end := normalizeTimeWindow(nil, nil, defaultLookbackDays)
	after := time.Now().UTC()
	if end.Before(before) || end.After(after.Add(time.Second)) {
		t.Errorf("expected end ~now, got %v (before=%v after=%v)", end, before, after)
	}
}

func TestNormalizeTimeWindow_PassesThroughValidWindow(t *testing.T) {
	end := time.Now()
	start := end.Add(-7 * 24 * time.Hour)
	gotStart, gotEnd := normalizeTimeWindow(&start, &end, defaultLookbackDays)
	if !gotStart.Equal(start) {
		t.Errorf("start time was modified: want %v, got %v", start, gotStart)
	}
	if !gotEnd.Equal(end) {
		t.Errorf("end time was modified: want %v, got %v", end, gotEnd)
	}
}

func TestNormalizeTimeWindow_PerTenantLookbackApplied(t *testing.T) {
	end := time.Now()
	// 14-day lookback (per-tenant config) should drive the default start, not 30.
	start, _ := normalizeTimeWindow(nil, &end, 14)
	delta := end.Sub(start)
	if delta < 13*24*time.Hour || delta > 15*24*time.Hour {
		t.Errorf("expected ~14 day window with lookbackDays=14, got %v", delta)
	}
}

func TestNormalizeTimeWindow_LookbackCappedAt90Days(t *testing.T) {
	end := time.Now()
	// 365-day "config" should be clamped to maxLookbackDays (90).
	start, _ := normalizeTimeWindow(nil, &end, 365)
	delta := end.Sub(start)
	if delta < 89*24*time.Hour || delta > 91*24*time.Hour {
		t.Errorf("expected ~90 day window when lookbackDays>%d, got %v", maxLookbackDays, delta)
	}
}

func TestNormalizeTimeWindow_LookbackZeroFallsBackToDefault(t *testing.T) {
	end := time.Now()
	start, _ := normalizeTimeWindow(nil, &end, 0)
	delta := end.Sub(start)
	if delta < 29*24*time.Hour || delta > 31*24*time.Hour {
		t.Errorf("expected default %d-day window when lookbackDays=0, got %v", defaultLookbackDays, delta)
	}
}
