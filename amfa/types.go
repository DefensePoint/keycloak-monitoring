// Package amfa provides types and services for reading AMFA (Adaptive MFA)
// events from per-tenant AMFA PostgreSQL databases.
package amfa

import (
	"errors"
	"time"
)

// ErrAmfaNotConfigured is returned when the caller asks for AMFA data for a
// tenant that does not have AMFA enabled in its configuration.
var ErrAmfaNotConfigured = errors.New("amfa: not configured for this tenant")

// ErrAmfaUnavailable is returned when the per-tenant AMFA database connection
// cannot be established or used.
var ErrAmfaUnavailable = errors.New("amfa: data source unavailable")

// EventRow is one AMFA event as joined from auth_event + auth_process +
// auth_context. It is the repository's row-level type; the service layer
// enriches it into Event.
type EventRow struct {
	EventID     string
	EventTime   time.Time
	EventType   string // LOGIN | LOGIN_ERROR | CLIENT_LOGIN | CLIENT_LOGIN_ERROR
	UserID      *string
	Client      string
	IPAddress   string
	Country     *string
	City        *string
	Lat         *float64
	Long        *float64
	IsVPN       bool
	RiskLevel   *int    // 1..4; nil if no auth_process row joined
	FinalStatus *string // LOGIN | LOGIN_ERROR; nil if no auth_process row joined
	// Device/agent context captured by AMFA at decision time.
	OperatingSystem  *string
	Browser          *string
	Device           *string
	SystemLanguage   *string
	ScreenResolution *string
}

// UserRiskyCount is the row shape returned by Repository.CountRepeatedRiskyByUser:
// one user and the number of risky events they accumulated in the query window.
type UserRiskyCount struct {
	UserID string
	Count  int64
}

// Event is what the service layer returns to the HTTP handler — EventRow plus
// Keycloak-enriched username/email.
type Event struct {
	EventID     string    `json:"event_id"`
	EventTime   time.Time `json:"event_time"`
	EventType   string    `json:"event_type"`
	UserID      *string   `json:"user_id"`
	Username    *string   `json:"username"`
	Email       *string   `json:"email"`
	Client      string    `json:"client"`
	IPAddress   string    `json:"ip_address"`
	Country     *string   `json:"country"`
	Lat         *float64  `json:"lat"`
	Long        *float64  `json:"long"`
	IsVPN       bool      `json:"is_vpn"`
	RiskLevel   *int      `json:"risk_level"`
	FinalStatus *string   `json:"final_status"`
}

// Stats are the four KPI numbers for the AMFA Events page.
type Stats struct {
	Total       int64 `json:"total"`
	Risky       int64 `json:"risky"` // pre_auth_risk_decision >= 3
	UniqueUsers int64 `json:"unique_users"`
	FlaggedIPs  int64 `json:"flagged_ips"` // is_vpn = true
}

// GeoBucket is one aggregated lat/long cell (~11 km) for the map.
type GeoBucket struct {
	Country    *string `json:"country"`
	Lat        float64 `json:"lat"`
	Long       float64 `json:"long"`
	Count      int64   `json:"count"`
	RiskyCount int64   `json:"risky_count"`
}

// ListEventsOptions filters for the events list query.
type ListEventsOptions struct {
	RealmID   string
	StartTime *time.Time
	EndTime   *time.Time
	RiskLevel *int   // treated as ">=" filter when set
	EventType string // exact-match filter when set
	Limit     int
	Offset    int
}

// StatsOptions filters for the KPI query.
type StatsOptions struct {
	RealmID   string
	StartTime *time.Time
	EndTime   *time.Time
}

// GeoOptions filters for the map geo-aggregation query.
type GeoOptions struct {
	RealmID   string
	StartTime *time.Time
	EndTime   *time.Time
}

// ListEventsResult is what ListEvents returns to the service layer.
type ListEventsResult struct {
	Items []EventRow
	Total int64
}
