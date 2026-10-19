package keycloak

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/rs/zerolog"

	"github.com/DefensePoint/keycloak-monitoring/internal/events"
	eventspostgres "github.com/DefensePoint/keycloak-monitoring/internal/events/postgres"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/internal/testdb"
	"github.com/DefensePoint/keycloak-monitoring/pkg/keycloakadmin"
)

func eventsTableCycle(t *testing.T, tenantID string) (EventStorageOutcome, map[string]int, int, string) {
	t.Helper()
	db, recorder := testdb.NewRecorder(t)
	api := newMockAdminAPI()
	api.EventsFunc = func(context.Context, string) ([]*keycloakadmin.EventRepresentation, error) {
		return []*keycloakadmin.EventRepresentation{
			{ID: "evt-1", Time: 1, Type: "LOGIN", RealmID: "realm-1"},
			{ID: "evt-2", Time: 2, Type: "LOGIN_ERROR", RealmID: "realm-1", Error: "invalid_credentials"},
		}, nil
	}

	var logs bytes.Buffer
	reporter := &capturedReporter{}
	failed := &failedWritesRecorder{failed: map[string]int{}}
	m := NewMonitor(tenantID, "Test Tenant", api, newTestConfig(), logger.New(zerolog.New(&logs)),
		newMockEventRepository(), eventspostgres.NewRepository(db, logger.NewNoop()),
		newMockMetricsRepository(), newMockHealthRepository(), newMockRealmRepository(), nil, nil,
		WithEventStorageReporter(reporter))
	m.SetMetrics(failed)

	m.collectEvents(context.Background())
	if len(reporter.outcomes) != 1 {
		t.Fatalf("reports = %d, want one per cycle", len(reporter.outcomes))
	}
	inserts := 0
	for _, s := range recorder.Against("events") {
		if strings.HasPrefix(s.SQL, "INSERT") {
			inserts++
		}
	}
	return reporter.outcomes[0], failed.failed, inserts, logs.String()
}

func TestMonitorCountsEventsRefusedForNoTenant(t *testing.T) {
	o, failed, inserts, logs := eventsTableCycle(t, "")

	if inserts != 0 {
		t.Errorf("inserted %d rows into events, want none", inserts)
	}
	if o.Failed != 2 || !errors.Is(o.FirstError, events.ErrMissingTenant) {
		t.Errorf("outcome = %+v, want 2 failed with ErrMissingTenant", o)
	}
	if failed["events"] != 2 {
		t.Errorf("failed writes = %v, want 2 in events", failed)
	}
	if !strings.Contains(logs, "Failed to save events to the events table") {
		t.Errorf("no error summary logged:\n%s", logs)
	}
}

func TestMonitorStoresEventsForATenant(t *testing.T) {
	o, failed, inserts, _ := eventsTableCycle(t, "tenant-1")

	if inserts != 2 {
		t.Errorf("inserted %d rows into events, want 2", inserts)
	}
	if o.Failed != 0 || o.FirstError != nil {
		t.Errorf("outcome = %+v, want nothing failed", o)
	}
	if failed["events"] != 0 {
		t.Errorf("failed writes = %v, want none", failed)
	}
}
