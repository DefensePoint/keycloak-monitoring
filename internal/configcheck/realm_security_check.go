package configcheck

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/pkg/keycloakadmin"
)

// RealmSecurityCheck checks realm-level security configurations.
type RealmSecurityCheck struct {
	keycloakClient KeycloakClient
	logger         *logger.Logger
	realms         []string
	tenantID       string
	config         RealmSecurityCheckConfig
}

// RealmSecurityCheckConfig contains configuration for realm security checks.
type RealmSecurityCheckConfig struct {
	CheckSSLRequired              bool
	CheckBruteForce               bool
	CheckPasswordPolicy           bool
	CheckEmailVerification        bool
	CheckAdminEvents              bool
	CheckUserEvents               bool
	CheckDuplicateEmails          bool
	MinPasswordLength             int
	RequirePasswordComplexity     bool
	MaxAccessTokenLifespanMinutes int
	MaxSSOSessionIdleMinutes      int
}

// NewRealmSecurityCheck creates a new realm security checker.
func NewRealmSecurityCheck(
	keycloakClient KeycloakClient,
	log *logger.Logger,
	realms []string,
	tenantID string,
	config RealmSecurityCheckConfig,
) *RealmSecurityCheck {
	// Set defaults
	if config.MinPasswordLength == 0 {
		config.MinPasswordLength = 8
	}
	if config.MaxAccessTokenLifespanMinutes == 0 {
		config.MaxAccessTokenLifespanMinutes = 15
	}
	if config.MaxSSOSessionIdleMinutes == 0 {
		config.MaxSSOSessionIdleMinutes = 30
	}

	return &RealmSecurityCheck{
		keycloakClient: keycloakClient,
		logger:         log.WithComponent("realm_security_check"),
		realms:         realms,
		tenantID:       tenantID,
		config:         config,
	}
}

// GetCheckType returns the unique identifier for this check.
func (c *RealmSecurityCheck) GetCheckType() string {
	return "realm_security"
}

// GetDescription returns a human-readable description of this check.
func (c *RealmSecurityCheck) GetDescription() string {
	return "Checks realm security configurations including SSL, password policies, brute force protection, and audit settings"
}

// Execute performs the configuration check.
func (c *RealmSecurityCheck) Execute(ctx context.Context) ([]*domain.Alert, error) {
	c.logger.Debug("Executing realm security check")

	realms, err := c.getRealmsToCheck(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get realms: %w", err)
	}

	var alerts []*domain.Alert

	for _, realmName := range realms {
		realm, err := c.keycloakClient.GetRealmInfo(ctx, realmName)
		if err != nil {
			c.logger.Error("Failed to get realm info",
				logger.Str("realm", realmName),
				logger.Err(err))
			continue
		}

		realmAlerts := c.checkRealm(realm)
		alerts = append(alerts, realmAlerts...)
	}

	c.logger.Info("Realm security check completed",
		logger.Int("total_alerts", len(alerts)))

	return alerts, nil
}

// getRealmsToCheck returns the list of realms to check.
func (c *RealmSecurityCheck) getRealmsToCheck(ctx context.Context) ([]string, error) {
	if len(c.realms) > 0 {
		return c.realms, nil
	}

	allRealms, err := c.keycloakClient.GetAllRealms(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch all realms: %w", err)
	}

	realmNames := make([]string, 0, len(allRealms))
	for _, realm := range allRealms {
		if realm.Enabled {
			realmNames = append(realmNames, realm.Realm)
		}
	}

	return realmNames, nil
}

