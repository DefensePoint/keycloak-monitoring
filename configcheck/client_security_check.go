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

// ClientSecurityCheckConfig contains configuration for client security checks.
type ClientSecurityCheckConfig struct {
	CheckRedirectURIs       bool
	CheckPublicClients      bool
	CheckDirectAccessGrants bool
	CheckClientSecret       bool
	CheckImplicitFlow       bool
	CheckStandardFlow       bool
	AllowLocalhostRedirects bool
	ProductionEnvironment   bool
}

// ClientSecurityCheck checks client-level security configurations.
type ClientSecurityCheck struct {
	keycloakClient KeycloakClient
	logger         *logger.Logger
	realms         []string
	tenantID       string
	config         ClientSecurityCheckConfig
}

// NewClientSecurityCheck creates a new client security checker.
func NewClientSecurityCheck(
	keycloakClient KeycloakClient,
	log *logger.Logger,
	realms []string,
	tenantID string,
	config ClientSecurityCheckConfig,
) *ClientSecurityCheck {
	return &ClientSecurityCheck{
		keycloakClient: keycloakClient,
		logger:         log.WithComponent("client_security_check"),
		realms:         realms,
		tenantID:       tenantID,
		config:         config,
	}
}

// GetCheckType returns the unique identifier for this check.
func (c *ClientSecurityCheck) GetCheckType() string {
	return "client_security"
}

// GetDescription returns a human-readable description of this check.
func (c *ClientSecurityCheck) GetDescription() string {
	return "Checks client security configurations including redirect URIs, public client settings, and flow configurations"
}

// Execute performs the configuration check.
func (c *ClientSecurityCheck) Execute(ctx context.Context) ([]*domain.Alert, error) {
	c.logger.Debug("Executing client security check")

	realms, err := c.getRealmsToCheck(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get realms: %w", err)
	}

	var alerts []*domain.Alert

	for _, realmName := range realms {
		clients, err := c.keycloakClient.GetClients(ctx, realmName)
		if err != nil {
			c.logger.Error("Failed to get clients",
				logger.Str("realm", realmName),
				logger.Err(err))
			continue
		}

		for _, client := range clients {
			clientAlerts := c.checkClient(realmName, client)
			alerts = append(alerts, clientAlerts...)
		}
	}

	c.logger.Info("Client security check completed",
		logger.Int("total_alerts", len(alerts)))

	return alerts, nil
}

