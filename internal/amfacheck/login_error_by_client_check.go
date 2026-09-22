package amfacheck

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/amfa"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

// LoginErrorByClientCheck implements the per-client login-error rule: like
// rule 4a (LoginErrorCheck), but counted per OAuth client instead of per
// realm, so a burst concentrated on one app isn't diluted by the realm's
// overall traffic. Deduped per (realm, client), the same persists-and-updates
// style as rule 4a/4b (no day component).
type LoginErrorByClientCheck struct {
	tenantID  string
	realmsFn  RealmsFunc
	repo      amfa.Repository
	threshold int
	window    time.Duration
	logger    Logger
}

// NewLoginErrorByClientCheck constructs the per-client login-error rule.
func NewLoginErrorByClientCheck(tenantID string, realmsFn RealmsFunc, repo amfa.Repository, threshold int, window time.Duration, log Logger) *LoginErrorByClientCheck {
	return &LoginErrorByClientCheck{
		tenantID:  tenantID,
		realmsFn:  realmsFn,
		repo:      repo,
		threshold: threshold,
		window:    window,
		logger:    log.WithComponent("amfa-login-error-by-client"),
	}
}

func (c *LoginErrorByClientCheck) GetCheckType() string { return "amfa-login-error-by-client" }
func (c *LoginErrorByClientCheck) GetDescription() string {
	return "AMFA excessive LOGIN_ERROR events for one client in a window"
}

func (c *LoginErrorByClientCheck) Execute(ctx context.Context) ([]*domain.Alert, error) {
	realms, err := c.realmsFn(ctx)
	if err != nil {
		return nil, fmt.Errorf("amfa-login-error-by-client: load realms: %w", err)
	}
	now := time.Now().UTC()
	start := now.Add(-c.window)

	var alerts []*domain.Alert
	for _, realm := range realms {
		counts, err := c.repo.CountByEventTypeAndClientInWindow(ctx, realm, "LOGIN_ERROR", start, now, c.threshold)
		if err != nil {
			return nil, fmt.Errorf("amfa-login-error-by-client: realm=%s: %w", realm, err)
		}
		for _, cc := range counts {
			alerts = append(alerts, c.toAlert(realm, cc))
		}
	}
	return alerts, nil
}

func (c *LoginErrorByClientCheck) toAlert(realm string, cc amfa.ClientEventCount) *domain.Alert {
	md, _ := json.Marshal(map[string]any{
		"event_type": "LOGIN_ERROR",
		"client":     cc.Client,
		"count":      cc.Count,
		"threshold":  c.threshold,
		"window":     c.window.String(),
		"realm_name": realm,
	})
	return &domain.Alert{
		TenantID:       c.tenantID,
		AlertID:        alertID("amfa-login-error-by-client", c.tenantID, realm, cc.Client),
		Source:         domain.AlertSourceEvent,
		Type:           domain.AlertTypeSecurity,
		Severity:       domain.AlertSeverityWarning,
		Status:         domain.AlertStatusActive,
		Title:          "AMFA: excessive LOGIN_ERROR events for one client",
		Description:    fmt.Sprintf("%d LOGIN_ERROR events for client '%s' in realm '%s' within %s (threshold: %d).", cc.Count, cc.Client, realm, c.window.String(), c.threshold),
		ResourceType:   "amfa_client",
		ResourceID:     cc.Client,
		ResourceName:   cc.Client,
		RealmName:      realm,
		CheckType:      c.GetCheckType(),
		Recommendation: "Investigate the burst of failed logins against this specific client. Check for credential stuffing or brute-force activity targeting this app.",
		Metadata:       string(md),
	}
}
