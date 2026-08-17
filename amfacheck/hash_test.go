package amfacheck

import (
	"testing"
)

func TestAlertID_StableAndDistinct(t *testing.T) {
	a := alertID("amfa-risk-rejected", "tenant-1", "event-1")
	b := alertID("amfa-risk-rejected", "tenant-1", "event-1")
	if a != b {
		t.Errorf("alertID not stable: %q != %q", a, b)
	}
	if len(a) != 64 {
		t.Errorf("alertID should be 64 hex chars (sha256), got %d: %q", len(a), a)
	}

	c := alertID("amfa-risk-rejected", "tenant-1", "event-2")
	if a == c {
		t.Error("alertID should differ for different event IDs")
	}

	d := alertID("amfa-vpn-risky", "tenant-1", "event-1")
	if a == d {
		t.Error("alertID should differ for different rule prefixes")
	}
}
