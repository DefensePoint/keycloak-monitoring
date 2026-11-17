package configcheck

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/alerts/clearing"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/pkg/keycloakadmin"
)

// IdentityProviderSecretExpiry is when one identity provider's client secret
// expires: ExpiresOn is midnight UTC of the last day it is valid.
type IdentityProviderSecretExpiry struct {
	Realm            string
	IdentityProvider string
	ExpiresOn        time.Time
}

// IdentityProviderSecretExpiryCheck warns before the client secret of an
// identity provider expires, and alerts once it has. Keycloak does not know
// when a secret expires: the provider that issued it does, so the dates come
// from the configuration.
type IdentityProviderSecretExpiryCheck struct {
	keycloakClient KeycloakClient
	logger         *logger.Logger
	tenantID       string
	warningDays    int
	secrets        []IdentityProviderSecretExpiry
	now            func() time.Time
}

// NewIdentityProviderSecretExpiryCheck creates a checker for the tenant's
// listed secrets.
func NewIdentityProviderSecretExpiryCheck(
	keycloakClient KeycloakClient,
	log *logger.Logger,
	tenantID string,
	warningDays int,
	secrets []IdentityProviderSecretExpiry,
) *IdentityProviderSecretExpiryCheck {
	if warningDays <= 0 {
		warningDays = 30
	}
	return &IdentityProviderSecretExpiryCheck{
		keycloakClient: keycloakClient,
		logger:         log.WithComponent("idp_secret_expiry_check"),
		tenantID:       tenantID,
		warningDays:    warningDays,
		secrets:        secrets,
		now:            time.Now,
	}
}

// GetCheckType returns the unique identifier for this check.
func (c *IdentityProviderSecretExpiryCheck) GetCheckType() string {
	return "identity_provider_secret_expiry"
}

// GetDescription returns a human-readable description of this check.
func (c *IdentityProviderSecretExpiryCheck) GetDescription() string {
	return "Warns before the client secret of an identity provider expires, from the expiry dates in the configuration"
}

// Execute performs the configuration check.
func (c *IdentityProviderSecretExpiryCheck) Execute(ctx context.Context) ([]*domain.Alert, error) {
	eval, err := c.Evaluate(ctx)
	return eval.Alerts, err
}

// Evaluate judges every listed secret whose realm is enabled, and reports the
// realms whose identity providers it read, and those with no listed secret. A secret whose realm or provider
// Keycloak does not have raises an alert, since its expiry is no longer
// watched; only a 404 says a realm does not exist, since Keycloak lists only
// the realms the monitoring client can see. A disabled realm is skipped, so
// its alerts resolve with it, and one whose providers could not be listed,
// or that the client cannot see, keeps its alerts as they are.
func (c *IdentityProviderSecretExpiryCheck) Evaluate(ctx context.Context) (Evaluation, error) {
	eval := Evaluation{Evaluation: clearing.Evaluation{TenantID: c.tenantID}}

	checked, scope, err := scopeRealms(ctx, c.keycloakClient, nil)
	if err != nil {
		return eval, fmt.Errorf("failed to get realms: %w", err)
	}
	eval.scope = scope

	now := c.now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	var realms []string
	byRealm := map[string][]IdentityProviderSecretExpiry{}
	for _, s := range c.secrets {
		if _, ok := byRealm[s.Realm]; !ok {
			realms = append(realms, s.Realm)
		}
		byRealm[s.Realm] = append(byRealm[s.Realm], s)
	}

	for _, realm := range realms {
		switch {
		case scope.checked[realm]:
			idps, err := c.keycloakClient.GetIdentityProviders(ctx, realm)
			if err != nil {
				c.logger.Error("Failed to list identity providers",
					logger.Str("realm", realm), logger.Err(err))
				continue
			}
			byAlias := make(map[string]*keycloakadmin.IdentityProviderRepresentation, len(idps))
			for _, idp := range idps {
				byAlias[idp.Alias] = idp
			}
			for _, s := range byRealm[realm] {
				if idp, ok := byAlias[s.IdentityProvider]; ok {
					if alert := c.judge(realm, idp, s, today); alert != nil {
						eval.Alerts = append(eval.Alerts, alert)
					}
				} else {
					eval.Alerts = append(eval.Alerts, c.createNotFoundAlert(s, "Keycloak has no identity provider with this alias in the realm"))
				}
			}
			eval.MarkRealmEvaluated(realm)
		case scope.visible[realm] || scope.unreadable[realm]:
			// Disabled, or not readable by the monitoring client.
		case scope.deleted(realm):
			for _, s := range byRealm[realm] {
				eval.Alerts = append(eval.Alerts, c.createNotFoundAlert(s, "Keycloak has no realm with this name"))
			}
		default:
			// Not listed, yet it exists or could not be asked about: the
			// client cannot see it, so its alerts stay as they are.
		}
	}

	// A realm with no listed secret has nothing to warn about, so it counts
	// as read: the alerts of entries since removed resolve.
	for _, realm := range checked {
		if _, ok := byRealm[realm]; !ok {
			eval.MarkRealmEvaluated(realm)
		}
	}

	c.logger.Debug("Identity provider secret expiry check completed",
		logger.Int("secrets", len(c.secrets)), logger.Int("total_alerts", len(eval.Alerts)))
	return eval, nil
}

