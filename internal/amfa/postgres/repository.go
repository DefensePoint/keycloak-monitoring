package postgres

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/DefensePoint/keycloak-monitoring/internal/amfa"
)

// Repository is a read-only GORM-backed implementation of amfa.Repository
// for one tenant's AMFA database.
type Repository struct {
	db           *gorm.DB
	lookbackDays int // 0 means use defaultLookbackDays
}

// defaultLookbackDays is applied when no per-tenant lookback is configured.
// Also serves as the hard cap if the configured value is unreasonably large.
const (
	defaultLookbackDays = 30
	maxLookbackDays     = 90
)

// NewRepository wraps the given GORM connection as an AMFA repository with
// the default lookback window (30 days, capped at 90).
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db, lookbackDays: defaultLookbackDays}
}

// NewRepositoryWithLookback is NewRepository with an explicit per-tenant
// lookback window (in days). Values <= 0 fall back to defaultLookbackDays;
// values > maxLookbackDays are capped at maxLookbackDays.
func NewRepositoryWithLookback(db *gorm.DB, lookbackDays int) *Repository {
	if lookbackDays <= 0 {
		lookbackDays = defaultLookbackDays
	}
	if lookbackDays > maxLookbackDays {
		lookbackDays = maxLookbackDays
	}
	return &Repository{db: db, lookbackDays: lookbackDays}
}

// Compile-time interface check
var _ amfa.Repository = (*Repository)(nil)

// ListEvents returns a page of AMFA events for a tenant's realm, joined across
// auth_event, auth_process, and auth_context. The realm filter checks
// auth_event.realm_id first, falling back to the JSONB text-extraction
// operator on auth_process.auth_context_json->>'realm_id' (see
// realmFilterExpr, context_expr.go); both store the realm NAME, not a UUID.
//
// Client/IP/country/geo/VPN are read with a COALESCE fallback: the auth_context
// table first (its intended source, which adaptive-auth never populates), then
// auth_process.auth_context_json (what works currently). See context_expr.go.
func (r *Repository) ListEvents(ctx context.Context, opts amfa.ListEventsOptions) (*amfa.ListEventsResult, error) {
	if opts.RealmID == "" {
		return nil, fmt.Errorf("amfa.ListEvents: realm_id is required")
	}
	if opts.Limit <= 0 || opts.Limit > 200 {
		opts.Limit = 25
	}
	if opts.Offset < 0 {
		opts.Offset = 0
	}
	start, end := normalizeTimeWindow(opts.StartTime, opts.EndTime, r.lookbackDays)

	// Shares eventRowSelect and eventRowScan with the incremental queries in
	// event_queries.go rather than carrying its own projection. Keeping a second
	// copy here is what let this query drift: it omitted city, operating_system,
	// browser, device, system_language and screen_resolution, which are declared
	// on amfa.EventRow and populated everywhere else. An unprojected column
	// scans as NULL, indistinguishable from absent data, so the Events page
	// showed those six as empty while the alert rules and events mirror read
	// them from the same rows without trouble.
	baseSelect := eventRowSelect + `
          AND e.event_time BETWEEN ? AND ?`

	args := []interface{}{opts.RealmID, opts.RealmID, start, end}
	if opts.RiskLevel != nil {
		baseSelect += " AND p.pre_auth_risk_decision >= ?"
		args = append(args, *opts.RiskLevel)
	}
	if opts.EventType != "" {
		baseSelect += " AND e.event_type = ?"
		args = append(args, opts.EventType)
	}
	baseSelect += " ORDER BY e.event_time DESC LIMIT ? OFFSET ?"
	args = append(args, opts.Limit, opts.Offset)

	var rows []eventRowScan
	if err := r.db.WithContext(ctx).Raw(baseSelect, args...).Scan(&rows).Error; err != nil {
		return nil, classifyDBError(err, "amfa list events")
	}

	// Count query mirrors the WHERE clause but omits ORDER/LIMIT/OFFSET.
	countSQL := `
        SELECT COUNT(*)
        FROM auth_event e
        LEFT JOIN auth_process p ON p.id = e.auth_process
        WHERE ` + realmFilterExpr + `
          AND e.event_time BETWEEN ? AND ?`
	countArgs := []interface{}{opts.RealmID, opts.RealmID, start, end}
	if opts.RiskLevel != nil {
		countSQL += " AND p.pre_auth_risk_decision >= ?"
		countArgs = append(countArgs, *opts.RiskLevel)
	}
	if opts.EventType != "" {
		countSQL += " AND e.event_type = ?"
		countArgs = append(countArgs, opts.EventType)
	}
	var total int64
	if err := r.db.WithContext(ctx).Raw(countSQL, countArgs...).Scan(&total).Error; err != nil {
		return nil, classifyDBError(err, "amfa count events")
	}

	items := make([]amfa.EventRow, 0, len(rows))
	for _, row := range rows {
		items = append(items, row.toEventRow())
	}
	return &amfa.ListEventsResult{Items: items, Total: total}, nil
}

