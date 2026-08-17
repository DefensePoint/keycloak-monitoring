package keycloakadmin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// GetUsersCount returns the total number of users in a realm
func (c *Client) GetUsersCount(ctx context.Context, realmName string) (int, error) {
	return c.getUsersCount(ctx, realmName, "")
}

// getUsersCount fetches a user count from the Keycloak Admin API /users/count
// endpoint. The optional query string (e.g. "?enabled=true") is appended to
// scope the count. Keycloak resolves this as a DB COUNT query, so no user
// objects are serialized or transferred.
func (c *Client) getUsersCount(ctx context.Context, realmName, query string) (int, error) {
	path := fmt.Sprintf("/admin/realms/%s/users/count%s", realmName, query)

	resp, err := c.get(ctx, path)
	if err != nil {
		return 0, fmt.Errorf("failed to get users count: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != 200 {
		return 0, fmt.Errorf("failed to get users count, status: %d, body: %s", resp.StatusCode, string(body))
	}

	var count int
	if err := json.Unmarshal(body, &count); err != nil {
		return 0, fmt.Errorf("failed to parse users count: %w", err)
	}

	return count, nil
}

// GetEnabledUsersCount returns the number of enabled vs disabled users in a realm.
//
// It issues two COUNT-only queries against the Keycloak Admin API.
// enabled is derived as (total - disabled) instead of being read from
// /users/count?enabled=true. The reason is that Keycloak's unfiltered
// /users/count excludes service-account users (matching what the admin console
// shows), while /users/count?enabled=true includes them.
func (c *Client) GetEnabledUsersCount(ctx context.Context, realmName string) (enabled int, disabled int, err error) {
	total, err := c.getUsersCount(ctx, realmName, "")
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get total users count: %w", err)
	}

	disabled, err = c.getUsersCount(ctx, realmName, "?enabled=false")
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get disabled users count: %w", err)
	}

	enabled = total - disabled
	if enabled < 0 {
		enabled = 0
	}

	return enabled, disabled, nil
}

// flexInt unmarshals an integer that may arrive as a JSON number (1) or a
// quoted JSON string ("1"). Keycloak's client-session-stats currently returns
// the counts as strings; this tolerates a future change to numbers without
// zeroing the metric. An empty or null value decodes to 0.
type flexInt int

func (f *flexInt) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" || s == "null" {
		*f = 0
		return nil
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return err
	}
	*f = flexInt(n)
	return nil
}

// clientSessionStat is one entry from Keycloak's client-session-stats endpoint.
// active/offline are accepted as either strings or numbers (see flexInt).
type clientSessionStat struct {
	ClientID string  `json:"clientId"`
	Active   flexInt `json:"active"`
	Offline  flexInt `json:"offline"`
}

// GetSessionsCount returns the total active and offline sessions across all
// clients in a realm.
//
// It uses Keycloak's realm-level client-session-stats endpoint — a single call
// that returns per-client active/offline counts. This is preferred over the
// per-client session-count endpoints because:
//   - it reflects the live session store, avoiding the stale over-counts the
//     per-client session-count endpoint can return for expired-but-not-evicted
//     sessions;
//   - it is one request instead of two per client; and
//   - it returns counts only, so Keycloak never resolves each session's user
//     (no per-session storage-provider lookups on federated realms).
//
// Only clients with sessions are included in the response; the sum over the
// array is the realm total.
func (c *Client) GetSessionsCount(ctx context.Context, realmName string) (activeSessions int, offlineSessions int, err error) {
	path := fmt.Sprintf("/admin/realms/%s/client-session-stats", realmName)

	resp, err := c.get(ctx, path)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get client session stats: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != 200 {
		return 0, 0, fmt.Errorf("failed to get client session stats, status: %d, body: %s", resp.StatusCode, string(body))
	}

	var stats []clientSessionStat
	if err := json.Unmarshal(body, &stats); err != nil {
		return 0, 0, fmt.Errorf("failed to parse client session stats: %w", err)
	}

	for _, s := range stats {
		activeSessions += int(s.Active)
		offlineSessions += int(s.Offline)
	}

	return activeSessions, offlineSessions, nil
}
