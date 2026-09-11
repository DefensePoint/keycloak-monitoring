package version

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestGetPublic_HidesSensitiveFields ensures the unauthenticated version
// payload never leaks build metadata that helps attackers fingerprint the
// deployment (Aikido pentest 2026-02 finding #3).
func TestGetPublic_HidesSensitiveFields(t *testing.T) {
	GitCommit = "abcdef1234567890"
	BuildDate = "2026-01-01T00:00:00Z"
	t.Cleanup(func() {
		GitCommit = "unknown"
		BuildDate = "unknown"
	})

	body, err := json.Marshal(GetPublic())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	for _, leak := range []string{
		GitCommit,
		BuildDate,
		"git_commit",
		"build_date",
		"go_version",
		"platform",
	} {
		if strings.Contains(string(body), leak) {
			t.Errorf("public version response leaks %q: %s", leak, body)
		}
	}
}
