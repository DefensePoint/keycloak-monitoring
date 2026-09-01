package reports

import (
	"context"
	"io"
	"reflect"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"repos.defensepoint.com/iam/oss-pmt/kmt/internal/domain"
	"repos.defensepoint.com/iam/oss-pmt/kmt/internal/logger"
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

func req() *GenerateRequest {
	return &GenerateRequest{
		TenantID:  "tenant-a",
		StartDate: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		EndDate:   time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC),
	}
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