// judge returns the alert for one secret, or nil while it is outside the
// warning window. ExpiresOn is the last valid day: on it the secret still
// warns, and from the next day on it has expired.
func (c *IdentityProviderSecretExpiryCheck) judge(
	realm string,
	idp *keycloakadmin.IdentityProviderRepresentation,
	s IdentityProviderSecretExpiry,
	today time.Time,
) *domain.Alert {
	days := int(s.ExpiresOn.Sub(today).Hours() / 24)
	switch {
	case days < 0:
		return c.createExpiredAlert(realm, idp, s, -days)
	case days <= c.warningDays:
		return c.createExpiringSoonAlert(realm, idp, s, days)
	default:
		return nil
	}
}

func (c *IdentityProviderSecretExpiryCheck) createExpiringSoonAlert(
	realm string,
	idp *keycloakadmin.IdentityProviderRepresentation,
	s IdentityProviderSecretExpiry,
	days int,
) *domain.Alert {
	name := idpDisplayName(idp)
	date := s.ExpiresOn.Format(time.DateOnly)
	when := fmt.Sprintf("in %d days", days)
	if days == 0 {
		when = "today"
	}
	metadataJSON, _ := json.Marshal(c.metadata(realm, idp, s, map[string]any{
		"days_until_expiry": days,
		"warning_threshold": c.warningDays,
	}))

	return &domain.Alert{
		TenantID:       c.tenantID,
		AlertID:        c.warningAlertID(realm, idp.Alias, s.ExpiresOn),
		Type:           domain.AlertTypeIdentityProvider,
		Severity:       domain.AlertSeverityWarning,
		Status:         domain.AlertStatusActive,
		Title:          fmt.Sprintf("Identity Provider '%s' client secret expires %s", name, when),
		Description:    fmt.Sprintf("The client secret of identity provider '%s' (alias: %s) in realm '%s' is valid until %s, the last day, which is %s. After that, logins through this provider fail until a new secret is set.", name, idp.Alias, realm, date, when),
		ResourceType:   "identity_provider",
		ResourceID:     idp.InternalID,
		ResourceName:   idp.Alias,
		RealmName:      realm,
		CheckType:      c.GetCheckType(),
		Recommendation: fmt.Sprintf("Create a new client secret for '%s' at the identity provider that issued it, set it in Keycloak under Identity Providers > %s, then update this provider's expires_on in the KMT configuration.", name, idp.Alias),
		Metadata:       string(metadataJSON),
	}
}

func (c *IdentityProviderSecretExpiryCheck) createExpiredAlert(
	realm string,
	idp *keycloakadmin.IdentityProviderRepresentation,
	s IdentityProviderSecretExpiry,
	daysSince int,
) *domain.Alert {
	name := idpDisplayName(idp)
	metadataJSON, _ := json.Marshal(c.metadata(realm, idp, s, map[string]any{
		"days_since_expiry": daysSince,
	}))

	return &domain.Alert{
		TenantID:       c.tenantID,
		AlertID:        c.expiredAlertID(realm, idp.Alias, s.ExpiresOn),
		Type:           domain.AlertTypeIdentityProvider,
		Severity:       domain.AlertSeverityCritical,
		Status:         domain.AlertStatusActive,
		Title:          fmt.Sprintf("Identity Provider '%s' client secret has expired", name),
		Description:    fmt.Sprintf("The client secret of identity provider '%s' (alias: %s) in realm '%s' was valid until %s, %d days ago. Logins through this provider fail until a new secret is set.", name, idp.Alias, realm, s.ExpiresOn.Format(time.DateOnly), daysSince),
		ResourceType:   "identity_provider",
		ResourceID:     idp.InternalID,
		ResourceName:   idp.Alias,
		RealmName:      realm,
		CheckType:      c.GetCheckType(),
		Recommendation: fmt.Sprintf("Create a new client secret for '%s' at the identity provider that issued it, set it in Keycloak under Identity Providers > %s, then update this provider's expires_on in the KMT configuration.", name, idp.Alias),
		Metadata:       string(metadataJSON),
		Supersedes:     []string{c.warningAlertID(realm, idp.Alias, s.ExpiresOn)},
	}
}