// checkRealm checks all security configurations for a realm.
func (c *RealmSecurityCheck) checkRealm(realm *keycloakadmin.RealmRepresentation) []*domain.Alert {
	var alerts []*domain.Alert

	// Check SSL requirement
	if c.config.CheckSSLRequired {
		if alert := c.checkSSLRequired(realm); alert != nil {
			alerts = append(alerts, alert)
		}
	}

	// Check brute force protection
	if c.config.CheckBruteForce {
		if alert := c.checkBruteForceProtection(realm); alert != nil {
			alerts = append(alerts, alert)
		}
	}

	// Check password policy
	if c.config.CheckPasswordPolicy {
		if policyAlerts := c.checkPasswordPolicy(realm); len(policyAlerts) > 0 {
			alerts = append(alerts, policyAlerts...)
		}
	}

	// Check email verification
	if c.config.CheckEmailVerification {
		if alert := c.checkEmailVerification(realm); alert != nil {
			alerts = append(alerts, alert)
		}
	}

	// Check admin events
	if c.config.CheckAdminEvents {
		if alert := c.checkAdminEvents(realm); alert != nil {
			alerts = append(alerts, alert)
		}
	}

	// Check user events
	if c.config.CheckUserEvents {
		if alert := c.checkUserEvents(realm); alert != nil {
			alerts = append(alerts, alert)
		}
	}

	// Check duplicate emails
	if c.config.CheckDuplicateEmails {
		if alert := c.checkDuplicateEmails(realm); alert != nil {
			alerts = append(alerts, alert)
		}
	}

	return alerts
}

// checkSSLRequired checks if SSL is required for the realm.
func (c *RealmSecurityCheck) checkSSLRequired(realm *keycloakadmin.RealmRepresentation) *domain.Alert {
	if realm.SslRequired != "all" && realm.SslRequired != "external" {
		severity := domain.AlertSeverityWarning
		if realm.SslRequired == "none" {
			severity = domain.AlertSeverityCritical
		}

		return c.createAlert(
			realm,
			"ssl_not_required",
			"SSL Not Required",
			severity,
			fmt.Sprintf("Realm '%s' does not require SSL for all connections (current: %s). Authentication traffic may not be encrypted.", realm.Realm, realm.SslRequired),
			fmt.Sprintf("Enable SSL requirement for realm '%s'. Navigate to Realm Settings > General and set 'Require SSL' to 'All requests'.", realm.Realm),
			map[string]interface{}{
				"ssl_required": realm.SslRequired,
			},
		)
	}
	return nil
}

// checkBruteForceProtection checks if brute force protection is enabled.
func (c *RealmSecurityCheck) checkBruteForceProtection(realm *keycloakadmin.RealmRepresentation) *domain.Alert {
	if !realm.BruteForceProtected {
		return c.createAlert(
			realm,
			"brute_force_disabled",
			"Brute Force Protection Disabled",
			domain.AlertSeverityWarning,
			fmt.Sprintf("Realm '%s' has brute force protection disabled. The realm is vulnerable to password guessing attacks.", realm.Realm),
			fmt.Sprintf("Enable brute force protection for realm '%s'. Navigate to Realm Settings > Security Defenses > Brute Force Detection and enable it.", realm.Realm),
			map[string]interface{}{
				"brute_force_protected": realm.BruteForceProtected,
			},
		)
	}
	return nil
}

