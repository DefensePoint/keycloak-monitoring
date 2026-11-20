package domain

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// A realm the monitoring client has never been able to read has no last read
// time. Sent as Go's zero time, the pages showed it as read in the year 1.
func TestKeycloakRealmInfoJSON_LastChecked(t *testing.T) {
	never, err := json.Marshal(KeycloakRealmInfo{RealmName: "gamma"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(never), "last_checked") {
		t.Errorf("never read: %s, want no last_checked", never)
	}

	read, err := json.Marshal(KeycloakRealmInfo{RealmName: "gamma", LastChecked: time.Date(2026, 11, 20, 15, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(read), `"last_checked":"2026-11-20T15:00:00Z"`) {
		t.Errorf("read: %s, want its last_checked", read)
	}
}
