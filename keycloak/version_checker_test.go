package keycloak

import "testing"

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		name     string
		v1       string
		v2       string
		expected int
	}{
		{
			name:     "v1 greater than v2",
			v1:       "25.0.2",
			v2:       "25.0.1",
			expected: 1,
		},
		{
			name:     "v1 less than v2",
			v1:       "25.0.1",
			v2:       "25.0.2",
			expected: -1,
		},
		{
			name:     "v1 equals v2",
			v1:       "25.0.1",
			v2:       "25.0.1",
			expected: 0,
		},
		{
			name:     "Major version difference",
			v1:       "26.0.0",
			v2:       "25.0.1",
			expected: 1,
		},
		{
			name:     "Minor version difference",
			v1:       "25.1.0",
			v2:       "25.0.1",
			expected: 1,
		},
		{
			name:     "With v prefix",
			v1:       "v25.0.1",
			v2:       "v25.0.1",
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CompareVersions(tt.v1, tt.v2)
			if result != tt.expected {
				t.Errorf("CompareVersions(%q, %q) = %d, want %d", tt.v1, tt.v2, result, tt.expected)
			}
		})
	}
}

func TestIsVersionOutdated(t *testing.T) {
	tests := []struct {
		name           string
		currentVersion string
		latestVersion  string
		expected       bool
	}{
		{
			name:           "Current is outdated",
			currentVersion: "25.0.1",
			latestVersion:  "25.0.2",
			expected:       true,
		},
		{
			name:           "Current is up to date",
			currentVersion: "25.0.2",
			latestVersion:  "25.0.2",
			expected:       false,
		},
		{
			name:           "Current is newer",
			currentVersion: "25.0.3",
			latestVersion:  "25.0.2",
			expected:       false,
		},
		{
			name:           "Patch version behind",
			currentVersion: "25.0.1",
			latestVersion:  "25.0.2",
			expected:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsVersionOutdated(tt.currentVersion, tt.latestVersion)
			if result != tt.expected {
				t.Errorf("IsVersionOutdated(%q, %q) = %v, want %v",
					tt.currentVersion, tt.latestVersion, result, tt.expected)
			}
		})
	}
}