// checkPasswordPolicy checks password policy configuration.
func (c *RealmSecurityCheck) checkPasswordPolicy(realm *keycloakadmin.RealmRepresentation) []*domain.Alert {
	var alerts []*domain.Alert

	// Parse password policy
	policy := parsePasswordPolicy(realm.PasswordPolicy)

	// Check minimum length
	if length, exists := policy["length"]; exists {
		if lenInt, ok := parseIntValue(length); ok && lenInt < c.config.MinPasswordLength {
			alerts = append(alerts, c.createAlert(
				realm,
				"weak_password_length",
				"Weak Password Length Requirement",
				domain.AlertSeverityWarning,
				fmt.Sprintf("Realm '%s' requires only %d character passwords. Recommended minimum is %d characters.", realm.Realm, lenInt, c.config.MinPasswordLength),
				fmt.Sprintf("Increase minimum password length for realm '%s'. Navigate to Realm Settings > Authentication > Policies tab and set 'Minimum Length' to at least %d.", realm.Realm, c.config.MinPasswordLength),
				map[string]interface{}{
					"current_min_length":     lenInt,
					"recommended_min_length": c.config.MinPasswordLength,
					"password_policy":        realm.PasswordPolicy,
				},
			))
		}
	} else {
		// No minimum length set
		alerts = append(alerts, c.createAlert(
			realm,
			"no_password_length",
			"No Password Length Requirement",
			domain.AlertSeverityWarning,
			fmt.Sprintf("Realm '%s' has no minimum password length requirement.", realm.Realm),
			fmt.Sprintf("Set minimum password length for realm '%s'. Navigate to Realm Settings > Authentication > Policies tab and set 'Minimum Length' to at least %d.", realm.Realm, c.config.MinPasswordLength),
			map[string]interface{}{
				"recommended_min_length": c.config.MinPasswordLength,
				"password_policy":        realm.PasswordPolicy,
			},
		))
	}

	// Check complexity requirements if configured
	if c.config.RequirePasswordComplexity {
		missing := []string{}
		if _, exists := policy["upperCase"]; !exists {
			missing = append(missing, "uppercase letters")
		}
		if _, exists := policy["lowerCase"]; !exists {
			missing = append(missing, "lowercase letters")
		}
		if _, exists := policy["digits"]; !exists {
			missing = append(missing, "digits")
		}
		if _, exists := policy["specialChars"]; !exists {
			missing = append(missing, "special characters")
		}

		if len(missing) > 0 {
			alerts = append(alerts, c.createAlert(
				realm,
				"weak_password_complexity",
				"Weak Password Complexity Requirements",
				domain.AlertSeverityWarning,
				fmt.Sprintf("Realm '%s' password policy is missing complexity requirements: %s.", realm.Realm, strings.Join(missing, ", ")),
				fmt.Sprintf("Add password complexity requirements for realm '%s'. Navigate to Realm Settings > Authentication > Policies tab and add: %s.", realm.Realm, strings.Join(missing, ", ")),
				map[string]interface{}{
					"missing_requirements": missing,
					"password_policy":      realm.PasswordPolicy,
				},
			))
		}
	}

	return alerts
}

// checkEmailVerification checks if email verification is properly configured.
func (c *RealmSecurityCheck) checkEmailVerification(realm *keycloakadmin.RealmRepresentation) *domain.Alert {
	if realm.RegistrationAllowed && !realm.VerifyEmail {
		return c.createAlert(
			realm,
			"registration_without_verification",
			"Self-Registration Without Email Verification",
			domain.AlertSeverityWarning,
			fmt.Sprintf("Realm '%s' allows self-registration but does not require email verification. Users can create accounts without proving email ownership.", realm.Realm),
			fmt.Sprintf("Enable email verification for realm '%s'. Navigate to Realm Settings > Login and enable 'Verify email'.", realm.Realm),
			map[string]interface{}{
				"registration_allowed": realm.RegistrationAllowed,
				"verify_email":         realm.VerifyEmail,
			},
		)
	}
	return nil
}

// checkAdminEvents checks if admin event logging is enabled.
func (c *RealmSecurityCheck) checkAdminEvents(realm *keycloakadmin.RealmRepresentation) *domain.Alert {
	if !realm.AdminEventsEnabled {
		return c.createAlert(
			realm,
			"admin_events_disabled",
			"Admin Event Logging Disabled",
			domain.AlertSeverityWarning,
			fmt.Sprintf("Realm '%s' has admin event logging disabled. There is no audit trail for administrative changes.", realm.Realm),
			fmt.Sprintf("Enable admin event logging for realm '%s'. Navigate to Realm Settings > Events and enable 'Save Events' under Admin Events Settings.", realm.Realm),
			map[string]interface{}{
				"admin_events_enabled":         realm.AdminEventsEnabled,
				"admin_events_details_enabled": realm.AdminEventsDetailsEnabled,
			},
		)
	}
	return nil
}

