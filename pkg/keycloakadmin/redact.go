package keycloakadmin

// RedactedClient is a ClientRepresentation as an API caller may see it. It
// lists the fields it keeps rather than the ones it drops: the secret, the
// attributes and the protocol mappers are left out, because attributes hold the
// rotated secret and SAML private keys, and mapper configs can hold any value.
type RedactedClient struct {
	ID                        string   `json:"id"`
	ClientID                  string   `json:"clientId"`
	Name                      string   `json:"name"`
	Description               string   `json:"description"`
	RootURL                   string   `json:"rootUrl"`
	AdminURL                  string   `json:"adminUrl"`
	BaseURL                   string   `json:"baseUrl"`
	SurrogateAuthRequired     bool     `json:"surrogateAuthRequired"`
	Enabled                   bool     `json:"enabled"`
	AlwaysDisplayInConsole    bool     `json:"alwaysDisplayInConsole"`
	ClientAuthenticatorType   string   `json:"clientAuthenticatorType"`
	RedirectUris              []string `json:"redirectUris"`
	WebOrigins                []string `json:"webOrigins"`
	NotBefore                 int      `json:"notBefore"`
	BearerOnly                bool     `json:"bearerOnly"`
	ConsentRequired           bool     `json:"consentRequired"`
	StandardFlowEnabled       bool     `json:"standardFlowEnabled"`
	ImplicitFlowEnabled       bool     `json:"implicitFlowEnabled"`
	DirectAccessGrantsEnabled bool     `json:"directAccessGrantsEnabled"`
	ServiceAccountsEnabled    bool     `json:"serviceAccountsEnabled"`
	PublicClient              bool     `json:"publicClient"`
	FrontchannelLogout        bool     `json:"frontchannelLogout"`
	Protocol                  string   `json:"protocol"`
	FullScopeAllowed          bool     `json:"fullScopeAllowed"`
	NodeReRegistrationTimeout int      `json:"nodeReRegistrationTimeout"`
}

// RedactClients returns the clients without their credentials, in order.
func RedactClients(clients []*ClientRepresentation) []*RedactedClient {
	if clients == nil {
		return nil
	}
	redacted := make([]*RedactedClient, len(clients))
	for i, client := range clients {
		redacted[i] = RedactClient(client)
	}
	return redacted
}

// RedactClient returns client with only the fields of RedactedClient.
func RedactClient(client *ClientRepresentation) *RedactedClient {
	if client == nil {
		return nil
	}
	return &RedactedClient{
		ID:                        client.ID,
		ClientID:                  client.ClientID,
		Name:                      client.Name,
		Description:               client.Description,
		RootURL:                   client.RootURL,
		AdminURL:                  client.AdminURL,
		BaseURL:                   client.BaseURL,
		SurrogateAuthRequired:     client.SurrogateAuthRequired,
		Enabled:                   client.Enabled,
		AlwaysDisplayInConsole:    client.AlwaysDisplayInConsole,
		ClientAuthenticatorType:   client.ClientAuthenticatorType,
		RedirectUris:              client.RedirectUris,
		WebOrigins:                client.WebOrigins,
		NotBefore:                 client.NotBefore,
		BearerOnly:                client.BearerOnly,
		ConsentRequired:           client.ConsentRequired,
		StandardFlowEnabled:       client.StandardFlowEnabled,
		ImplicitFlowEnabled:       client.ImplicitFlowEnabled,
		DirectAccessGrantsEnabled: client.DirectAccessGrantsEnabled,
		ServiceAccountsEnabled:    client.ServiceAccountsEnabled,
		PublicClient:              client.PublicClient,
		FrontchannelLogout:        client.FrontchannelLogout,
		Protocol:                  client.Protocol,
		FullScopeAllowed:          client.FullScopeAllowed,
		NodeReRegistrationTimeout: client.NodeReRegistrationTimeout,
	}
}
