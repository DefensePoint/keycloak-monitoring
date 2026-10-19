package keycloakadmin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// GetRealmInfo fetches information about a specific realm
func (c *Client) GetRealmInfo(ctx context.Context, realmName string) (*RealmRepresentation, error) {
	realmSeg, err := pathSegment(realmName)
	if err != nil {
		return nil, err
	}
	path := fmt.Sprintf("/admin/realms/%s", realmSeg)

	resp, err := c.get(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("failed to get realm info: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// A deleted realm is a 404. A realm the client lost its roles in still
	// answers 200, with a stub that has no id; see RealmRepresentation.
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("failed to get realm info, status: %d%s: %w", resp.StatusCode, c.replyDetail("get realm info", resp.StatusCode, body), ErrNotFound)
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to get realm info, status: %d%s", resp.StatusCode, c.replyDetail("get realm info", resp.StatusCode, body))
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
		return nil, fmt.Errorf("failed to get realms, status: %d%s", resp.StatusCode, c.replyDetail("get realms", resp.StatusCode, body))
	}

	var realms []*RealmRepresentation
	if err := json.Unmarshal(body, &realms); err != nil {
		return nil, fmt.Errorf("failed to parse realms: %w", err)
	}

	return realms, nil
}
