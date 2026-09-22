package amfamirror

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/DefensePoint/keycloak-monitoring/amfa"
	"github.com/DefensePoint/keycloak-monitoring/events"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

const (
	// SourceSystem marks mirrored rows in the events table.
	SourceSystem = events.SourceSystemAmfa
	// eventIDPrefix namespaces AMFA event IDs inside the events table's global
	// event_id unique index. Internal only: the UI never displays it.
	eventIDPrefix = "amfa-"
)

// SourceForRealm returns the events-table source value for a realm,
// e.g. "amfa:MyRealm".
func SourceForRealm(realm string) string {
	return events.SourceForAmfaRealm(realm)
}

// ToDomainEvent flattens one AMFA event row into a platform domain.Event.
// AMFA-specific attributes land in the typed nullable fields (risk_level,
// final_status, is_vpn, country, lat, long); the original row is preserved in
// raw_data for the details view. Severity derives from the event type ONLY
// (never from risk level — risk is its own dimension in the UI).
func ToDomainEvent(row amfa.EventRow, tenantID, realm string, user *amfa.KeycloakUser) *domain.Event {
	// amfaEventID is the raw AMFA auth_event.id — the exact value the KMT
	// extension stamps onto the Keycloak event as "amfa_event_id", so it is the
	// merge key between the two rows.
	amfaEventID := row.EventID
	ev := &domain.Event{
		TenantID:         tenantID,
		EventID:          eventIDPrefix + row.EventID,
		Type:             row.EventType,
		Category:         "authentication",
		Severity:         severityForType(row.EventType),
		Description:      descriptionFor(row, realm),
		Source:           SourceForRealm(realm),
		SourceIP:         row.IPAddress,
		SourceSystem:     SourceSystem,
		ClientID:         row.Client,
		Status:           "processed",
		Timestamp:        row.EventTime,
		AMFAEventID:      &amfaEventID,
		RiskLevel:        row.RiskLevel,
		FinalStatus:      row.FinalStatus,
		Country:          row.Country,
		City:             row.City,
		Lat:              row.Lat,
		Long:             row.Long,
		OperatingSystem:  row.OperatingSystem,
		Browser:          row.Browser,
		Device:           row.Device,
		SystemLanguage:   row.SystemLanguage,
		ScreenResolution: row.ScreenResolution,
	}

	isVPN := row.IsVPN
	ev.IsVPN = &isVPN

	if row.UserID != nil {
		ev.UserID = *row.UserID
	}
	if user != nil {
		if user.Username != nil {
			ev.Username = *user.Username
		}
		if user.Email != nil {
			ev.Email = *user.Email
		}
	}
	if row.Country != nil {
		ev.Location = *row.Country
	}

	if raw, err := json.Marshal(rawPayload(row)); err == nil {
		ev.RawData = string(raw)
	}
	return ev
}

// severityForType maps AMFA event types to the platform severity vocabulary.
// Same rule the Keycloak mirror applies: error-type events are "error",
// everything else "info". Risk level deliberately plays no part here.
func severityForType(eventType string) string {
	if strings.HasSuffix(strings.ToUpper(eventType), "_ERROR") {
		return "error"
	}
	return "info"
}

func descriptionFor(row amfa.EventRow, realm string) string {
	return fmt.Sprintf("AMFA %s event in realm %s", row.EventType, realm)
}

// rawPayload is the JSON stored in raw_data: the original AMFA row, keyed with
// the same field names the AMFA API uses.
func rawPayload(row amfa.EventRow) map[string]any {
	return map[string]any{
		"amfa_event_id":     row.EventID,
		"event_time":        row.EventTime,
		"event_type":        row.EventType,
		"user_id":           row.UserID,
		"client":            row.Client,
		"ip_address":        row.IPAddress,
		"country":           row.Country,
		"city":              row.City,
		"lat":               row.Lat,
		"long":              row.Long,
		"is_vpn":            row.IsVPN,
		"risk_level":        row.RiskLevel,
		"final_status":      row.FinalStatus,
		"operating_system":  row.OperatingSystem,
		"browser":           row.Browser,
		"device":            row.Device,
		"system_language":   row.SystemLanguage,
		"screen_resolution": row.ScreenResolution,
	}
}
