package amfacheck

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/alerts/clearing"
	"github.com/DefensePoint/keycloak-monitoring/internal/amfa"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

// RepeatedRiskyCheck implements rule 2: a warning alert per (realm, user) while
// a user has >= threshold events at risk >= minRisk within the rolling window
// and is still making them: at least one within activeWithin.
//
// The window is long (24h by default), so its count alone stays over the
// threshold for a day after a burst, and a second burst the same day would be
// the same unbroken condition, invisible to whoever handled the first. Asking
// for recent activity as well ends the condition when the user goes quiet: the
// alert resolves, and if they start again it reopens and notifies.
type RepeatedRiskyCheck struct {
	tenantID  string
	realmsFn  RealmsFunc
	repo      amfa.Repository
	threshold int
	window    time.Duration
	minRisk   int
	// activeWithin is how recent a risky attempt must be for the user to
	// count as still active.
	activeWithin time.Duration
	logger       Logger
}

// NewRepeatedRiskyCheck constructs rule 2.
// activeWithin should be at least the poll interval, or an attempt landing
// between two polls is never seen as recent.
func NewRepeatedRiskyCheck(tenantID string, realmsFn RealmsFunc, repo amfa.Repository, threshold int, window time.Duration, minRisk int, activeWithin time.Duration, log Logger) *RepeatedRiskyCheck {
	return &RepeatedRiskyCheck{
		tenantID:     tenantID,
		realmsFn:     realmsFn,
		repo:         repo,
		threshold:    threshold,
		window:       window,
		minRisk:      minRisk,
		activeWithin: activeWithin,
		logger:       log.WithComponent("amfa-repeated-risky"),
	}
}

func (c *RepeatedRiskyCheck) GetCheckType() string { return "amfa-repeated-risky" }
func (c *RepeatedRiskyCheck) GetDescription() string {
	return "AMFA repeated risky attempts by one user within the window"
}

func (c *RepeatedRiskyCheck) Execute(ctx context.Context) ([]*domain.Alert, error) {
	ev, err := c.Evaluate(ctx)
	return ev.Alerts, err
}

// Evaluate runs the check and reports every realm it counted. The query only
// returns users over the threshold, so the realm as a whole is what was
// evaluated: a user's alert there resolves once they drop below it.
func (c *RepeatedRiskyCheck) Evaluate(ctx context.Context) (clearing.Evaluation, error) {
	realms, err := c.realmsFn(ctx)
	if err != nil {
		return clearing.Evaluation{}, fmt.Errorf("amfa-repeated-risky: load realms: %w", err)
	}
	now := time.Now().UTC()
	since := now.Add(-c.window)
	activeSince := now.Add(-c.activeWithin)

	ev := clearing.Evaluation{TenantID: c.tenantID}
	for _, realm := range realms {
		counts, err := c.repo.CountRepeatedRiskyByUser(ctx, realm, since, c.minRisk, c.threshold)
		if err != nil {
			return clearing.Evaluation{}, fmt.Errorf("amfa-repeated-risky: realm=%s: %w", realm, err)
		}
		var active map[string]bool
		if len(counts) > 0 {
			recent, err := c.repo.CountRepeatedRiskyByUser(ctx, realm, activeSince, c.minRisk, 1)
			if err != nil {
				return clearing.Evaluation{}, fmt.Errorf("amfa-repeated-risky: realm=%s: recent activity: %w", realm, err)
			}
			active = make(map[string]bool, len(recent))
			for _, uc := range recent {
				active[uc.UserID] = true
			}
		}
		ev.MarkRealmEvaluated(realm)
		for _, uc := range counts {
			if active[uc.UserID] {
				ev.Alerts = append(ev.Alerts, c.toAlert(realm, uc))
			}
		}
	}
	return ev, nil
}

func (c *RepeatedRiskyCheck) toAlert(realm string, uc amfa.UserRiskyCount) *domain.Alert {
	md, _ := json.Marshal(map[string]any{
		"user_id":        uc.UserID,
		"count":          uc.Count,
		"threshold":      c.threshold,
		"min_risk_level": c.minRisk,
		"window":         c.window.String(),
		"realm_name":     realm,
	})
	return &domain.Alert{
		TenantID:       c.tenantID,
		AlertID:        alertID("amfa-repeated-risky", c.tenantID, realm, uc.UserID),
		Source:         domain.AlertSourceEvent,
		Type:           domain.AlertTypeSecurity,
		Severity:       domain.AlertSeverityWarning,
		Status:         domain.AlertStatusActive,
		Title:          "AMFA: repeated risky login attempts by one user",
		Description:    fmt.Sprintf("User %s had %d risky (risk >= %d) authentication attempts in realm '%s' within %s (threshold: %d).", uc.UserID, uc.Count, c.minRisk, realm, c.window.String(), c.threshold),
		ResourceType:   "amfa_user",
		ResourceID:     uc.UserID,
		ResourceName:   uc.UserID,
		RealmName:      realm,
		CheckType:      c.GetCheckType(),
		Recommendation: "Review this user's recent authentication history. Repeated risky attempts may indicate credential stuffing, an account takeover attempt, or a misconfigured client.",
		Metadata:       string(md),
	}
}
