package amfacheck

import (
	"context"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/amfa"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

// LoginErrorCheck implements rule 4a: a warning alert when the count of AMFA
// LOGIN_ERROR events in the current window-aligned bucket reaches the
// threshold, deduped per (realm, window bucket).
type LoginErrorCheck struct {
	tenantID  string
	realmsFn  RealmsFunc
	repo      amfa.Repository
	threshold int
	window    time.Duration
	logger    Logger
}

// NewLoginErrorCheck constructs rule 4a.
func NewLoginErrorCheck(tenantID string, realmsFn RealmsFunc, repo amfa.Repository, threshold int, window time.Duration, log Logger) *LoginErrorCheck {
	return &LoginErrorCheck{
		tenantID:  tenantID,
		realmsFn:  realmsFn,
		repo:      repo,
		threshold: threshold,
		window:    window,
		logger:    log.WithComponent("amfa-login-error"),
	}
}

func (c *LoginErrorCheck) GetCheckType() string { return "amfa-login-error" }
func (c *LoginErrorCheck) GetDescription() string {
	return "AMFA excessive LOGIN_ERROR events in a window"
}

func (c *LoginErrorCheck) Execute(ctx context.Context) ([]*domain.Alert, error) {
	return runEventTypeThresholdCheck(ctx, eventTypeThresholdParams{
		tenantID:       c.tenantID,
		realmsFn:       c.realmsFn,
		repo:           c.repo,
		eventType:      "LOGIN_ERROR",
		threshold:      c.threshold,
		window:         c.window,
		checkType:      c.GetCheckType(),
		idPrefix:       "amfa-login-error",
		title:          "AMFA: excessive LOGIN_ERROR events",
		recommendation: "Investigate the burst of failed logins. Check for brute-force or credential-stuffing activity against this realm.",
	})
}
