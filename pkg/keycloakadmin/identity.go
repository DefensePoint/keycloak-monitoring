package keycloakadmin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
)

// GetIdentityProviders returns a list of identity providers in a realm
func (c *Client) GetIdentityProviders(ctx context.Context, realmName string) ([]*IdentityProviderRepresentation, error) {
	path := fmt.Sprintf("/admin/realms/%s/identity-provider/instances", realmName)

	resp, err := c.get(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("failed to get identity providers: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to get identity providers, status: %d, body: %s", resp.StatusCode, string(body))
	}

	var identityProviders []*IdentityProviderRepresentation
	if err := json.Unmarshal(body, &identityProviders); err != nil {
		return nil, fmt.Errorf("failed to parse identity providers: %w", err)
	}

	return identityProviders, nil
}
