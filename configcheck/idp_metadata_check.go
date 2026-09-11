package configcheck

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/pkg/keycloakadmin"
)

// IdentityProviderMetadataCheck checks if SAML identity providers have
// the metadata descriptor URL configured and validates certificate expiration.
type IdentityProviderMetadataCheck struct {
	keycloakClient    KeycloakClient
	logger            *logger.Logger
	realms            []string
	tenantID          string
	certWarningDays   int
	metadataFetcher   *SAMLMetadataFetcher
	checkCertificates bool
}

// NewIdentityProviderMetadataCheck creates a new identity provider metadata checker.
func NewIdentityProviderMetadataCheck(
	keycloakClient KeycloakClient,
	log *logger.Logger,
	realms []string,
	tenantID string,
	certWarningDays int,
	certCheckTimeout time.Duration,
) *IdentityProviderMetadataCheck {
	// Default values
	if certWarningDays <= 0 {
		certWarningDays = 30 // Default to 30 days
	}
	if certCheckTimeout <= 0 {
		certCheckTimeout = 30 * time.Second // Default to 30 seconds
	}

	return &IdentityProviderMetadataCheck{
		keycloakClient:    keycloakClient,
		logger:            log.WithComponent("idp_metadata_check"),
		realms:            realms,
		tenantID:          tenantID,
		certWarningDays:   certWarningDays,
		metadataFetcher:   NewSAMLMetadataFetcher(certCheckTimeout, log),
		checkCertificates: true,
	}
}

// GetCheckType returns the unique identifier for this check.
func (c *IdentityProviderMetadataCheck) GetCheckType() string {
	return "identity_provider_metadata"
}

// GetDescription returns a human-readable description of this check.
func (c *IdentityProviderMetadataCheck) GetDescription() string {
	return "Checks if SAML identity providers have the metadata descriptor URL configured and validates certificate expiration"
}

// Execute performs the configuration check.
func (c *IdentityProviderMetadataCheck) Execute(ctx context.Context) ([]*domain.Alert, error) {
	c.logger.Debug("Executing identity provider metadata check")

	// Get realms to check
	realms, err := c.getRealmsToCheck(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get realms: %w", err)
	}

	var alerts []*domain.Alert

	// Check each realm
	for _, realm := range realms {
		realmAlerts, err := c.checkRealm(ctx, realm)
		if err != nil {
			c.logger.Error("Failed to check realm",
				logger.Str("realm", realm),
				logger.Err(err))
			continue
		}

		alerts = append(alerts, realmAlerts...)
	}

	c.logger.Info("Identity provider metadata check completed",
		logger.Int("total_alerts", len(alerts)))

	return alerts, nil
}

