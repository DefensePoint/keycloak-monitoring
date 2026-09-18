package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/amfa"
)

// eventRowScan is the row shape for the full-EventRow queries below. It mirrors
// the inline scan struct used by ListEvents in repository.go.
type eventRowScan struct {
	EventID          string    `gorm:"column:event_id"`
	EventTime        time.Time `gorm:"column:event_time"`
	EventType        string    `gorm:"column:event_type"`
	UserID           *string   `gorm:"column:user_id"`
	Client           string    `gorm:"column:client"`
	IPAddress        string    `gorm:"column:ip_address"`
	Country          *string   `gorm:"column:country"`
	City             *string   `gorm:"column:city"`
	Lat              *float64  `gorm:"column:lat"`
	Long             *float64  `gorm:"column:long"`
	IsVPN            bool      `gorm:"column:is_vpn"`
	RiskLevel        *int      `gorm:"column:risk_level"`
	FinalStatus      *string   `gorm:"column:final_status"`
	OperatingSystem  *string   `gorm:"column:operating_system"`
	Browser          *string   `gorm:"column:browser"`
	Device           *string   `gorm:"column:device"`
	SystemLanguage   *string   `gorm:"column:system_language"`
	ScreenResolution *string   `gorm:"column:screen_resolution"`
}

func (s eventRowScan) toEventRow() amfa.EventRow {
	return amfa.EventRow{
		EventID:          s.EventID,
		EventTime:        s.EventTime,
		EventType:        s.EventType,
		UserID:           s.UserID,
		Client:           s.Client,
		IPAddress:        s.IPAddress,
		Country:          s.Country,
		City:             s.City,
		Lat:              s.Lat,
		Long:             s.Long,
		IsVPN:            s.IsVPN,
		RiskLevel:        s.RiskLevel,
		FinalStatus:      s.FinalStatus,
		OperatingSystem:  s.OperatingSystem,
		Browser:          s.Browser,
		Device:           s.Device,
		SystemLanguage:   s.SystemLanguage,
		ScreenResolution: s.ScreenResolution,
	}
}

// eventRowSelect is the shared SELECT/JOIN prefix used by the per-event queries.
// It is identical to the projection in ListEvents, including the COALESCE
// fallback from the (unpopulated) auth_context table to auth_context_json for
// client/IP/country/geo/VPN. See ListEvents for why the fallback is required.
const eventRowSelect = `
        SELECT
            e.id              AS event_id,
            e.event_time      AS event_time,
            e.event_type      AS event_type,
            e.user_id         AS user_id,
            ` + exprClient + `  AS client,
            ` + exprIP + `  AS ip_address,
            ` + exprCountry + `  AS country,
            ` + exprCity + `  AS city,
            ` + exprLat + `  AS lat,
            ` + exprLong + `  AS long,
            ` + exprIsVPN + `  AS is_vpn,
            p.pre_auth_risk_decision   AS risk_level,
            p.final_status    AS final_status,
            ` + exprOS + `  AS operating_system,
            ` + exprBrowser + `  AS browser,
            ` + exprDevice + `  AS device,
            ` + exprSystemLanguage + `  AS system_language,
            ` + exprScreenResolution + `  AS screen_resolution
        FROM auth_event e
        LEFT JOIN auth_process p   ON p.id   = e.auth_process
        LEFT JOIN auth_context ac  ON ac.hash = e.auth_context_hash
        WHERE ` + realmFilterExpr

// ListRejectedEventsSince returns risk-rejected (pre_auth_risk_decision = 4)
// events newer than `since` for the realm, oldest first.
func (r *Repository) ListRejectedEventsSince(ctx context.Context, realm string, since time.Time) ([]amfa.EventRow, error) {
	if realm == "" {
		return nil, fmt.Errorf("amfa.ListRejectedEventsSince: realm is required")
	}
	sql := eventRowSelect + `
          AND p.pre_auth_risk_decision = 4
          AND e.event_time > ?
        ORDER BY e.event_time ASC`

	var rows []eventRowScan
	if err := r.db.WithContext(ctx).Raw(sql, realm, realm, since).Scan(&rows).Error; err != nil {
		return nil, classifyDBError(err, "amfa list rejected events")
	}
	out := make([]amfa.EventRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.toEventRow())
	}
	return out, nil
}

