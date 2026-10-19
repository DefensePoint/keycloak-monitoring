package postgres

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/events"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/internal/testdb"
)

func newGuardTestRepo(t *testing.T) (*Repository, *testdb.Recorder, *bytes.Buffer) {
	t.Helper()
	db, recorder := testdb.NewRecorder(t)
	var buf bytes.Buffer
	return &Repository{db: db, logger: logger.New(zerolog.New(&buf))}, recorder, &buf
}

func eventWriters(repo *Repository) map[string]func(context.Context, *domain.Event) error {
	return map[string]func(context.Context, *domain.Event) error{
		"Save":               repo.Save,
		"MergeKeycloakEvent": repo.MergeKeycloakEvent,
		"MergeAmfaEvent":     repo.MergeAmfaEvent,
	}
}

func keycloakEvent(tenantID, realm string) *domain.Event {
	return &domain.Event{
		TenantID: tenantID, EventID: "kc-1", Type: "LOGIN",
		Source: events.SourceForKeycloakRealm(realm), SourceSystem: events.SourceSystemKeycloak,
		Timestamp: time.Now(),
	}
}

// logLevels returns the level of every JSON log line in buf.
func logLevels(t *testing.T, buf *bytes.Buffer) []string {
	t.Helper()
	var levels []string
	for _, raw := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if raw == "" {
			continue
		}
		var line map[string]any
		if err := json.Unmarshal([]byte(raw), &line); err != nil {
			t.Fatalf("log line %q: %v", raw, err)
		}
		levels = append(levels, line["level"].(string))
	}
	return levels
}

func TestSaveRefusesAnEventWithNoTenant(t *testing.T) {
	for _, name := range []string{"Save", "MergeKeycloakEvent", "MergeAmfaEvent"} {
		t.Run(name, func(t *testing.T) {
			repo, recorder, buf := newGuardTestRepo(t)

			err := eventWriters(repo)[name](context.Background(), keycloakEvent("", "master"))
			if !errors.Is(err, events.ErrMissingTenant) {
				t.Fatalf("err = %v, want ErrMissingTenant", err)
			}
			if got := recorder.Against("events"); len(got) != 0 {
				t.Errorf("issued %d statements against events, want none", len(got))
			}

			var line map[string]any
			if err := json.Unmarshal(buf.Bytes(), &line); err != nil {
				t.Fatalf("want one JSON log line, got %q: %v", buf.String(), err)
			}
			if line["level"] != "error" || line["event_id"] != "kc-1" ||
				line["source"] != "keycloak:master" || line["source_system"] != "keycloak" {
				t.Errorf("log line = %v", line)
			}
		})
	}
}

// The pollers retry a refused event on every cycle, so only the first refusal
// from a source is an Error.
func TestSaveLogsTheFirstRefusalFromEachSourceAtError(t *testing.T) {
	repo, _, buf := newGuardTestRepo(t)
	ctx := context.Background()

	for _, realm := range []string{"master", "master", "demo"} {
		if err := repo.Save(ctx, keycloakEvent("", realm)); !errors.Is(err, events.ErrMissingTenant) {
			t.Fatalf("Save(%s) err = %v, want ErrMissingTenant", realm, err)
		}
	}

	if got, want := logLevels(t, buf), []string{"error", "debug", "error"}; !slices.Equal(got, want) {
		t.Errorf("log levels = %v, want %v", got, want)
	}
}

func TestSaveWritesAnEventWithATenant(t *testing.T) {
	for _, name := range []string{"Save", "MergeKeycloakEvent", "MergeAmfaEvent"} {
		t.Run(name, func(t *testing.T) {
			repo, recorder, buf := newGuardTestRepo(t)

			if err := eventWriters(repo)[name](context.Background(), keycloakEvent("tenant-a", "master")); err != nil {
				t.Fatalf("write: %v", err)
			}
			got := recorder.Against("events")
			if len(got) != 1 || !strings.HasPrefix(got[0].SQL, "INSERT") {
				t.Fatalf("statements = %v, want one INSERT", got)
			}
			if !slices.Contains(got[0].Args, any("tenant-a")) {
				t.Errorf("insert binds %v, want tenant-a among them", got[0].Args)
			}
			if buf.Len() != 0 {
				t.Errorf("logged %q for a normal write", buf.String())
			}
		})
	}
}

func TestSaveOfANilEventFailsWithoutPanicking(t *testing.T) {
	repo, recorder, _ := newGuardTestRepo(t)

	err := repo.Save(context.Background(), nil)
	if err == nil || errors.Is(err, events.ErrMissingTenant) {
		t.Fatalf("err = %v, want a non-tenant error", err)
	}
	if got := recorder.Against("events"); len(got) != 0 {
		t.Errorf("issued %d statements against events, want none", len(got))
	}
}
