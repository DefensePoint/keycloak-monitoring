package keycloakadmin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
)

// identityProviderPageSize is how many identity providers one request asks
// for. Keycloak returns at most 100 when not asked for a number.
const identityProviderPageSize = 100

// GetIdentityProviders returns every identity provider in a realm, read page
// by page: without paging, Keycloak lists only the first 100.
func (c *Client) GetIdentityProviders(ctx context.Context, realmName string) ([]*IdentityProviderRepresentation, error) {
	realmSeg, err := pathSegment(realmName)
	if err != nil {
		return nil, err
	}

	var all []*IdentityProviderRepresentation
	for first := 0; ; first += identityProviderPageSize {
		path := fmt.Sprintf("/admin/realms/%s/identity-provider/instances?first=%d&max=%d", realmSeg, first, identityProviderPageSize)
		page, err := c.getIdentityProviderPage(ctx, path)
		if err != nil {
			return nil, err
		}
		// A server that ignores first and max returns every provider on each
		// request: a longer page, or one that starts over, is the whole list.
		if len(page) > identityProviderPageSize {
			return page, nil
		}
		if first > 0 && len(page) > 0 && len(all) > 0 && page[0].Alias == all[0].Alias {
			return all, nil
		}
		all = append(all, page...)
		if len(page) < identityProviderPageSize {
			return all, nil
		}
	}
}

func (c *Client) getIdentityProviderPage(ctx context.Context, path string) ([]*IdentityProviderRepresentation, error) {
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
		return nil, fmt.Errorf("failed to get identity providers, status: %d%s", resp.StatusCode, c.replyDetail("get identity providers", resp.StatusCode, body))
	}

	var identityProviders []*IdentityProviderRepresentation
	if err := json.Unmarshal(body, &identityProviders); err != nil {
		return nil, fmt.Errorf("failed to parse identity providers: %w", err)
	}

	return identityProviders, nil
}
