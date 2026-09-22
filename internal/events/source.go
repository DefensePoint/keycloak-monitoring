package events

// The events table identifies where a row came from with two columns:
// source_system names the producer ("keycloak", "amfa") and source narrows it
// to one realm ("keycloak:MyRealm", "amfa:MyRealm").
//
// That format is a contract between the writers that stamp it and the readers
// that filter on it, and it used to be hand-built in five separate places: the
// Keycloak monitor and the AMFA mirror produced it, while the tenant events
// handler, the stats handler and the report service each parsed it back to
// scope a query to one tenant's realms. Three of those readers exist
// specifically to stop one tenant seeing another's events.
//
// The failure mode of that duplication is silent. A changed prefix leaves the
// writers writing and the readers matching nothing, so the Events page and
// reports go empty rather than erroring, and the tenant scoping quietly stops
// filtering. Everything now goes through the helpers below so a change lands in
// one place.
const (
	// SourceSystemKeycloak is the source_system value for rows mirrored from a
	// Keycloak event log.
	SourceSystemKeycloak = "keycloak"
	// SourceSystemAmfa is the source_system value for rows mirrored from an
	// AMFA (adaptive authentication) database.
	SourceSystemAmfa = "amfa"

	// sourceSeparator divides the source system from the realm name. Realm
	// names cannot contain it: Keycloak forbids "/" and ":" in realm names, and
	// the AMFA client rejects a realm carrying a path separator before it ever
	// builds a request.
	sourceSeparator = ":"
)

// SourceForKeycloakRealm returns the source value for Keycloak events in a
// realm, e.g. "keycloak:MyRealm". Used by the writer that stamps the row.
func SourceForKeycloakRealm(realm string) string {
	return SourceSystemKeycloak + sourceSeparator + realm
}

// SourceForAmfaRealm returns the source value for AMFA events in a realm, e.g.
// "amfa:MyRealm". Used by the writer that stamps the row.
func SourceForAmfaRealm(realm string) string {
	return SourceSystemAmfa + sourceSeparator + realm
}

// SourcesForRealm returns every source value a single realm can produce.
//
// This is the reader's counterpart: callers scoping a query to a tenant expand
// each of the tenant's realms through this and pass the result as
// ListOptions.Sources, which matches exactly rather than by substring. Adding a
// new source system means extending this function, and every tenant-scoped
// query picks it up instead of silently excluding the new rows.
func SourcesForRealm(realm string) []string {
	return []string{
		SourceForKeycloakRealm(realm),
		SourceForAmfaRealm(realm),
	}
}
