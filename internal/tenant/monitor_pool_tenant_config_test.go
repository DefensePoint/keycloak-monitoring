package tenant

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

type globalSettings struct{ name string }

func overriddenTenant() *domain.KeycloakTenant {
	return &domain.KeycloakTenant{
		TenantID:      "t1",
		Name:          "Tenant One",
		ServerURL:     "https://keycloak.example.com",
		AdminRealm:    "master",
		ClientID:      "monitoring-service",
		Enabled:       true,
		Configuration: `{"polling":{"health_interval":"20s"}}`,
	}
}

// The factory is the only side that knows the config type, so it merges; the
// pool hands the one merged result to both the client and the monitor.
func TestStartMonitor_PassesTheFactoryMergedConfigToClientAndMonitor(t *testing.T) {
	global := &globalSettings{name: "global"}
	merged := &globalSettings{name: "merged"}
	factory := &mockMonitorFactory{resolved: merged}
	manager := NewMonitorPoolManager(&mockService{}, factory, global, &mockLogger{}, nil, nil, nil, nil, nil, nil, nil)

	if err := manager.startMonitorForTenant(context.Background(), overriddenTenant()); err != nil {
		t.Fatalf("startMonitorForTenant: %v", err)
	}

	if factory.resolveCalls != 1 || factory.resolvedFrom != overriddenTenant().Configuration {
		t.Errorf("factory asked %d times with %q, want once with the tenant's configuration", factory.resolveCalls, factory.resolvedFrom)
	}
	if factory.clientInstance != merged {
		t.Errorf("client got %#v, want the merged config", factory.clientInstance)
	}
	if factory.monitorInstance != merged {
		t.Errorf("monitor got %#v, want the merged config", factory.monitorInstance)
	}
}

// An override the factory cannot use must not leave the tenant on built-in
// defaults, and must be reported.
func TestStartMonitor_UnusableOverrideKeepsTheGlobalConfigAndWarns(t *testing.T) {
	global := &globalSettings{name: "global"}
	factory := &mockMonitorFactory{resolveErr: errors.New("invalid JSON")}
	log := &recordingLogger{}
	manager := NewMonitorPoolManager(&mockService{}, factory, global, log, nil, nil, nil, nil, nil, nil, nil)

	if err := manager.startMonitorForTenant(context.Background(), overriddenTenant()); err != nil {
		t.Fatalf("startMonitorForTenant: %v", err)
	}

	if factory.clientInstance != global || factory.monitorInstance != global {
		t.Errorf("client got %#v and monitor %#v, want the global config", factory.clientInstance, factory.monitorInstance)
	}
	warned := false
	for _, e := range log.entries {
		id, _ := e.field("tenant_id")
		errText, _ := e.field("error")
		if e.level == "warn" && id == "t1" && strings.Contains(fmt.Sprint(errText), "invalid JSON") {
			warned = true
		}
	}
	if !warned {
		t.Errorf("no warning naming the tenant and the error: %+v", log.entries)
	}
}

func TestStartMonitor_NoOverrideUsesTheGlobalConfigAsIs(t *testing.T) {
	global := &globalSettings{name: "global"}
	factory := &mockMonitorFactory{}
	manager := NewMonitorPoolManager(&mockService{}, factory, global, &mockLogger{}, nil, nil, nil, nil, nil, nil, nil)

	tenant := overriddenTenant()
	tenant.Configuration = ""
	if err := manager.startMonitorForTenant(context.Background(), tenant); err != nil {
		t.Fatalf("startMonitorForTenant: %v", err)
	}

	if factory.resolveCalls != 0 {
		t.Errorf("factory asked to merge %d times for a tenant with no override", factory.resolveCalls)
	}
	if factory.clientInstance != global || factory.monitorInstance != global {
		t.Errorf("client got %#v and monitor %#v, want the global config", factory.clientInstance, factory.monitorInstance)
	}
}