// checkUserEvents checks if user event logging is configured.
func (c *RealmSecurityCheck) checkUserEvents(realm *keycloakadmin.RealmRepresentation) *domain.Alert {
	if !realm.EventsEnabled {
		return c.createAlert(
			realm,
			"user_events_disabled",
			"User Event Logging Disabled",
			domain.AlertSeverityInfo,
			fmt.Sprintf("Realm '%s' has user event logging disabled. Login attempts and authentication events are not being tracked.", realm.Realm),
			fmt.Sprintf("Enable user event logging for realm '%s'. Navigate to Realm Settings > Events and enable 'Save Events' under User Events Settings.", realm.Realm),
			map[string]interface{}{
				"events_enabled":      realm.EventsEnabled,
				"events_listeners":    realm.EventsListeners,
				"enabled_event_types": realm.EnabledEventTypes,
			},
		)
	} else if len(realm.EventsListeners) == 0 {
		return c.createAlert(
			realm,
			"no_event_listeners",
			"No Event Listeners Configured",
			domain.AlertSeverityWarning,
			fmt.Sprintf("Realm '%s' has events enabled but no event listeners configured. Events may not be saved.", realm.Realm),
			fmt.Sprintf("Configure event listeners for realm '%s'. Navigate to Realm Settings > Events and add 'jboss-logging' or database listener.", realm.Realm),
			map[string]interface{}{
				"events_enabled":   realm.EventsEnabled,
				"events_listeners": realm.EventsListeners,
			},
		)
	}
	return nil
}

// checkDuplicateEmails checks if duplicate emails are allowed.
func (c *RealmSecurityCheck) checkDuplicateEmails(realm *keycloakadmin.RealmRepresentation) *domain.Alert {
	if realm.DuplicateEmailsAllowed {
		return c.createAlert(
			realm,
			"duplicate_emails_allowed",
			"Duplicate Email Addresses Allowed",
			domain.AlertSeverityWarning,
			fmt.Sprintf("Realm '%s' allows duplicate email addresses. This can cause identity confusion and security issues.", realm.Realm),
			fmt.Sprintf("Disable duplicate emails for realm '%s'. Navigate to Realm Settings > Login and disable 'Duplicate emails'.", realm.Realm),
			map[string]interface{}{
				"duplicate_emails_allowed": realm.DuplicateEmailsAllowed,
			},
		)
	}
	return nil
}

// createAlert creates a configuration alert.
func (c *RealmSecurityCheck) createAlert(
	realm *keycloakadmin.RealmRepresentation,
	alertType string,
	title string,
	severity domain.AlertSeverity,
	description string,
	recommendation string,
	additionalMetadata map[string]interface{},
) *domain.Alert {
	alertID := c.generateAlertID(realm.Realm, alertType)

	metadata := map[string]interface{}{
		"realm":      realm.Realm,
		"realm_name": realm.DisplayName,
		"alert_type": alertType,
	}
	for k, v := range additionalMetadata {
		metadata[k] = v
	}

	metadataJSON, _ := json.Marshal(metadata)

	return &domain.Alert{
		TenantID:       c.tenantID,
		AlertID:        alertID,
		Type:           domain.AlertTypeRealm,
		Severity:       severity,
		Status:         domain.AlertStatusActive,
		Title:          title,
		Description:    description,
		ResourceType:   "realm",
		ResourceID:     realm.ID,
		ResourceName:   realm.Realm,
		RealmName:      realm.Realm,
		CheckType:      c.GetCheckType(),
		Recommendation: recommendation,
		Metadata:       string(metadataJSON),
	}
}

// generateAlertID generates a stable, unique alert ID for deduplication.
// Includes tenantID so two tenants with a same-named realm never collide —
// alert_id was historically treated as globally unique in the database
// despite deduplication being meant to happen per-tenant.
func (c *RealmSecurityCheck) generateAlertID(realmName, alertType string) string {
	input := fmt.Sprintf("%s:%s:%s:%s", c.tenantID, c.GetCheckType(), realmName, alertType)
	hash := sha256.Sum256([]byte(input))
	return fmt.Sprintf("realm-security-%x", hash[:16])
}

// parsePasswordPolicy parses the password policy string into a map.
func parsePasswordPolicy(policy string) map[string]string {
	result := make(map[string]string)
	if policy == "" {
		return result
	}

	// Policy format: "length(8) and digits(1) and upperCase(1)"
	parts := strings.Split(policy, " and ")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if idx := strings.Index(part, "("); idx > 0 {
			name := part[:idx]
			value := strings.TrimSuffix(part[idx+1:], ")")
			result[name] = value
		}
	}

	return result
}

// parseIntValue attempts to parse an integer from a string.
func parseIntValue(s string) (int, bool) {
	var val int
	_, err := fmt.Sscanf(s, "%d", &val)
	return val, err == nil
}
