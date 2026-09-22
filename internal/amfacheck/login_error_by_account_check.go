package amfacheck

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/amfa"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

// LoginErrorByAccountCheck implements the per-account login-error rule: a
// warning alert when one user account accumulates >= threshold LOGIN_ERROR
// events within a rolling window, the classic single-account brute-force
// pattern that isn't covered by risk-level-based rules (a brute-forced
// account's failed attempts don't necessarily score as risky) or by the
// realm-wide login-error rule (diluted by every other user's traffic).
// Deduped per (realm, user, UTC day), the same shape as rule 2
// (RepeatedRiskyCheck).
type LoginErrorByAccountCheck struct {
	tenantID  string
	realmsFn  RealmsFunc
	repo      amfa.Repository
	threshold int
	window    time.Duration
	logger    Logger
}

// NewLoginErrorByAccountCheck constructs the per-account login-error rule.
func NewLoginErrorByAccountCheck(tenantID string, realmsFn RealmsFunc, repo amfa.Repository, threshold int, window time.Duration, log Logger) *LoginErrorByAccountCheck {
	return &LoginErrorByAccountCheck{
		tenantID:  tenantID,
		realmsFn:  realmsFn,
		repo:      repo,
		threshold: threshold,
		window:    window,
		logger:    log.WithComponent("amfa-login-error-by-account"),
	}
}

func (c *LoginErrorByAccountCheck) GetCheckType() string { return "amfa-login-error-by-account" }
func (c *LoginErrorByAccountCheck) GetDescription() string {
	return "AMFA excessive LOGIN_ERROR events for one account in a window"
}

func (c *LoginErrorByAccountCheck) Execute(ctx context.Context) ([]*domain.Alert, error) {
	realms, err := c.realmsFn(ctx)
	if err != nil {
		return nil, fmt.Errorf("amfa-login-error-by-account: load realms: %w", err)
	}
	now := time.Now().UTC()
	since := now.Add(-c.window)
	day := now.Format("2006-01-02")

	var alerts []*domain.Alert
	for _, realm := range realms {
		counts, err := c.repo.CountByEventTypeByUser(ctx, realm, "LOGIN_ERROR", since, c.threshold)
		if err != nil {
			return nil, fmt.Errorf("amfa-login-error-by-account: realm=%s: %w", realm, err)
		}
		for _, uc := range counts {
			alerts = append(alerts, c.toAlert(realm, day, uc))
		}
	}
	return alerts, nil
}

func (c *LoginErrorByAccountCheck) toAlert(realm, day string, uc amfa.UserRiskyCount) *domain.Alert {
	md, _ := json.Marshal(map[string]any{
		"event_type": "LOGIN_ERROR",
		"user_id":    uc.UserID,
		"count":      uc.Count,
		"threshold":  c.threshold,
		"window":     c.window.String(),
		"realm_name": realm,
		"utc_date":   day,
	})
	return &domain.Alert{
		TenantID:       c.tenantID,
		AlertID:        alertID("amfa-login-error-by-account", c.tenantID, realm, uc.UserID, day),
		Source:         domain.AlertSourceEvent,
		Type:           domain.AlertTypeSecurity,
		Severity:       domain.AlertSeverityWarning,
		Status:         domain.AlertStatusActive,
		Title:          "AMFA: excessive LOGIN_ERROR events for one account",
		Description:    fmt.Sprintf("User %s had %d LOGIN_ERROR events in realm '%s' within %s (threshold: %d).", uc.UserID, uc.Count, realm, c.window.String(), c.threshold),
		ResourceType:   "amfa_user",
		ResourceID:     uc.UserID,
		ResourceName:   uc.UserID,
		RealmName:      realm,
		CheckType:      c.GetCheckType(),
		Recommendation: "Review this account's recent login attempts. A burst of failed logins against one account is the classic sign of a password-guessing or credential-stuffing attempt.",
		Metadata:       string(md),
	}
}
