package testdb

import "testing"

func TestDisposableName(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"monitoring", false},
		{"keycloak", false},
		{"adaptive_mfa", false},
		{"latest", false},
		{"contest", false},
		{"kmt_test", true},
		{"amfa_test", true},
		{"test", true},
		{"kmt-test-db", true},
		{"scratch_db", true},
		{"tmp_purge", true},
	}

	for _, tt := range tests {
		if got := disposableName.MatchString(tt.name); got != tt.want {
			t.Errorf("disposableName.MatchString(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}
