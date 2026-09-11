package amfacheck

import (
	"context"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/amfa"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

// ClientLoginErrorCheck implements rule 4b: identical to rule 4a but counts
// CLIENT_LOGIN_ERROR events and uses a distinct AlertID prefix.
type ClientLoginErrorCheck struct {
	tenantID  string
	realmsFn  RealmsFunc
	repo      amfa.Repository
	threshold int
	window    time.Duration
	logger    Logger
}

// NewClientLoginErrorCheck constructs rule 4b.
func NewClientLoginErrorCheck(tenantID string, realmsFn RealmsFunc, repo amfa.Repository, threshold int, window time.Duration, log Logger) *ClientLoginErrorCheck {
	return &ClientLoginErrorCheck{
		tenantID:  tenantID,
		realmsFn:  realmsFn,
		repo:      repo,
		threshold: threshold,
		window:    window,
		logger:    log.WithComponent("amfa-client-login-error"),
	}
}

func (c *ClientLoginErrorCheck) GetCheckType() string { return "amfa-client-login-error" }
func (c *ClientLoginErrorCheck) GetDescription() string {
	return "AMFA excessive CLIENT_LOGIN_ERROR events in a window"
}

func (c *ClientLoginErrorCheck) Execute(ctx context.Context) ([]*domain.Alert, error) {
	return runEventTypeThresholdCheck(ctx, eventTypeThresholdParams{
		tenantID:       c.tenantID,
		realmsFn:       c.realmsFn,
		repo:           c.repo,
		eventType:      "CLIENT_LOGIN_ERROR",
		threshold:      c.threshold,
		window:         c.window,
		checkType:      c.GetCheckType(),
		idPrefix:       "amfa-client-login-error",
		title:          "AMFA: excessive CLIENT_LOGIN_ERROR events",
		recommendation: "Investigate the burst of client authentication failures. Check client credentials, service-account settings, and client configuration for this realm.",
	})
}