// getRealmsToCheck returns the list of realms to check.
func (c *IdentityProviderMetadataCheck) getRealmsToCheck(ctx context.Context) ([]string, error) {
	if len(c.realms) > 0 {
		return c.realms, nil
	}

	// Fetch all realms
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

// checkRealm checks all identity providers in a realm.
func (c *IdentityProviderMetadataCheck) checkRealm(ctx context.Context, realmName string) ([]*domain.Alert, error) {
	// Get identity providers for this realm
	idps, err := c.keycloakClient.GetIdentityProviders(ctx, realmName)
	if err != nil {
		return nil, fmt.Errorf("failed to get identity providers: %w", err)
	}

	var alerts []*domain.Alert

	for _, idp := range idps {
		// Check if this is a SAML identity provider
		if idp.ProviderID != "saml" {
			c.logger.Debug("Skipping non-SAML identity provider",
				logger.Str("realm", realmName),
				logger.Str("idp_alias", idp.Alias),
				logger.Str("provider_id", idp.ProviderID))
			continue
		}

		// Check if metadata descriptor URL is configured
		metadataURL, exists := idp.Config["metadataDescriptorUrl"]
		if !exists || metadataURL == "" {
			alert := c.createMissingMetadataAlert(realmName, idp)
			alerts = append(alerts, alert)

			c.logger.Warn("SAML identity provider missing metadata descriptor URL",
				logger.Str("realm", realmName),
				logger.Str("idp_alias", idp.Alias))
			continue
		}

		// If certificate checking is enabled, validate certificates
		if c.checkCertificates {
			certAlerts := c.checkCertificateExpiration(ctx, realmName, idp, metadataURL)
			alerts = append(alerts, certAlerts...)
		}
	}

	return alerts, nil
}

// getIDPDisplayName returns the display name if available, otherwise falls back to alias.
func (c *IdentityProviderMetadataCheck) getIDPDisplayName(idp *keycloakadmin.IdentityProviderRepresentation) string {
	if idp.DisplayName != "" {
		return idp.DisplayName
	}
	return idp.Alias
}

// createMissingMetadataAlert creates a configuration alert for a misconfigured identity provider.
func (c *IdentityProviderMetadataCheck) createMissingMetadataAlert(realmName string, idp *keycloakadmin.IdentityProviderRepresentation) *domain.Alert {
	// Generate a stable alert ID for deduplication
	alertID := c.generateAlertID(realmName, idp.Alias)
	idpName := c.getIDPDisplayName(idp)

	// Create metadata with additional context
	metadata := map[string]interface{}{
		"realm":       realmName,
		"idp_alias":   idp.Alias,
		"idp_name":    idp.DisplayName,
		"provider_id": idp.ProviderID,
		"enabled":     idp.Enabled,
		"internal_id": idp.InternalID,
	}

	metadataJSON, _ := json.Marshal(metadata)

	return &domain.Alert{
		TenantID:       c.tenantID,
		AlertID:        alertID,
		Type:           domain.AlertTypeIdentityProvider,
		Severity:       domain.AlertSeverityWarning,
		Status:         domain.AlertStatusActive,
		Title:          fmt.Sprintf("SAML Identity Provider '%s' (alias: %s) missing metadata descriptor URL", idpName, idp.Alias),
		Description:    fmt.Sprintf("The SAML identity provider '%s' (alias: %s) in realm '%s' does not have a metadata descriptor URL configured. This may cause issues with SAML federation and prevent automatic metadata updates.", idpName, idp.Alias, realmName),
		ResourceType:   "identity_provider",
		ResourceID:     idp.InternalID,
		ResourceName:   idp.Alias,
		RealmName:      realmName,
		CheckType:      c.GetCheckType(),
		Recommendation: fmt.Sprintf("Configure the 'metadataDescriptorUrl' in the SAML identity provider settings for '%s'. Navigate to Realm Settings > Identity Providers > %s > SAML Config and provide the metadata descriptor URL from your SAML provider.", idpName, idp.Alias),
		Metadata:       string(metadataJSON),
	}
}

// generateAlertID generates a stable, unique alert ID for deduplication.
// Includes tenantID so two tenants with a same-named realm/IDP never collide
// — alert_id was historically treated as globally unique in the database
// despite deduplication being meant to happen per-tenant.
func (c *IdentityProviderMetadataCheck) generateAlertID(realmName, idpAlias string) string {
	// Use a combination of tenant, check type, realm, and IDP alias
	input := fmt.Sprintf("%s:%s:%s:%s", c.tenantID, c.GetCheckType(), realmName, idpAlias)
	hash := sha256.Sum256([]byte(input))
	return fmt.Sprintf("idp-metadata-%x", hash[:16]) // Use first 16 bytes of hash
}

// generateCertAlertID generates a stable alert ID for certificate expiration alerts.
func (c *IdentityProviderMetadataCheck) generateCertAlertID(realmName, idpAlias, certSerialNum string) string {
	// Include certificate serial number to track specific certificates
	input := fmt.Sprintf("%s:%s:cert:%s:%s:%s", c.tenantID, c.GetCheckType(), realmName, idpAlias, certSerialNum)
	hash := sha256.Sum256([]byte(input))
	return fmt.Sprintf("idp-cert-%x", hash[:16])
}

// checkCertificateExpiration checks if certificates in the SAML metadata are expired or expiring soon.
func (c *IdentityProviderMetadataCheck) checkCertificateExpiration(
	ctx context.Context,
	realmName string,
	idp *keycloakadmin.IdentityProviderRepresentation,
	metadataURL string,
) []*domain.Alert {
	var alerts []*domain.Alert

	c.logger.Debug("Checking certificate expiration for SAML IDP",
		logger.Str("realm", realmName),
		logger.Str("idp_alias", idp.Alias),
		logger.Str("metadata_url", metadataURL))

	// Fetch and parse certificates from metadata
	certificates, err := c.metadataFetcher.FetchAndParseCertificates(ctx, metadataURL)
	if err != nil {
		c.logger.Warn("Failed to fetch or parse SAML metadata certificates",
			logger.Str("realm", realmName),
			logger.Str("idp_alias", idp.Alias),
			logger.Str("metadata_url", metadataURL),
			logger.Err(err))

		// Create alert for metadata fetch failure
		alert := c.createMetadataFetchErrorAlert(realmName, idp, metadataURL, err)
		return []*domain.Alert{alert}
	}

	// Check each certificate for expiration
	now := time.Now()
	for _, cert := range certificates {
		status := CheckCertificateExpiration(cert, c.certWarningDays, now)

		if status.IsExpired {
			alert := c.createExpiredCertificateAlert(realmName, idp, metadataURL, cert, status)
			alerts = append(alerts, alert)

			c.logger.Warn("SAML IDP certificate is expired",
				logger.Str("realm", realmName),
				logger.Str("idp_alias", idp.Alias),
				logger.Str("subject", cert.Subject),
				logger.Time("expired_at", cert.NotAfter))
		} else if status.IsExpiringSoon {
			alert := c.createExpiringSoonCertificateAlert(realmName, idp, metadataURL, cert, status)
			alerts = append(alerts, alert)

			c.logger.Warn("SAML IDP certificate is expiring soon",
				logger.Str("realm", realmName),
				logger.Str("idp_alias", idp.Alias),
				logger.Str("subject", cert.Subject),
				logger.Time("expires_at", cert.NotAfter),
				logger.Int("days_until_expiry", status.DaysUntilExpiry))
		} else {
			c.logger.Debug("SAML IDP certificate is valid",
				logger.Str("realm", realmName),
				logger.Str("idp_alias", idp.Alias),
				logger.Str("subject", cert.Subject),
				logger.Time("expires_at", cert.NotAfter),
				logger.Int("days_until_expiry", status.DaysUntilExpiry))
		}
	}

	return alerts
}

// createExpiredCertificateAlert creates an alert for an expired certificate.
func (c *IdentityProviderMetadataCheck) createExpiredCertificateAlert(
	realmName string,
	idp *keycloakadmin.IdentityProviderRepresentation,
	metadataURL string,
	cert *CertificateInfo,
	status CertificateExpirationStatus,
) *domain.Alert {
	alertID := c.generateCertAlertID(realmName, idp.Alias, cert.SerialNum)
	idpName := c.getIDPDisplayName(idp)

	metadata := map[string]interface{}{
		"realm":             realmName,
		"idp_alias":         idp.Alias,
		"idp_name":          idp.DisplayName,
		"provider_id":       idp.ProviderID,
		"internal_id":       idp.InternalID,
		"metadata_url":      metadataURL,
		"cert_subject":      cert.Subject,
		"cert_issuer":       cert.Issuer,
		"cert_serial":       cert.SerialNum,
		"cert_use":          cert.Use,
		"cert_not_before":   cert.NotBefore.Format(time.RFC3339),
		"cert_not_after":    cert.NotAfter.Format(time.RFC3339),
		"days_since_expiry": status.DaysUntilExpiry, // Negative value for expired certs
	}

	metadataJSON, _ := json.Marshal(metadata)

	return &domain.Alert{
		TenantID:       c.tenantID,
		AlertID:        alertID,
		Type:           domain.AlertTypeIdentityProvider,
		Severity:       domain.AlertSeverityCritical,
		Status:         domain.AlertStatusActive,
		Title:          fmt.Sprintf("SAML Identity Provider '%s' certificate has expired", idpName),
		Description:    fmt.Sprintf("The SAML identity provider '%s' (alias: %s) in realm '%s' has an expired certificate. The certificate expired on %s (%d days ago). SAML authentication will fail until the certificate is renewed.", idpName, idp.Alias, realmName, cert.NotAfter.Format("2006-01-02"), -status.DaysUntilExpiry),
		ResourceType:   "identity_provider",
		ResourceID:     idp.InternalID,
		ResourceName:   idp.Alias,
		RealmName:      realmName,
		CheckType:      c.GetCheckType(),
		Recommendation: fmt.Sprintf("Update the SAML metadata for '%s' with a renewed certificate. Contact your SAML identity provider administrator to obtain updated metadata with a valid certificate. Then update the metadata descriptor URL or manually upload the new metadata in Keycloak.", idpName),
		Metadata:       string(metadataJSON),
	}
}

// createExpiringSoonCertificateAlert creates an alert for a certificate expiring soon.
func (c *IdentityProviderMetadataCheck) createExpiringSoonCertificateAlert(
	realmName string,
	idp *keycloakadmin.IdentityProviderRepresentation,
	metadataURL string,
	cert *CertificateInfo,
	status CertificateExpirationStatus,
) *domain.Alert {
	alertID := c.generateCertAlertID(realmName, idp.Alias, cert.SerialNum)
	idpName := c.getIDPDisplayName(idp)

	metadata := map[string]interface{}{
		"realm":             realmName,
		"idp_alias":         idp.Alias,
		"idp_name":          idp.DisplayName,
		"provider_id":       idp.ProviderID,
		"internal_id":       idp.InternalID,
		"metadata_url":      metadataURL,
		"cert_subject":      cert.Subject,
		"cert_issuer":       cert.Issuer,
		"cert_serial":       cert.SerialNum,
		"cert_use":          cert.Use,
		"cert_not_before":   cert.NotBefore.Format(time.RFC3339),
		"cert_not_after":    cert.NotAfter.Format(time.RFC3339),
		"days_until_expiry": status.DaysUntilExpiry,
		"warning_threshold": c.certWarningDays,
	}

	metadataJSON, _ := json.Marshal(metadata)

	return &domain.Alert{
		TenantID:       c.tenantID,
		AlertID:        alertID,
		Type:           domain.AlertTypeIdentityProvider,
		Severity:       domain.AlertSeverityWarning,
		Status:         domain.AlertStatusActive,
		Title:          fmt.Sprintf("SAML Identity Provider '%s' certificate expiring soon", idpName),
		Description:    fmt.Sprintf("The SAML identity provider '%s' (alias: %s) in realm '%s' has a certificate that will expire in %d days (on %s). Plan to renew the certificate before it expires to prevent SAML authentication failures.", idpName, idp.Alias, realmName, status.DaysUntilExpiry, cert.NotAfter.Format("2006-01-02")),
		ResourceType:   "identity_provider",
		ResourceID:     idp.InternalID,
		ResourceName:   idp.Alias,
		RealmName:      realmName,
		CheckType:      c.GetCheckType(),
		Recommendation: fmt.Sprintf("Contact your SAML identity provider administrator to obtain updated metadata with a renewed certificate for '%s'. Update the metadata in Keycloak before the certificate expires on %s.", idpName, cert.NotAfter.Format("2006-01-02")),
		Metadata:       string(metadataJSON),
	}
}

// createMetadataFetchErrorAlert creates an alert for metadata fetch errors.
func (c *IdentityProviderMetadataCheck) createMetadataFetchErrorAlert(
	realmName string,
	idp *keycloakadmin.IdentityProviderRepresentation,
	metadataURL string,
	err error,
) *domain.Alert {
	// Use different ID for fetch errors to distinguish from cert issues
	alertID := c.generateCertAlertID(realmName, idp.Alias, "fetch-error")
	idpName := c.getIDPDisplayName(idp)

	metadata := map[string]interface{}{
		"realm":        realmName,
		"idp_alias":    idp.Alias,
		"idp_name":     idp.DisplayName,
		"provider_id":  idp.ProviderID,
		"internal_id":  idp.InternalID,
		"metadata_url": metadataURL,
		"error":        err.Error(),
	}

	metadataJSON, _ := json.Marshal(metadata)

	return &domain.Alert{
		TenantID:       c.tenantID,
		AlertID:        alertID,
		Type:           domain.AlertTypeIdentityProvider,
		Severity:       domain.AlertSeverityWarning,
		Status:         domain.AlertStatusActive,
		Title:          fmt.Sprintf("SAML Identity Provider '%s' metadata fetch failed", idpName),
		Description:    fmt.Sprintf("Failed to fetch or parse SAML metadata for identity provider '%s' (alias: %s) in realm '%s' from URL: %s. Error: %s. This may indicate the metadata URL is incorrect, unreachable, or the metadata format is invalid.", idpName, idp.Alias, realmName, metadataURL, err.Error()),
		ResourceType:   "identity_provider",
		ResourceID:     idp.InternalID,
		ResourceName:   idp.Alias,
		RealmName:      realmName,
		CheckType:      c.GetCheckType(),
		Recommendation: fmt.Sprintf("Verify the metadata descriptor URL for '%s' is correct and accessible. Check network connectivity, firewall rules, and that the SAML provider's metadata endpoint is operational. Ensure the metadata follows standard SAML 2.0 format.", idpName),
		Metadata:       string(metadataJSON),
	}
}
