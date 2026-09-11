package reports

import (
	"context"
	"io"
	"reflect"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
)

func testLogger() *logger.Logger { return logger.New(zerolog.New(io.Discard)) }

type mockEventLister struct {
	called     bool
	gotSources []string
	events     []*domain.Event
}

func (m *mockEventLister) List(_ context.Context, opts *EventListOptions) ([]*domain.Event, error) {
	m.called = true
	m.gotSources = opts.Sources
	return m.events, nil
}

type mockRealmLister struct {
	realms []*domain.KeycloakRealmInfo
	err    error
}

func (m *mockRealmLister) ListRealms(_ context.Context, _ string) ([]*domain.KeycloakRealmInfo, error) {
	return m.realms, m.err
}

func realmInfos(names ...string) []*domain.KeycloakRealmInfo {
	out := make([]*domain.KeycloakRealmInfo, 0, len(names))
	for _, n := range names {
		out = append(out, &domain.KeycloakRealmInfo{RealmName: n})
	}
	return out
}

// req builds a request for an unrestricted caller.
func req() *GenerateRequest {
	return reqScoped(RealmScope{All: true})
}

func reqScoped(scope RealmScope) *GenerateRequest {
	return &GenerateRequest{
		TenantID:  "tenant-a",
		StartDate: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		EndDate:   time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC),
		Scope:     scope,
	}
}

type mockAlertReader struct {
	called     bool
	gotRealms  []string
	alertsList []*domain.Alert
}

func (m *mockAlertReader) ListActiveAlerts(_ context.Context, _ string, opts *AlertListOptions) ([]*domain.Alert, error) {
	m.called = true
	m.gotRealms = opts.RealmNames
	return m.alertsList, nil
}

type mockOperatorReader struct {
	called    bool
	gotRealms []string
	summaries []*domain.OperatorMetricsSummary
}

func (m *mockOperatorReader) GetAllOperatorsSummary(_ context.Context, _ string, _, _ time.Time, realmNames []string) ([]*domain.OperatorMetricsSummary, error) {
	m.called = true
	m.gotRealms = realmNames
	return m.summaries, nil
}

func TestAggregateData_ScopesEventsToTenantRealms(t *testing.T) {
	el := &mockEventLister{events: []*domain.Event{{Type: "LOGIN", Source: "keycloak:realmA"}}}
	rl := &mockRealmLister{realms: realmInfos("realmA", "realmB")}
	svc := NewService(el, nil, nil, rl, testLogger(), nil)

	if _, err := svc.AggregateData(context.Background(), req()); err != nil {
		t.Fatalf("AggregateData() error = %v", err)
	}
	if !el.called {
		t.Fatal("event lister was not called")
	}
	want := []string{"keycloak:realmA", "amfa:realmA", "keycloak:realmB", "amfa:realmB"}
	if !reflect.DeepEqual(el.gotSources, want) {
		t.Errorf("event query sources = %v, want %v", el.gotSources, want)
	}
}

func TestAggregateData_NoRealmsDeniesEvents(t *testing.T) {
	el := &mockEventLister{events: []*domain.Event{{Type: "LOGIN", Source: "keycloak:master"}}}
	rl := &mockRealmLister{realms: nil}
	svc := NewService(el, nil, nil, rl, testLogger(), nil)

	data, err := svc.AggregateData(context.Background(), req())
	if err != nil {
		t.Fatalf("AggregateData() error = %v", err)
	}
	if el.called {
		t.Error("event lister must not run an unscoped query when the tenant has no sources")
	}
	if data.TotalEvents != 0 {
		t.Errorf("TotalEvents = %d, want 0", data.TotalEvents)
	}
}

func TestAggregateData_NilRealmListerDeniesEvents(t *testing.T) {
	el := &mockEventLister{events: []*domain.Event{{Type: "LOGIN", Source: "keycloak:master"}}}
	svc := NewService(el, nil, nil, nil, testLogger(), nil)

	if _, err := svc.AggregateData(context.Background(), req()); err != nil {
		t.Fatalf("AggregateData() error = %v", err)
	}
	if el.called {
		t.Error("event lister must not run when no realm lister is configured")
	}
}

func TestAggregateData_RealmScope(t *testing.T) {
	tests := []struct {
		name        string
		scope       RealmScope
		wantSources []string
		wantRealms  []string
		wantQueries bool
	}{
		{
			name:        "unrestricted caller covers the whole tenant",
			scope:       RealmScope{All: true},
			wantSources: []string{"keycloak:realmA", "amfa:realmA", "keycloak:realmB", "amfa:realmB"},
			wantQueries: true,
		},
		{
			name:        "caller restricted to one realm covers only it",
			scope:       RealmScope{Realms: []string{"realmB"}},
			wantSources: []string{"keycloak:realmB", "amfa:realmB"},
			wantRealms:  []string{"realmB"},
			wantQueries: true,
		},
		{
			name:  "caller whose scope names no realm covers nothing",
			scope: RealmScope{},
		},
		{
			name:  "realm outside the tenant adds nothing",
			scope: RealmScope{Realms: []string{"realmZ"}},
			// The queries still run, filtered to a realm the tenant does not
			// have, so they return no rows rather than every row.
			wantRealms:  []string{"realmZ"},
			wantQueries: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			el := &mockEventLister{events: []*domain.Event{{Type: "LOGIN", Source: "keycloak:realmA"}}}
			ar := &mockAlertReader{alertsList: []*domain.Alert{{RealmName: "realmA"}}}
			or := &mockOperatorReader{summaries: []*domain.OperatorMetricsSummary{
				{OperatorEmail: "operator@example.com", TotalAlertsHandled: 42, TotalWorkTimeHours: 7},
			}}
			rl := &mockRealmLister{realms: realmInfos("realmA", "realmB")}
			svc := NewService(el, ar, or, rl, testLogger(), nil)

			data, err := svc.AggregateData(context.Background(), reqScoped(tt.scope))
			if err != nil {
				t.Fatalf("AggregateData() error = %v", err)
			}

			if ar.called != tt.wantQueries {
				t.Errorf("alert reader called = %v, want %v", ar.called, tt.wantQueries)
			}
			if tt.wantQueries && !reflect.DeepEqual(ar.gotRealms, tt.wantRealms) {
				t.Errorf("alert query realms = %v, want %v", ar.gotRealms, tt.wantRealms)
			}
			if or.called != tt.wantQueries {
				t.Errorf("operator reader called = %v, want %v", or.called, tt.wantQueries)
			}
			if tt.wantQueries && !reflect.DeepEqual(or.gotRealms, tt.wantRealms) {
				t.Errorf("operator query realms = %v, want %v", or.gotRealms, tt.wantRealms)
			}
			if !tt.wantQueries && (data.TotalOperators != 0 || len(data.OperatorMetrics) != 0) {
				t.Errorf("report carries %d operators for a scope covering no realm", data.TotalOperators)
			}
			if len(tt.wantSources) == 0 {
				if el.called {
					t.Errorf("event lister ran for a scope covering no realm (sources = %v)", el.gotSources)
				}
				return
			}
			if !reflect.DeepEqual(el.gotSources, tt.wantSources) {
				t.Errorf("event query sources = %v, want %v", el.gotSources, tt.wantSources)
			}
		})
	}
}
