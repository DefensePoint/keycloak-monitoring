package alertscleanup

import (
	"database/sql"
	"fmt"
)

// HealthMarker is written into keycloak_health.server_version so cleanup can
// find the row. It is a version-shaped string: product code feeds this column
// into version comparison, and a non-version marker shows up as a bogus
// Keycloak version until the row is removed.
const HealthMarker = "0.0.0-e2e-fixture"

const (
	eventsSQL = `
		DELETE FROM keycloak_events
		WHERE tenant_id = $1
		AND event_id LIKE $2`
	healthSQL = `
		DELETE FROM keycloak_health
		WHERE tenant_id = $1
		AND server_version = $2`
)

// Cleanup deletes only the events from this run and the health row this test
// stamped. It does not touch configuration_alerts: the test never inserts
// those rows, alert_id is a stable hash the checker upserts, and a hard
// delete would remove a real tenant's alert plus its operator_actions history.
func Cleanup(db *sql.DB, tenantID, eventLike string) error {
	if _, err := db.Exec(eventsSQL, tenantID, eventLike); err != nil {
		return fmt.Errorf("cleanup events: %w", err)
	}
	if _, err := db.Exec(healthSQL, tenantID, HealthMarker); err != nil {
		return fmt.Errorf("cleanup health: %w", err)
	}
	return nil
}
