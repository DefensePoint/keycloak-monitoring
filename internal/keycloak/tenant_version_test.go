package keycloak

import (
	"context"
	"testing"
	"time"
)

func enteredVersion(version string, setAt *time.Time) TenantVersionLookup {
	return func(context.Context, string) (string, *time.Time, bool) {
		return version, setAt, version != ""
	}
}

func cachedChecker() *VersionChecker {
	return &VersionChecker{
		cache:     &VersionInfo{LatestKeycloakVersion: "26.8.2", KeycloakReleaseURL: "https://example.invalid/26.8.2"},
		cacheTime: time.Now(),
		cacheTTL:  time.Hour,
	}
}

// A Keycloak that does not report its version to a read-only client is
// checked against the version an operator entered, and the result says so.
func TestGetVersionInfo_FallsBackToTheEnteredVersion(t *testing.T) {
	setAt := time.Date(2026, 11, 20, 9, 0, 0, 0, time.UTC)
	tests := []struct {
		name         string
		reported     string
		entered      string
		wantVersion  string
		wantSource   string
		wantOutdated bool
		wantSetAt    bool
	}{
		{"reported only", "26.4.2", "", "26.4.2", VersionSourceKeycloak, true, false},
		{"entered only", "", "26.4.2", "26.4.2", VersionSourceTenantSettings, true, true},
		{"entered and current", "", "26.8.2", "26.8.2", VersionSourceTenantSettings, false, true},
		{"reported wins over entered", "26.8.2", "26.4.2", "26.8.2", VersionSourceKeycloak, false, false},
		{"neither", "", "", "", "", false, false},
	}
	for _, tt := range tests {
		svc := WithTenantVersions(
			NewService(nil, nil, fixedHealthRepo{version: tt.reported}, nil, nil, cachedChecker(), nil),
			enteredVersion(tt.entered, &setAt))
		info, err := svc.GetVersionInfo(context.Background(), "t1")
		if err != nil {
			t.Fatalf("%s: GetVersionInfo: %v", tt.name, err)
		}
		if info.CurrentVersion != tt.wantVersion || info.VersionSource != tt.wantSource || info.IsKeycloakOutdated != tt.wantOutdated {
			t.Errorf("%s: got version %q source %q outdated %v, want %q %q %v", tt.name,
				info.CurrentVersion, info.VersionSource, info.IsKeycloakOutdated, tt.wantVersion, tt.wantSource, tt.wantOutdated)
		}
		if (info.VersionSetAt != nil) != tt.wantSetAt {
			t.Errorf("%s: set at %v, want present %v", tt.name, info.VersionSetAt, tt.wantSetAt)
		}
	}
}

// The dashboard shows the update warning for an entered version too. Before,
// it needed a reported version to show any version information at all.
func TestGetDashboard_ShowsTheUpdateCheckForAnEnteredVersion(t *testing.T) {
	svc := WithTenantVersions(
		NewService(nil, nil, fixedHealthRepo{version: ""}, noRealms{}, nil, cachedChecker(), nil),
		enteredVersion("26.4.2", nil))
	dashboard, err := svc.GetDashboard(context.Background(), "t1", "")
	if err != nil {
		t.Fatalf("GetDashboard: %v", err)
	}
	if dashboard.VersionInfo == nil {
		t.Fatal("no version information for an entered version")
	}
	if !dashboard.VersionInfo.IsKeycloakOutdated || dashboard.VersionInfo.VersionSource != VersionSourceTenantSettings {
		t.Errorf("got %+v, want 26.4.2 from tenant settings, outdated", dashboard.VersionInfo)
	}
}

// Without a lookup, as in every caller that does not wire one, nothing changes.
func TestWithTenantVersions_LeavesOtherServicesAlone(t *testing.T) {
	var other Service
	if got := WithTenantVersions(other, enteredVersion("26.4.2", nil)); got != nil {
		t.Errorf("WithTenantVersions(nil) = %v, want nil", got)
	}
}