// ListVPNRiskyEventsSince returns VPN-flagged events with risk >= minRisk newer
// than `since` for the realm, oldest first.
func (r *Repository) ListVPNRiskyEventsSince(ctx context.Context, realm string, since time.Time, minRisk int) ([]amfa.EventRow, error) {
	if realm == "" {
		return nil, fmt.Errorf("amfa.ListVPNRiskyEventsSince: realm is required")
	}
	// The VPN flag must come through the auth_context_json fallback
	// (context_expr.go). Filtering on ac.is_vpn alone matched nothing, so the
	// vpn_risky alert rule could never fire against real AMFA data.
	sql := eventRowSelect + `
          AND (` + exprIsVPN + `) = true
          AND p.pre_auth_risk_decision >= ?
          AND e.event_time > ?
        ORDER BY e.event_time ASC`

	var rows []eventRowScan
	if err := r.db.WithContext(ctx).Raw(sql, realm, realm, minRisk, since).Scan(&rows).Error; err != nil {
		return nil, classifyDBError(err, "amfa list vpn risky events")
	}
	out := make([]amfa.EventRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.toEventRow())
	}
	return out, nil
}

// ListEventsSince returns all events with event_time strictly newer than
// `since` for the realm, oldest first, capped at `limit` rows. Used by the
// events mirror to incrementally pull new AMFA events; callers page by
// re-invoking with the last returned EventTime.
func (r *Repository) ListEventsSince(ctx context.Context, realm string, since time.Time, limit int) ([]amfa.EventRow, error) {
	if realm == "" {
		return nil, fmt.Errorf("amfa.ListEventsSince: realm is required")
	}
	if limit <= 0 {
		limit = 500
	}
	sql := eventRowSelect + `
          AND e.event_time > ?
        ORDER BY e.event_time ASC
        LIMIT ?`

	var rows []eventRowScan
	if err := r.db.WithContext(ctx).Raw(sql, realm, realm, since, limit).Scan(&rows).Error; err != nil {
		return nil, classifyDBError(err, "amfa list events since")
	}
	out := make([]amfa.EventRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.toEventRow())
	}
	return out, nil
}

// CountRepeatedRiskyByUser returns users in the realm with >= threshold events
// at risk >= minRisk since `since`.
func (r *Repository) CountRepeatedRiskyByUser(ctx context.Context, realm string, since time.Time, minRisk, threshold int) ([]amfa.UserRiskyCount, error) {
	if realm == "" {
		return nil, fmt.Errorf("amfa.CountRepeatedRiskyByUser: realm is required")
	}
	sql := `
        SELECT e.user_id AS user_id, COUNT(*) AS cnt
        FROM auth_event e
        LEFT JOIN auth_process p ON p.id = e.auth_process
        WHERE ` + realmFilterExpr + `
          AND p.pre_auth_risk_decision >= ?
          AND e.event_time >= ?
          AND e.user_id IS NOT NULL
        GROUP BY e.user_id
        HAVING COUNT(*) >= ?`

	type scanRow struct {
		UserID string `gorm:"column:user_id"`
		Cnt    int64  `gorm:"column:cnt"`
	}
	var rows []scanRow
	if err := r.db.WithContext(ctx).Raw(sql, realm, realm, minRisk, since, threshold).Scan(&rows).Error; err != nil {
		return nil, classifyDBError(err, "amfa count repeated risky by user")
	}
	out := make([]amfa.UserRiskyCount, 0, len(rows))
	for _, row := range rows {
		out = append(out, amfa.UserRiskyCount{UserID: row.UserID, Count: row.Cnt})
	}
	return out, nil
}

// CountByEventTypeInWindow returns the count of events of eventType in
// [start, end] for the realm.
func (r *Repository) CountByEventTypeInWindow(ctx context.Context, realm, eventType string, start, end time.Time) (int64, error) {
	if realm == "" {
		return 0, fmt.Errorf("amfa.CountByEventTypeInWindow: realm is required")
	}
	sql := `
        SELECT COUNT(*)
        FROM auth_event e
        LEFT JOIN auth_process p ON p.id = e.auth_process
        WHERE ` + realmFilterExpr + `
          AND e.event_type = ?
          AND e.event_time BETWEEN ? AND ?`

	var count int64
	if err := r.db.WithContext(ctx).Raw(sql, realm, realm, eventType, start, end).Scan(&count).Error; err != nil {
		return 0, classifyDBError(err, "amfa count by event type in window")
	}
	return count, nil
}

