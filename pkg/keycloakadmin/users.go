package keycloakadmin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
)

// GetRealmUsers fetches all users from a realm with pagination
func (c *Client) GetRealmUsers(ctx context.Context, realmName string, first, max int) ([]UserRepresentation, error) {
	path := fmt.Sprintf("/admin/realms/%s/users?first=%d&max=%d", realmName, first, max)
	resp, err := c.get(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("failed to get users: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}

	var users []UserRepresentation
	if err := json.NewDecoder(resp.Body).Decode(&users); err != nil {
		// Body already read, use the bytes
		if err := json.Unmarshal(body, &users); err != nil {
			return nil, fmt.Errorf("failed to decode users: %w", err)
		}
	}

	return users, nil
}

// GetUsers returns a list of users in a realm (pointer version for compatibility)
func (c *Client) GetUsers(ctx context.Context, realmName string, first, max int) ([]*UserRepresentation, error) {
	path := fmt.Sprintf("/admin/realms/%s/users?first=%d&max=%d", realmName, first, max)

	resp, err := c.get(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("failed to get users: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to get users, status: %d, body: %s", resp.StatusCode, string(body))
	}

	var users []*UserRepresentation
	if err := json.Unmarshal(body, &users); err != nil {
		return nil, fmt.Errorf("failed to parse users: %w", err)
	}

	return users, nil
}

// GetUserByID fetches a specific user by their ID
func (c *Client) GetUserByID(ctx context.Context, realmName, userID string) (*UserRepresentation, error) {
	path := fmt.Sprintf("/admin/realms/%s/users/%s", realmName, userID)

	resp, err := c.get(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to get user, status: %d, body: %s", resp.StatusCode, string(body))
	}

	var user UserRepresentation
	if err := json.Unmarshal(body, &user); err != nil {
		return nil, fmt.Errorf("failed to parse user: %w", err)
	}

	return &user, nil
}

// GetUserGroups fetches the groups a user belongs to
func (c *Client) GetUserGroups(ctx context.Context, realmName, userID string) ([]GroupRepresentation, error) {
	path := fmt.Sprintf("/admin/realms/%s/users/%s/groups", realmName, userID)

	resp, err := c.get(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("failed to get user groups: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to get user groups, status: %d, body: %s", resp.StatusCode, string(body))
	}

	var groups []GroupRepresentation
	if err := json.Unmarshal(body, &groups); err != nil {
		return nil, fmt.Errorf("failed to parse user groups: %w", err)
	}

	return groups, nil
}

// GetUserRoleMappings fetches the role mappings for a user
func (c *Client) GetUserRoleMappings(ctx context.Context, realmName, userID string) (*RoleMappingsRepresentation, error) {
	path := fmt.Sprintf("/admin/realms/%s/users/%s/role-mappings", realmName, userID)

	resp, err := c.get(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("failed to get user role mappings: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to get user role mappings, status: %d, body: %s", resp.StatusCode, string(body))
	}

	var roleMappings RoleMappingsRepresentation
	if err := json.Unmarshal(body, &roleMappings); err != nil {
		return nil, fmt.Errorf("failed to parse user role mappings: %w", err)
	}

	return &roleMappings, nil
}

// GetUserDetails fetches complete user information including groups and roles
func (c *Client) GetUserDetails(ctx context.Context, realmName, userID string) (*UserDetails, error) {
	// Fetch user info
	user, err := c.GetUserByID(ctx, realmName, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	// Fetch user groups
	groups, err := c.GetUserGroups(ctx, realmName, userID)
	if err != nil {
		c.logger.Warn("Failed to get user groups",
			"realm", realmName,
			"user_id", userID,
			"error", err)
		groups = []GroupRepresentation{}
	}

	// Fetch user role mappings
	roleMappings, err := c.GetUserRoleMappings(ctx, realmName, userID)
	if err != nil {
		c.logger.Warn("Failed to get user role mappings",
			"realm", realmName,
			"user_id", userID,
			"error", err)
		roleMappings = &RoleMappingsRepresentation{}
	}

	return &UserDetails{
		User:         user,
		Groups:       groups,
		RoleMappings: roleMappings,
	}, nil
}