func (c *IdentityProviderSecretExpiryCheck) createNotFoundAlert(s IdentityProviderSecretExpiry, reason string) *domain.Alert {
	metadataJSON, _ := json.Marshal(map[string]any{
		"realm":      s.Realm,
		"idp_alias":  s.IdentityProvider,
		"expires_on": s.ExpiresOn.Format(time.DateOnly),
	})

	return &domain.Alert{
		TenantID:       c.tenantID,
		AlertID:        c.notFoundAlertID(s.Realm, s.IdentityProvider),
		Type:           domain.AlertTypeIdentityProvider,
		Severity:       domain.AlertSeverityWarning,
		Status:         domain.AlertStatusActive,
		Title:          fmt.Sprintf("Identity Provider '%s' not found for its secret expiry date", s.IdentityProvider),
		Description:    fmt.Sprintf("The KMT configuration lists a client secret expiry date for identity provider '%s' in realm '%s', but %s. It may have been renamed or deleted, so its secret expiry is not watched.", s.IdentityProvider, s.Realm, reason),
		ResourceType:   "identity_provider",
		ResourceName:   s.IdentityProvider,
		RealmName:      s.Realm,
		CheckType:      c.GetCheckType(),
		Recommendation: "Correct or remove this entry under keycloak.global.config_checker.checks.identity_provider_secret_expiry.secrets in the KMT configuration.",
		Metadata:       string(metadataJSON),
	}
}

func (c *IdentityProviderSecretExpiryCheck) metadata(
	realm string,
	idp *keycloakadmin.IdentityProviderRepresentation,
	s IdentityProviderSecretExpiry,
	extra map[string]any,
) map[string]any {
	m := map[string]any{
		"realm":       realm,
		"idp_alias":   idp.Alias,
		"idp_name":    idp.DisplayName,
		"provider_id": idp.ProviderID,
		"internal_id": idp.InternalID,
		"expires_on":  s.ExpiresOn.Format(time.DateOnly),
	}
	for k, v := range extra {
		m[k] = v
	}
	return m
}

// warningAlertID is the ID of the expiring-soon alert. Like a certificate's
// serial number, the date tells one secret from the next: a renewed secret
// warns afresh rather than reopening an acknowledged warning.
func (c *IdentityProviderSecretExpiryCheck) warningAlertID(realm, alias string, expiresOn time.Time) string {
	return c.alertID("idp-secret-", "secret", realm, alias, expiresOn.Format(time.DateOnly))
}

// expiredAlertID differs from the warning's, so expiry is a new alert, not an
// update of a warning that may have been acknowledged or resolved.
func (c *IdentityProviderSecretExpiryCheck) expiredAlertID(realm, alias string, expiresOn time.Time) string {
	return c.alertID("idp-secret-expired-", "secret-expired", realm, alias, expiresOn.Format(time.DateOnly))
}

func (c *IdentityProviderSecretExpiryCheck) notFoundAlertID(realm, alias string) string {
	return c.alertID("idp-secret-missing-", "secret-missing", realm, alias, "")
}

func (c *IdentityProviderSecretExpiryCheck) alertID(prefix, kind, realm, alias, date string) string {
	input := fmt.Sprintf("%s:%s:%s:%s:%s:%s", c.tenantID, c.GetCheckType(), kind, realm, alias, date)
	hash := sha256.Sum256([]byte(input))
	return fmt.Sprintf("%s%x", prefix, hash[:16])
}

// idpDisplayName returns the provider's display name, or its alias.
func idpDisplayName(idp *keycloakadmin.IdentityProviderRepresentation) string {
	if idp.DisplayName != "" {
		return idp.DisplayName
	}
	return idp.Alias
}
