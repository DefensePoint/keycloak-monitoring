package fx

import (
	"fmt"
	"strings"
	"testing"
)

// moduleIndex returns where the module named name sits in opts, or -1.
// fx.Module values cannot be compared, so they are told apart by name.
func moduleIndex(t *testing.T, opts []interface{ String() string }, name string) int {
	t.Helper()
	prefix := fmt.Sprintf("fx.Module(%q", name)
	for i, o := range opts {
		if strings.HasPrefix(o.String(), prefix) {
			return i
		}
	}
	t.Fatalf("module %q not in the app", name)
	return -1
}

// fx runs the modules' start hooks in the order the app lists them, and two
// of the tenant checkers depend on that order:
//   - the configuration checks use the Keycloak clients the monitoring start
//     hook creates, so configcheck must come after monitoring;
//   - the event checks load the tenant cache, which must happen before the
//     HTTP server serves tenant edits, so eventscheck must come before server.
func TestNewApp_ModuleOrderTheTenantCheckersRelyOn(t *testing.T) {
	var opts []interface{ String() string }
	for _, o := range appModules("config.yaml") {
		opts = append(opts, o)
	}
	if moduleIndex(t, opts, "configcheck") < moduleIndex(t, opts, "monitoring") {
		t.Error("configcheck starts before monitoring has created the tenants' Keycloak clients")
	}
	if moduleIndex(t, opts, "eventscheck") > moduleIndex(t, opts, "server") {
		t.Error("eventscheck loads the tenant cache after the HTTP server starts serving")
	}
}
