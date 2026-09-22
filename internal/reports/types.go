package reports

import (
	"slices"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

// AlertListOptions contains options for listing alerts.
type AlertListOptions struct {
	Limit  int
	Offset int
	// RealmNames restricts the query to these realms. An empty list applies no
	// filter at all, which reads as every realm.
	RealmNames []string
}

// RealmScope is the set of realms a report may cover. All means unrestricted;
// otherwise Realms lists every realm it may include, so the zero value denies
// rather than grants.
type RealmScope struct {
	Realms []string
	All    bool
}

// covers reports whether a realm is inside the scope.
func (s RealmScope) covers(realm string) bool {
	return s.All || slices.Contains(s.Realms, realm)
}

// empty reports a scope that names no realm at all.
func (s RealmScope) empty() bool {
	return !s.All && len(s.Realms) == 0
}

// Data contains all data needed for a report.
type Data struct {
	TenantID    string
	TenantName  string
	StartDate   time.Time
	EndDate     time.Time
	GeneratedAt time.Time

	// Statistics
	TotalAlerts    int
	CriticalAlerts int
	HighAlerts     int
	MediumAlerts   int
	LowAlerts      int
	InfoAlerts     int
	OpenAlerts     int
	TotalEvents    int
	TotalRealms    int

	// Detailed data
	AlertsByType     []TypeStats
	TopEventTypes    []TypeStats
	TopAlertedRealms []RealmStats
	AlertList        []AlertDetail

	// Operator metrics
	OperatorMetrics    []*domain.OperatorMetricsSummary
	TotalOperators     int
	TotalOperatorHours float64
	AvgResponseTimeMin float64 // in minutes

	// Template-specific fields
	CoverImagePath string
	LogoPath       string
	RiskLevel      string
	RiskLevelText  string
}

// RealmStats represents statistics for a realm.
type RealmStats struct {
	Name          string
	AlertCount    int
	EventCount    int
	CriticalCount int
	ErrorCount    int
	WarningCount  int
	InfoCount     int
}

// TypeStats represents statistics for a type.
type TypeStats struct {
	Type  string
	Count int
}

// AlertDetail represents a detailed alert for the findings section.
type AlertDetail struct {
	ID          int
	Title       string
	Severity    string
	Status      string
	Description string
	Remediation string
	RealmName   string
}

// GenerateRequest contains parameters for report generation.
type GenerateRequest struct {
	TenantID   string
	TenantName string
	StartDate  time.Time
	EndDate    time.Time
	// Scope bounds both the alerts and the events the report may include.
	Scope RealmScope
}