// GetStats returns the four KPI numbers for the AMFA Events page in a single
// aggregate query.
func (r *Repository) GetStats(ctx context.Context, opts amfa.StatsOptions) (amfa.Stats, error) {
	start, end := normalizeTimeWindow(opts.StartTime, opts.EndTime, r.lookbackDays)

	// All four KPIs describe the same user-selected time window so they remain
	// comparable. Previously unique_users was scoped to the last 24h via an
	// extra FILTER clause; that produced an always-zero KPI for any window not
	// ending at NOW(). See the design spec § 5.4 for the rationale.
	// flagged_ips counts distinct VPN-flagged IPs. Both the IP and the VPN flag
	// go through the auth_context_json fallback (context_expr.go): reading
	// ac.ip_address / ac.is_vpn alone made this KPI permanently 0, because
	// adaptive-auth never writes auth_context rows.
	//
	// An empty RealmID means "all realms": the AMFA DB is per-tenant and holds
	// every realm's auth_event rows, so we aggregate the DISTINCT counts across
	// realms in one pass (summing per-realm results would double-count users/IPs
	// seen in more than one realm). We still require some realm attribution: the
	// AMFA DB also holds auth_event rows with no realm known from EITHER source
	// (realmAttributedExpr, context_expr.go). Counting those here would inflate
	// the KPIs above what summing the per-realm queries (which apply the same
	// e.realm_id-first fallback via realmFilterExpr) would show.
	realmClause := "AND " + realmAttributedExpr
	args := []interface{}{start, end}
	if opts.RealmID != "" {
		realmClause = "AND " + realmFilterExpr
		args = append(args, opts.RealmID, opts.RealmID)
	}

	sql := `
        SELECT
            COUNT(*)                                                              AS total,
            COUNT(*) FILTER (WHERE p.pre_auth_risk_decision >= 3)                  AS risky,
            COUNT(DISTINCT e.user_id)                                              AS unique_users,
            COUNT(DISTINCT NULLIF(` + exprIP + `, ''))
                FILTER (WHERE (` + exprIsVPN + `) = true)                          AS flagged_ips
        FROM auth_event e
        LEFT JOIN auth_process p   ON p.id   = e.auth_process
        LEFT JOIN auth_context ac  ON ac.hash = e.auth_context_hash
        WHERE e.event_time BETWEEN ? AND ?
          ` + realmClause

	var scan struct {
		Total       int64 `gorm:"column:total"`
		Risky       int64 `gorm:"column:risky"`
		UniqueUsers int64 `gorm:"column:unique_users"`
		FlaggedIPs  int64 `gorm:"column:flagged_ips"`
	}
	if err := r.db.WithContext(ctx).Raw(sql, args...).Scan(&scan).Error; err != nil {
		return amfa.Stats{}, classifyDBError(err, "amfa stats query")
	}
	return amfa.Stats{
		Total:       scan.Total,
		Risky:       scan.Risky,
		UniqueUsers: scan.UniqueUsers,
		FlaggedIPs:  scan.FlaggedIPs,
	}, nil
}

// GetGeoBuckets aggregates events into lat/long cells (rounded to 0.1 degree,
// roughly 11 km) for the map view.
func (r *Repository) GetGeoBuckets(ctx context.Context, opts amfa.GeoOptions) ([]amfa.GeoBucket, error) {
	if opts.RealmID == "" {
		return nil, fmt.Errorf("amfa.GetGeoBuckets: realm_id is required")
	}
	start, end := normalizeTimeWindow(opts.StartTime, opts.EndTime, r.lookbackDays)

	// Country and coordinates go through the auth_context_json fallback
	// (context_expr.go). Reading ac.lat / ac.long alone returned zero buckets
	// for every realm, because adaptive-auth never writes auth_context rows -
	// the map was empty no matter how much geolocated traffic AMFA had.
	sql := `
        SELECT
            ` + exprCountry + `                        AS country,
            ROUND((` + exprLat + `)::numeric, 1)::float8   AS lat,
            ROUND((` + exprLong + `)::numeric, 1)::float8  AS long,
            COUNT(*)                                                       AS count,
            COUNT(*) FILTER (WHERE p.pre_auth_risk_decision >= 3)          AS risky_count
        FROM auth_event e
        LEFT JOIN auth_process p   ON p.id   = e.auth_process
        LEFT JOIN auth_context ac  ON ac.hash = e.auth_context_hash
        WHERE ` + realmFilterExpr + `
          AND e.event_time BETWEEN ? AND ?
          AND ` + exprHasGeo + `
        GROUP BY ` + exprCountry + `,
                 ROUND((` + exprLat + `)::numeric, 1),
                 ROUND((` + exprLong + `)::numeric, 1)
        ORDER BY count DESC`

	var buckets []amfa.GeoBucket
	if err := r.db.WithContext(ctx).Raw(sql, opts.RealmID, opts.RealmID, start, end).Scan(&buckets).Error; err != nil {
		return nil, classifyDBError(err, "amfa geo query")
	}
	return buckets, nil
}

// normalizeTimeWindow applies sensible defaults:
//   - If end is nil, use now (UTC).
//   - If start is nil, use end - lookbackDays.
//   - Hard-cap the window at maxLookbackDays.
//
// lookbackDays<=0 falls back to defaultLookbackDays so callers (including
// the legacy single-arg shape via NewRepository) get a safe default.
func normalizeTimeWindow(start, end *time.Time, lookbackDays int) (time.Time, time.Time) {
	if lookbackDays <= 0 {
		lookbackDays = defaultLookbackDays
	}
	if lookbackDays > maxLookbackDays {
		lookbackDays = maxLookbackDays
	}
	if end == nil {
		now := time.Now().UTC()
		end = &now
	}
	if start == nil {
		s := end.Add(-time.Duration(lookbackDays) * 24 * time.Hour)
		start = &s
	}
	if end.Sub(*start) > time.Duration(maxLookbackDays)*24*time.Hour {
		s := end.Add(-time.Duration(maxLookbackDays) * 24 * time.Hour)
		start = &s
	}
	return *start, *end
}
