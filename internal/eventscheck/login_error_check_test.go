package eventscheck

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

type fakeEventReader struct {
	count int64
	err   error
	calls int
}

func (f *fakeEventReader) CountEventsByType(_ context.Context, _, _, _ string, _, _ time.Time) (int64, error) {
	f.calls++
	return f.count, f.err
}

func staticRealms(names ...string) RealmsFunc {
	copied := append([]string(nil), names...)
	return func(context.Context) ([]string, error) { return copied, nil }
}

func TestLoginErrorCheck_AlertWhenCountAtThreshold(t *testing.T) {
	reader := &fakeEventReader{count: 10}
	c := NewLoginErrorCheck(reader, discardLogger{}, "tenant-1", staticRealms("master"), 10, 5*time.Minute)

	alerts, err := c.Execute(context.Background())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}
	a := alerts[0]
	if a.Severity != domain.AlertSeverityWarning {
		t.Errorf("severity = %q, want %q", a.Severity, domain.AlertSeverityWarning)
	}
	if a.CheckType != "login_error_events" {
		t.Errorf("check_type = %q, want login_error_events", a.CheckType)
	}
	if a.RealmName != "master" {
		t.Errorf("realm = %q, want master", a.RealmName)
	}
	if a.TenantID != "tenant-1" {
		t.Errorf("tenant = %q, want tenant-1", a.TenantID)
	}
}

func TestLoginErrorCheck_NoAlertBelowThreshold(t *testing.T) {
	reader := &fakeEventReader{count: 9}
	c := NewLoginErrorCheck(reader, discardLogger{}, "tenant-1", staticRealms("master"), 10, 5*time.Minute)
	alerts, err := c.Execute(context.Background())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(alerts) != 0 {
		t.Errorf("expected 0 alerts below threshold, got %d", len(alerts))
	}
}

func TestLoginErrorCheck_AlertIDStable(t *testing.T) {
	reader := &fakeEventReader{count: 10}
	c := NewLoginErrorCheck(reader, discardLogger{}, "tenant-1", staticRealms("master"), 10, 5*time.Minute)

	a1, err := c.Execute(context.Background())
	if err != nil {
		t.Fatalf("Execute(1): %v", err)
	}
	a2, err := c.Execute(context.Background())
	if err != nil {
		t.Fatalf("Execute(2): %v", err)
	}
	if a1[0].AlertID != a2[0].AlertID {
		t.Errorf("AlertID not stable: %q != %q", a1[0].AlertID, a2[0].AlertID)
	}
}

func TestLoginErrorCheck_UsesRealmsFnEachTick(t *testing.T) {
	reader := &fakeEventReader{count: 10}
	calls := 0
	realmsFn := RealmsFunc(func(context.Context) ([]string, error) {
		calls++
		if calls == 1 {
			return []string{"master"}, nil
		}
		return []string{"master", "staff"}, nil
	})
	c := NewLoginErrorCheck(reader, discardLogger{}, "tenant-1", realmsFn, 10, 5*time.Minute)

	first, err := c.Execute(context.Background())
	if err != nil {
		t.Fatalf("Execute(1): %v", err)
	}
	if len(first) != 1 {
		t.Fatalf("first tick: expected 1 alert, got %d", len(first))
	}

	second, err := c.Execute(context.Background())
	if err != nil {
		t.Fatalf("Execute(2): %v", err)
	}
	if len(second) != 2 {
		t.Fatalf("second tick: expected 2 alerts after realm added, got %d", len(second))
	}
}

func TestLoginErrorCheck_RealmsFnError(t *testing.T) {
	reader := &fakeEventReader{count: 10}
	realmsFn := RealmsFunc(func(context.Context) ([]string, error) {
		return nil, errors.New("keycloak down")
	})
	c := NewLoginErrorCheck(reader, discardLogger{}, "tenant-1", realmsFn, 10, 5*time.Minute)
	_, err := c.Execute(context.Background())
	if err == nil {
		t.Fatal("expected error when realmsFn fails")
	}
	if reader.calls != 0 {
		t.Errorf("CountEventsByType called %d times, want 0 when realmsFn fails", reader.calls)
	}
}
