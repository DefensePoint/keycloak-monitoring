package keycloak

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// The dashboard's realm list leaves out the last read time of a realm never
// read, like the realm itself does.
func TestRealmSummaryJSON_LastChecked(t *testing.T) {
	never, err := json.Marshal(RealmSummary{RealmName: "gamma"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(never), "last_checked") {
		t.Errorf("never read: %s, want no last_checked", never)
	}

	read, err := json.Marshal(RealmSummary{RealmName: "gamma", LastChecked: time.Date(2026, 11, 20, 15, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(read), `"last_checked":"2026-11-20T15:00:00Z"`) {
		t.Errorf("read: %s, want its last_checked", read)
	}
}
