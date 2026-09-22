package keycloakadmin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
)

// GetRealmInfo fetches information about a specific realm
func (c *Client) GetRealmInfo(ctx context.Context, realmName string) (*RealmRepresentation, error) {
	path := fmt.Sprintf("/admin/realms/%s", realmName)

	resp, err := c.get(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("failed to get realm info: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to get realm info, status: %d, body: %s", resp.StatusCode, string(body))
	}

	var realm RealmRepresentation
	if err := json.Unmarshal(body, &realm); err != nil {
		return nil, fmt.Errorf("failed to parse realm info: %w", err)
	}

	return &realm, nil
}

// GetAllRealms fetches all realms from Keycloak
func (c *Client) GetAllRealms(ctx context.Context) ([]*RealmRepresentation, error) {
	path := "/admin/realms"

	resp, err := c.get(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("failed to get realms: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to get realms, status: %d, body: %s", resp.StatusCode, string(body))
	}

	var realms []*RealmRepresentation
	if err := json.Unmarshal(body, &realms); err != nil {
		return nil, fmt.Errorf("failed to parse realms: %w", err)
	}

	return realms, nil
}
