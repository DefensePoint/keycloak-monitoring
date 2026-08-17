package reports

import (
	"time"

	"repos.defensepoint.com/iam/oss-pmt/kmt/internal/domain"
)

// AlertListOptions contains options for listing alerts.
type AlertListOptions struct {
	Limit  int
	Offset int
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
}