// CountByEventTypeAndClientInWindow returns, per OAuth client in the realm,
// the count of eventType events in [start, end], keeping only clients whose
// count >= threshold. Used by the per-client login-error rule.
func (r *Repository) CountByEventTypeAndClientInWindow(ctx context.Context, realm, eventType string, start, end time.Time, threshold int) ([]amfa.ClientEventCount, error) {
	if realm == "" {
		return nil, fmt.Errorf("amfa.CountByEventTypeAndClientInWindow: realm is required")
	}
	sql := `
        SELECT ` + exprClient + ` AS client, COUNT(*) AS cnt
        FROM auth_event e
        LEFT JOIN auth_process p   ON p.id   = e.auth_process
        LEFT JOIN auth_context ac  ON ac.hash = e.auth_context_hash
        WHERE ` + realmFilterExpr + `
          AND e.event_type = ?
          AND e.event_time BETWEEN ? AND ?
          AND ` + exprClient + ` <> ''
        GROUP BY ` + exprClient + `
        HAVING COUNT(*) >= ?`

	type scanRow struct {
		Client string `gorm:"column:client"`
		Cnt    int64  `gorm:"column:cnt"`
	}
	var rows []scanRow
	if err := r.db.WithContext(ctx).Raw(sql, realm, realm, eventType, start, end, threshold).Scan(&rows).Error; err != nil {
		return nil, classifyDBError(err, "amfa count by event type and client in window")
	}
	out := make([]amfa.ClientEventCount, 0, len(rows))
	for _, row := range rows {
		out = append(out, amfa.ClientEventCount{Client: row.Client, Count: row.Cnt})
	}
	return out, nil
}

// CountDistinctRejectedUsersSince returns the number of distinct users with at
// least one pre_auth_risk_decision=4 (rejected) event in the realm since
// `since`. Used by the realm-wide reject-burst rule.
func (r *Repository) CountDistinctRejectedUsersSince(ctx context.Context, realm string, since time.Time) (int64, error) {
	if realm == "" {
		return 0, fmt.Errorf("amfa.CountDistinctRejectedUsersSince: realm is required")
	}
	sql := `
        SELECT COUNT(DISTINCT e.user_id)
        FROM auth_event e
        LEFT JOIN auth_process p ON p.id = e.auth_process
        WHERE ` + realmFilterExpr + `
          AND p.pre_auth_risk_decision = 4
          AND e.event_time >= ?
          AND e.user_id IS NOT NULL`

	var count int64
	if err := r.db.WithContext(ctx).Raw(sql, realm, realm, since).Scan(&count).Error; err != nil {
		return 0, classifyDBError(err, "amfa count distinct rejected users since")
	}
	return count, nil
}

// CountByEventTypeByUser returns, per user in the realm, the count of
// eventType events since `since`, keeping only users whose count >=
// threshold. Used by the per-account login-error rule.
func (r *Repository) CountByEventTypeByUser(ctx context.Context, realm, eventType string, since time.Time, threshold int) ([]amfa.UserRiskyCount, error) {
	if realm == "" {
		return nil, fmt.Errorf("amfa.CountByEventTypeByUser: realm is required")
	}
	sql := `
        SELECT e.user_id AS user_id, COUNT(*) AS cnt
        FROM auth_event e
        LEFT JOIN auth_process p ON p.id = e.auth_process
        WHERE ` + realmFilterExpr + `
          AND e.event_type = ?
          AND e.event_time >= ?
          AND e.user_id IS NOT NULL
        GROUP BY e.user_id
        HAVING COUNT(*) >= ?`

	type scanRow struct {
		UserID string `gorm:"column:user_id"`
		Cnt    int64  `gorm:"column:cnt"`
	}
	var rows []scanRow
	if err := r.db.WithContext(ctx).Raw(sql, realm, realm, eventType, since, threshold).Scan(&rows).Error; err != nil {
		return nil, classifyDBError(err, "amfa count by event type by user")
	}
	out := make([]amfa.UserRiskyCount, 0, len(rows))
	for _, row := range rows {
		out = append(out, amfa.UserRiskyCount{UserID: row.UserID, Count: row.Cnt})
	}
	return out, nil
}