// getRealmsToCheck returns the list of realms to check.
func (c *ClientSecurityCheck) getRealmsToCheck(ctx context.Context) ([]string, error) {
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

// checkClient checks all security configurations for a client.
func (c *ClientSecurityCheck) checkClient(realmName string, client *keycloakadmin.ClientRepresentation) []*domain.Alert {
	var alerts []*domain.Alert

	// Skip built-in clients
	if c.isBuiltInClient(client.ClientID) {
		return alerts
	}

	// Check redirect URIs
	if c.config.CheckRedirectURIs {
		if redirectAlerts := c.checkRedirectURIs(realmName, client); len(redirectAlerts) > 0 {
			alerts = append(alerts, redirectAlerts...)
		}
	}

	// Check public client configurations
	if c.config.CheckPublicClients {
		if alert := c.checkPublicClientConfig(realmName, client); alert != nil {
			alerts = append(alerts, alert)
		}
	}

	// Check direct access grants
	if c.config.CheckDirectAccessGrants {
		if alert := c.checkDirectAccessGrants(realmName, client); alert != nil {
			alerts = append(alerts, alert)
		}
	}

	// Check client secret for confidential clients
	if c.config.CheckClientSecret {
		if alert := c.checkClientSecret(realmName, client); alert != nil {
			alerts = append(alerts, alert)
		}
	}

	// Check implicit flow
	if c.config.CheckImplicitFlow {
		if alert := c.checkImplicitFlow(realmName, client); alert != nil {
			alerts = append(alerts, alert)
		}
	}

	// Check standard flow
	if c.config.CheckStandardFlow {
		if alert := c.checkStandardFlow(realmName, client); alert != nil {
			alerts = append(alerts, alert)
		}
	}

	return alerts
}

// isBuiltInClient checks if a client is a built-in Keycloak client.
func (c *ClientSecurityCheck) isBuiltInClient(clientID string) bool {
	builtInClients := []string{
		"account",
		"account-console",
		"admin-cli",
		"broker",
		"realm-management",
		"security-admin-console",
	}

	for _, builtin := range builtInClients {
		if clientID == builtin {
			return true
		}
	}
	return false
}

// checkRedirectURIs checks for insecure redirect URI configurations.
func (c *ClientSecurityCheck) checkRedirectURIs(realmName string, client *keycloakadmin.ClientRepresentation) []*domain.Alert {
	var alerts []*domain.Alert

	for _, uri := range client.RedirectUris {
		// Check for wildcard URIs
		if strings.Contains(uri, "*") && uri != "http://localhost*" && uri != "https://localhost*" {
			alerts = append(alerts, c.createAlert(
				realmName,
				client,
				"wildcard_redirect_uri",
				"Wildcard Redirect URI",
				domain.AlertSeverityWarning,
				fmt.Sprintf("Client '%s' in realm '%s' has wildcard redirect URI: %s. This creates an open redirect vulnerability.", client.ClientID, realmName, uri),
				fmt.Sprintf("Replace wildcard redirect URI with specific URLs for client '%s'. Navigate to Clients > %s > Settings and update redirect URIs.", client.ClientID, client.ClientID),
				map[string]interface{}{
					"redirect_uri":      uri,
					"all_redirect_uris": client.RedirectUris,
				},
			))
		}

		// Check for localhost in production
		if c.config.ProductionEnvironment && !c.config.AllowLocalhostRedirects {
			if strings.Contains(strings.ToLower(uri), "localhost") || strings.Contains(uri, "127.0.0.1") {
				alerts = append(alerts, c.createAlert(
					realmName,
					client,
					"localhost_redirect_uri",
					"Localhost Redirect URI in Production",
					domain.AlertSeverityWarning,
					fmt.Sprintf("Client '%s' in realm '%s' has localhost redirect URI in production: %s.", client.ClientID, realmName, uri),
					fmt.Sprintf("Remove localhost redirect URI from client '%s' in production. Navigate to Clients > %s > Settings and remove development URIs.", client.ClientID, client.ClientID),
					map[string]interface{}{
						"redirect_uri":      uri,
						"all_redirect_uris": client.RedirectUris,
					},
				))
			}
		}

		// Check for HTTP (non-HTTPS) in production
		if c.config.ProductionEnvironment && strings.HasPrefix(uri, "http://") && !strings.Contains(uri, "localhost") {
			alerts = append(alerts, c.createAlert(
				realmName,
				client,
				"http_redirect_uri",
				"HTTP Redirect URI in Production",
				domain.AlertSeverityWarning,
				fmt.Sprintf("Client '%s' in realm '%s' has non-HTTPS redirect URI: %s. Authentication tokens may be exposed.", client.ClientID, realmName, uri),
				fmt.Sprintf("Update redirect URI to HTTPS for client '%s'. Navigate to Clients > %s > Settings and change to HTTPS.", client.ClientID, client.ClientID),
				map[string]interface{}{
					"redirect_uri":      uri,
					"all_redirect_uris": client.RedirectUris,
				},
			))
		}
	}

	return alerts
}

// checkPublicClientConfig checks public client configurations.
func (c *ClientSecurityCheck) checkPublicClientConfig(realmName string, client *keycloakadmin.ClientRepresentation) *domain.Alert {
	// Public client with service accounts enabled
	if client.PublicClient && client.ServiceAccountsEnabled {
		return c.createAlert(
			realmName,
			client,
			"public_client_service_account",
			"Public Client with Service Accounts",
			domain.AlertSeverityCritical,
			fmt.Sprintf("Client '%s' in realm '%s' is public but has service accounts enabled. This is a security misconfiguration - public clients cannot securely use client credentials.", client.ClientID, realmName),
			fmt.Sprintf("Either make client '%s' confidential or disable service accounts. Navigate to Clients > %s > Settings.", client.ClientID, client.ClientID),
			map[string]interface{}{
				"public_client":            client.PublicClient,
				"service_accounts_enabled": client.ServiceAccountsEnabled,
			},
		)
	}

	return nil
}

// checkDirectAccessGrants checks if direct access grants (ROPC flow) is enabled.
func (c *ClientSecurityCheck) checkDirectAccessGrants(realmName string, client *keycloakadmin.ClientRepresentation) *domain.Alert {
	if client.DirectAccessGrantsEnabled {
		return c.createAlert(
			realmName,
			client,
			"direct_access_grants_enabled",
			"Direct Access Grants Enabled",
			domain.AlertSeverityInfo,
			fmt.Sprintf("Client '%s' in realm '%s' has direct access grants (Resource Owner Password Credentials) enabled. This flow is less secure than authorization code flow.", client.ClientID, realmName),
			fmt.Sprintf("Consider using authorization code flow instead for client '%s'. Navigate to Clients > %s > Settings and disable 'Direct Access Grants'.", client.ClientID, client.ClientID),
			map[string]interface{}{
				"direct_access_grants_enabled": client.DirectAccessGrantsEnabled,
			},
		)
	}

	return nil
}

// checkClientSecret checks if confidential clients have a secret configured.
func (c *ClientSecurityCheck) checkClientSecret(realmName string, client *keycloakadmin.ClientRepresentation) *domain.Alert {
	// Confidential client without secret (service account clients need secrets)
	if !client.PublicClient && client.Secret == "" && !client.BearerOnly {
		return c.createAlert(
			realmName,
			client,
			"missing_client_secret",
			"Confidential Client Missing Secret",
			domain.AlertSeverityCritical,
			fmt.Sprintf("Confidential client '%s' in realm '%s' has no client secret configured.", client.ClientID, realmName),
			fmt.Sprintf("Generate a client secret for '%s'. Navigate to Clients > %s > Credentials and regenerate the secret.", client.ClientID, client.ClientID),
			map[string]interface{}{
				"public_client": client.PublicClient,
				"bearer_only":   client.BearerOnly,
			},
		)
	}

	return nil
}

// checkImplicitFlow checks if implicit flow is enabled.
func (c *ClientSecurityCheck) checkImplicitFlow(realmName string, client *keycloakadmin.ClientRepresentation) *domain.Alert {
	if client.ImplicitFlowEnabled {
		return c.createAlert(
			realmName,
			client,
			"implicit_flow_enabled",
			"Implicit Flow Enabled",
			domain.AlertSeverityWarning,
			fmt.Sprintf("Client '%s' in realm '%s' has implicit flow enabled. Implicit flow is deprecated due to security concerns.", client.ClientID, realmName),
			fmt.Sprintf("Disable implicit flow and use authorization code flow with PKCE for client '%s'. Navigate to Clients > %s > Settings.", client.ClientID, client.ClientID),
			map[string]interface{}{
				"implicit_flow_enabled": client.ImplicitFlowEnabled,
			},
		)
	}

	return nil
}

// checkStandardFlow checks standard flow configuration.
func (c *ClientSecurityCheck) checkStandardFlow(realmName string, client *keycloakadmin.ClientRepresentation) *domain.Alert {
	// Public clients should use PKCE with standard flow
	if client.PublicClient && client.StandardFlowEnabled {
		// Note: We can't check if PKCE is enforced from ClientInfo
		// This would require additional API calls or configuration
		c.logger.Debug("Public client with standard flow detected",
			logger.Str("realm", realmName),
			logger.Str("client_id", client.ClientID))
	}

	return nil
}

// createAlert creates a configuration alert.
func (c *ClientSecurityCheck) createAlert(
	realmName string,
	client *keycloakadmin.ClientRepresentation,
	alertType string,
	title string,
	severity domain.AlertSeverity,
	description string,
	recommendation string,
	additionalMetadata map[string]interface{},
) *domain.Alert {
	alertID := c.generateAlertID(realmName, client.ClientID, alertType)

	metadata := map[string]interface{}{
		"realm":       realmName,
		"client_id":   client.ClientID,
		"client_name": client.Name,
		"alert_type":  alertType,
	}
	for k, v := range additionalMetadata {
		metadata[k] = v
	}

	metadataJSON, _ := json.Marshal(metadata)

	return &domain.Alert{
		TenantID:       c.tenantID,
		AlertID:        alertID,
		Type:           domain.AlertTypeClient,
		Severity:       severity,
		Status:         domain.AlertStatusActive,
		Title:          title,
		Description:    description,
		ResourceType:   "client",
		ResourceID:     client.ID,
		ResourceName:   client.ClientID,
		RealmName:      realmName,
		CheckType:      c.GetCheckType(),
		Recommendation: recommendation,
		Metadata:       string(metadataJSON),
	}
}

// generateAlertID generates a stable, unique alert ID for deduplication.
// Includes tenantID so two tenants with a same-named realm/client never
// collide — alert_id was historically treated as globally unique in the
// database despite deduplication being meant to happen per-tenant.
func (c *ClientSecurityCheck) generateAlertID(realmName, clientID, alertType string) string {
	input := fmt.Sprintf("%s:%s:%s:%s:%s", c.tenantID, c.GetCheckType(), realmName, clientID, alertType)
	hash := sha256.Sum256([]byte(input))
	return fmt.Sprintf("client-security-%x", hash[:16])
}
