package amfamirror

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/amfa"
	"github.com/DefensePoint/keycloak-monitoring/internal/events"
	eventspostgres "github.com/DefensePoint/keycloak-monitoring/internal/events/postgres"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/internal/testdb"
)

type warnLogger struct {
	discardLogger
	mu    sync.Mutex
	warns []string
}

func (l *warnLogger) Warn(msg string, _ ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.warns = append(l.warns, msg)
}

func (l *warnLogger) WithComponent(string) Logger { return l }

func mirrorCycle(t *testing.T, tenantID string) (int, int, []string, error) {
	t.Helper()
	db, recorder := testdb.NewRecorder(t)
	src := &fakeAmfaRepo{rowsByRealm: map[string][]amfa.EventRow{"master": eventRows(3)}}
	log := &warnLogger{}
	svc := NewService(tenantID, realmsFor("master"), src, eventspostgres.NewRepository(db, logger.NewNoop()),
		nil, log, time.Hour, 24*time.Hour)

	err := svc.RunOnce(context.Background())

	var eventInserts, watermarkInserts int
	for _, s := range recorder.Statements() {
		switch {
		case strings.HasPrefix(s.SQL, `INSERT INTO "events"`):
			eventInserts++
		case strings.Contains(s.SQL, "INSERT INTO amfa_mirror_watermarks"):
			watermarkInserts++
		}
	}
	return eventInserts, watermarkInserts, log.warns, err
}

func TestMirrorStopsOnAnEventWithNoTenant(t *testing.T) {
	eventInserts, watermarkInserts, warns, err := mirrorCycle(t, "")

	if !errors.Is(err, events.ErrMissingTenant) {
		t.Fatalf("RunOnce err = %v, want ErrMissingTenant", err)
	}
	if eventInserts != 0 || watermarkInserts != 0 {
		t.Errorf("inserted %d events and %d watermarks, want none", eventInserts, watermarkInserts)
	}
	if len(warns) != 1 || warns[0] != "amfamirror: realm cycle failed" {
		t.Errorf("warnings = %q, want the realm cycle failure", warns)
	}
}

func TestMirrorStoresEventsForATenant(t *testing.T) {
	eventInserts, watermarkInserts, warns, err := mirrorCycle(t, "tenantA")

	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if eventInserts != 3 || watermarkInserts != 1 {
		t.Errorf("inserted %d events and %d watermarks, want 3 and 1", eventInserts, watermarkInserts)
	}
	if len(warns) != 0 {
		t.Errorf("warnings = %q, want none", warns)
	}
}
