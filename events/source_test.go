package events

import (
	"slices"
	"testing"
)

func TestSourceForRealm_Format(t *testing.T) {
	if got, want := SourceForKeycloakRealm("MyRealm"), "keycloak:MyRealm"; got != want {
		t.Errorf("SourceForKeycloakRealm = %q, want %q", got, want)
	}
	if got, want := SourceForAmfaRealm("MyRealm"), "amfa:MyRealm"; got != want {
		t.Errorf("SourceForAmfaRealm = %q, want %q", got, want)
	}
}

// The invariant that actually matters, and the one whose violation would be
// silent: every source a writer can stamp for a realm must appear in the set
// readers use to scope a query to that realm. If a writer gains a source the
// reader set does not list, tenant-scoped queries quietly exclude those rows
// instead of failing, so the Events page and reports show nothing and nobody
// finds out.
func TestSourcesForRealm_CoversEveryWriterSource(t *testing.T) {
	const realm = "MyRealm"
	readerSet := SourcesForRealm(realm)

	writerSources := map[string]string{
		"keycloak monitor": SourceForKeycloakRealm(realm),
		"amfa mirror":      SourceForAmfaRealm(realm),
	}

	for who, produced := range writerSources {
		if !slices.Contains(readerSet, produced) {
			t.Errorf("the %s writes source %q, which SourcesForRealm does not list (%v); "+
				"tenant-scoped queries would silently exclude those rows",
				who, produced, readerSet)
		}
	}

	if len(readerSet) != len(writerSources) {
		t.Errorf("SourcesForRealm returned %d sources for %d known writers (%v); "+
			"a stale entry matches nothing, a missing one drops rows",
			len(readerSet), len(writerSources), readerSet)
	}
}

// Realm names are used verbatim, so a realm containing the separator would
// produce an ambiguous source. Keycloak forbids ":" and "/" in realm names and
// the AMFA client rejects path separators before building a request, so this
// pins the assumption rather than defending against it.
func TestSourcesForRealm_UsesRealmVerbatim(t *testing.T) {
	got := SourcesForRealm("realm-with-dashes_and_underscores")
	want := []string{
		"keycloak:realm-with-dashes_and_underscores",
		"amfa:realm-with-dashes_and_underscores",
	}
	if !slices.Equal(got, want) {
		t.Errorf("SourcesForRealm = %v, want %v", got, want)
	}
}

// Sources are matched with SQL IN (exact equality), never LIKE, so an empty
// realm must not produce a value that could match unrelated rows.
func TestSourcesForRealm_EmptyRealmIsStillExact(t *testing.T) {
	for _, s := range SourcesForRealm("") {
		if s != "keycloak:" && s != "amfa:" {
			t.Errorf("unexpected source %q for an empty realm", s)
		}
	}
}
