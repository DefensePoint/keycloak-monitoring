package amfamirror

import (
	"testing"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/amfa"
	"github.com/DefensePoint/keycloak-monitoring/internal/events"
)

// The writer half of the source-string contract.
//
// The reader half is asserted by the tenant-scoping tests in
// internal/http/chi and reports, which pin the literal strings they expect to
// match. Nothing asserted what the writers actually stamp, so routing them
// through the shared helpers could have wired the wrong one (amfa where
// keycloak was meant) and the whole suite would still have passed.
func TestToDomainEvent_StampsTheAmfaSource(t *testing.T) {
	row := amfa.EventRow{
		EventID:   "e1",
		EventTime: time.Now().UTC(),
		EventType: "LOGIN",
	}
	ev := ToDomainEvent(row, "tenantA", "MyRealm", nil)

	if got, want := ev.Source, "amfa:MyRealm"; got != want {
		t.Errorf("Source = %q, want %q", got, want)
	}
	if got, want := ev.SourceSystem, "amfa"; got != want {
		t.Errorf("SourceSystem = %q, want %q", got, want)
	}
	// The tenant is the isolation boundary, so an unstamped row would be
	// invisible to every tenant-scoped query and un-purgeable.
	if got, want := ev.TenantID, "tenantA"; got != want {
		t.Errorf("TenantID = %q, want %q", got, want)
	}

	// And the value must be one a reader scoping to this realm will match,
	// which is the property that actually keeps tenant scoping working.
	var matched bool
	for _, s := range events.SourcesForRealm("MyRealm") {
		if s == ev.Source {
			matched = true
		}
	}
	if !matched {
		t.Errorf("Source %q is not in SourcesForRealm(%q) = %v; a tenant-scoped "+
			"query would silently exclude these rows",
			ev.Source, "MyRealm", events.SourcesForRealm("MyRealm"))
	}
}
