package keycloakadmin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"
)

// EventQueryOptions represents query options for fetching events
type EventQueryOptions struct {
	Types     []string  // Event types to filter (LOGIN, LOGOUT, etc.)
	Client    string    // Client ID to filter
	User      string    // User ID to filter
	IPAddress string    // IP address to filter
	DateFrom  time.Time // Start date for events
	DateTo    time.Time // End date for events
	First     int       // Pagination: first result index
	Max       int       // Pagination: maximum number of results
}

// AdminEventQueryOptions represents query options for fetching admin events
type AdminEventQueryOptions struct {
	OperationTypes []string  // Operation types to filter (CREATE, UPDATE, DELETE, etc.)
	ResourcePath   string    // Resource path to filter
	DateFrom       time.Time // Start date for events
	DateTo         time.Time // End date for events
	First          int       // Pagination: first result index
	Max            int       // Pagination: maximum number of results
}

// formatKeycloakTime formats a time.Time to Keycloak's expected format (yyyy-MM-dd)
func formatKeycloakTime(t time.Time) string {
	return t.Format("2006-01-02")
}

// GetEvents fetches events from a realm
func (c *Client) GetEvents(ctx context.Context, realmName string, options *EventQueryOptions) ([]*EventRepresentation, error) {
	path := fmt.Sprintf("/admin/realms/%s/events", realmName)

	// Build query parameters
	if options != nil {
		params := url.Values{}

		if len(options.Types) > 0 {
			params.Add("type", strings.Join(options.Types, ","))
		}

		if options.Client != "" {
			params.Add("client", options.Client)
		}

		if options.User != "" {
			params.Add("user", options.User)
		}

		if options.IPAddress != "" {
			params.Add("ipAddress", options.IPAddress)
		}

		if !options.DateFrom.IsZero() {
			params.Add("dateFrom", formatKeycloakTime(options.DateFrom))
		}

		if !options.DateTo.IsZero() {
			params.Add("dateTo", formatKeycloakTime(options.DateTo))
		}

		if options.First > 0 {
			params.Add("first", fmt.Sprintf("%d", options.First))
		}

		if options.Max > 0 {
			params.Add("max", fmt.Sprintf("%d", options.Max))
		}

		if len(params) > 0 {
			path = path + "?" + params.Encode()
		}
	}

	resp, err := c.get(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("failed to get events: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to get events, status: %d, body: %s", resp.StatusCode, string(body))
	}

	var events []*EventRepresentation
	if err := json.Unmarshal(body, &events); err != nil {
		return nil, fmt.Errorf("failed to parse events: %w", err)
	}

	return events, nil
}

// GetAdminEvents fetches admin events from a realm
func (c *Client) GetAdminEvents(ctx context.Context, realmName string, options *AdminEventQueryOptions) ([]*AdminEventRepresentation, error) {
	path := fmt.Sprintf("/admin/realms/%s/admin-events", realmName)

	// Build query parameters
	if options != nil {
		params := url.Values{}

		if len(options.OperationTypes) > 0 {
			for _, opType := range options.OperationTypes {
				params.Add("operationTypes", opType)
			}
		}

		if options.ResourcePath != "" {
			params.Add("resourcePath", options.ResourcePath)
		}

		if !options.DateFrom.IsZero() {
			params.Add("dateFrom", formatKeycloakTime(options.DateFrom))
		}

		if !options.DateTo.IsZero() {
			params.Add("dateTo", formatKeycloakTime(options.DateTo))
		}

		if options.First > 0 {
			params.Add("first", fmt.Sprintf("%d", options.First))
		}

		if options.Max > 0 {
			params.Add("max", fmt.Sprintf("%d", options.Max))
		}

		if len(params) > 0 {
			path = path + "?" + params.Encode()
		}
	}

	resp, err := c.get(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("failed to get admin events: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to get admin events, status: %d, body: %s", resp.StatusCode, string(body))
	}

	var events []*AdminEventRepresentation
	if err := json.Unmarshal(body, &events); err != nil {
		return nil, fmt.Errorf("failed to parse admin events: %w", err)
	}

	return events, nil
}

// GetRecentEvents is a helper method to get recent events using configuration
func (c *Client) GetRecentEvents(ctx context.Context, realmName string) ([]*EventRepresentation, error) {
	options := &EventQueryOptions{
		Types:    c.config.EventTypes,
		DateFrom: time.Now().Add(-c.config.EventLookbackDuration),
		DateTo:   time.Now(),
		Max:      c.config.MaxEventsPerPoll,
	}

	return c.GetEvents(ctx, realmName, options)
}
