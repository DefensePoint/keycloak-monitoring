package keycloakadmin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
)

// GetClients returns a list of clients in a realm
// Explicitly sets max=1000 to ensure all clients are returned (Keycloak default may vary)
func (c *Client) GetClients(ctx context.Context, realmName string) ([]*ClientRepresentation, error) {
	path := fmt.Sprintf("/admin/realms/%s/clients?first=0&max=1000", realmName)

	resp, err := c.get(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("failed to get clients: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to get clients, status: %d, body: %s", resp.StatusCode, string(body))
	}

	var clients []*ClientRepresentation
	if err := json.Unmarshal(body, &clients); err != nil {
		return nil, fmt.Errorf("failed to parse clients: %w", err)
	}

	return clients, nil
}
