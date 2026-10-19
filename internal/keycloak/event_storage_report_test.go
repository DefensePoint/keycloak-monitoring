package keycloak

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/metrics"
	"github.com/DefensePoint/keycloak-monitoring/pkg/keycloakadmin"
)

// Each events cycle reports how storing the polled events went, and counts
// the events that could not be stored, so a tenant losing its events is seen
// beyond a log line.

type capturedReporter struct {
	mu       sync.Mutex
	outcomes []EventStorageOutcome
}

func (r *capturedReporter) ReportEventStorage(_ context.Context, _, _ string, o EventStorageOutcome) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.outcomes = append(r.outcomes, o)
}

type failedWritesRecorder struct {
	metrics.NopRecorder
	mu     sync.Mutex
	failed map[string]int // table -> events
}

func (r *failedWritesRecorder) AddEventWritesFailed(_, table string, n int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.failed[table] += n
}

func storageCycle(t *testing.T, setup func(api *mockAdminAPI, kc *mockEventRepository, general *mockGeneralEventRepository)) (EventStorageOutcome, map[string]int) {
	t.Helper()
	api, kc, general := newMockAdminAPI(), newMockEventRepository(), newMockGeneralEventRepository()
	api.EventsFunc = func(context.Context, string) ([]*keycloakadmin.EventRepresentation, error) {
		return []*keycloakadmin.EventRepresentation{
			{ID: "evt-1", Time: 1, Type: "LOGIN", RealmID: "realm-1"},
			{ID: "evt-2", Time: 2, Type: "LOGIN_ERROR", RealmID: "realm-1", Error: "invalid_credentials"},
		}, nil
	}
	setup(api, kc, general)

	reporter := &capturedReporter{}
	recorder := &failedWritesRecorder{failed: map[string]int{}}
	m := NewMonitor("tenant-1", "Test Tenant", api, newTestConfig(), newTestLogger(), kc, general,
		newMockMetricsRepository(), newMockHealthRepository(), newMockRealmRepository(), nil, nil,
		WithEventStorageReporter(reporter))
	m.SetMetrics(recorder)

	m.collectEvents(context.Background())
	if len(reporter.outcomes) != 1 {
		t.Fatalf("reports = %d, want one per cycle", len(reporter.outcomes))
	}
	return reporter.outcomes[0], recorder.failed
}

func TestEventStorageReport(t *testing.T) {
	writeErr := errors.New("duplicate key value violates unique constraint")

	t.Run("everything stored", func(t *testing.T) {
		o, failed := storageCycle(t, func(*mockAdminAPI, *mockEventRepository, *mockGeneralEventRepository) {})
		if o.Attempted != 4 || o.Failed != 0 || o.FirstError != nil {
			t.Errorf("outcome = %+v, want 4 attempted (2 events, 2 tables), none failed", o)
		}
		if len(failed) != 0 {
			t.Errorf("failed writes counted: %v", failed)
		}
	})

	t.Run("the keycloak_events batch fails", func(t *testing.T) {
		o, failed := storageCycle(t, func(_ *mockAdminAPI, kc *mockEventRepository, _ *mockGeneralEventRepository) {
			kc.SaveEventsFunc = func(context.Context, []*domain.KeycloakEvent) error { return writeErr }
		})
		if o.Attempted != 2 || o.Failed != 2 || !errors.Is(o.FirstError, writeErr) {
			t.Errorf("outcome = %+v, want 2 attempted, 2 failed, with the error", o)
		}
		if failed["keycloak_events"] != 2 || failed["events"] != 0 {
			t.Errorf("failed writes = %v, want 2 in keycloak_events", failed)
		}
	})

	t.Run("one write to the events table fails", func(t *testing.T) {
		o, failed := storageCycle(t, func(_ *mockAdminAPI, _ *mockEventRepository, general *mockGeneralEventRepository) {
			general.SaveFunc = func(_ context.Context, e *domain.Event) error {
				if e.EventID == "evt-2" {
					return writeErr
				}
				return nil
			}
		})
		if o.Attempted != 4 || o.Failed != 1 || !errors.Is(o.FirstError, writeErr) {
			t.Errorf("outcome = %+v, want 4 attempted, 1 failed, with the error", o)
		}
		if failed["events"] != 1 || failed["keycloak_events"] != 0 {
			t.Errorf("failed writes = %v, want 1 in events", failed)
		}
	})

	// Keycloak being unreachable is not a storage failure: the health alert
	// covers it.
	t.Run("the fetch fails", func(t *testing.T) {
		o, _ := storageCycle(t, func(api *mockAdminAPI, _ *mockEventRepository, _ *mockGeneralEventRepository) {
			api.EventsFunc = func(context.Context, string) ([]*keycloakadmin.EventRepresentation, error) {
				return nil, errors.New("connection refused")
			}
		})
		if o.Attempted != 0 || o.Failed != 0 {
			t.Errorf("outcome = %+v, want nothing attempted", o)
		}
	})
}
