package keycloakadmin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
)

// GetRealmRoles fetches all realm-level roles from Keycloak
func (c *Client) GetRealmRoles(ctx context.Context, realmName string) ([]RoleRepresentation, error) {
	path := fmt.Sprintf("/admin/realms/%s/roles", realmName)
	resp, err := c.get(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("failed to get realm roles: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}

	var roles []RoleRepresentation
	if err := json.Unmarshal(body, &roles); err != nil {
		return nil, fmt.Errorf("failed to decode roles: %w", err)
	}

	return roles, nil
}
