package keycloakadmin

import (
	"context"
	"fmt"
	"net/url"
	"slices"
	"strings"
)

// normalizeEventTypes returns the configured event types in the form
// Keycloak reads them: an upper-case name per entry, with entries split on
// commas, trimmed, and empty or repeated ones dropped. Keycloak fails the whole
// events request on a name it does not know, so " LOGIN_ERROR" (a space left
// from a comma-separated setting) or "login" stopped every poll.
func normalizeEventTypes(types []string) []string {
	var out []string
	for _, entry := range types {
		for _, name := range strings.Split(entry, ",") {
			name = strings.ToUpper(strings.TrimSpace(name))
			if name != "" && !slices.Contains(out, name) {
				out = append(out, name)
			}
		}
	}
	return out
}

// queryEventTypes returns the configured event types to send to Keycloak: the
// ones its server info lists, so a misspelt name no longer fails the request
// for the others. Nil means all types. When none of the configured names is
// one Keycloak knows, it returns an error rather than nil, which would ask for
// every type. When the list cannot be read, the configured names are sent.
func (c *Client) queryEventTypes(ctx context.Context) ([]string, error) {
	if len(c.config.EventTypes) == 0 {
		return nil, nil
	}
	c.eventTimesMu.Lock()
	defer c.eventTimesMu.Unlock()
	c.refreshServerEventsLocked(ctx)
	return c.queryTypes, c.queryTypesErr
}

// filterEventTypes splits configured into the names known lists and the
// rest. A nil known keeps them all.
func filterEventTypes(configured []string, known map[string]bool) (keep, unknown []string) {
	if known == nil {
		return configured, nil
	}
	for _, name := range configured {
		if known[name] {
			keep = append(keep, name)
		} else {
			unknown = append(unknown, name)
		}
	}
	return keep, unknown
}

// setQueryEventTypesLocked works out the types to query from the event types
// Keycloak listed (nil if it could not be read) and warns about the
// configured names it does not know. It runs on each server info refresh, so
// the warning repeats at most once per eventTimesCheckEvery.
func (c *Client) setQueryEventTypesLocked(known map[string]bool) {
	keep, unknown := filterEventTypes(c.config.EventTypes, known)
	c.queryTypes, c.queryTypesErr = keep, nil
	if len(unknown) == 0 {
		return
	}
	if len(keep) == 0 {
		c.queryTypesErr = fmt.Errorf("none of the configured event types %v is an event type this Keycloak knows; no events are fetched until keycloak events.types is fixed", unknown)
		c.logger.Error("No configured event type is one Keycloak knows; no events are fetched",
			"keycloak", c.keycloakHost(), "unknown_types", strings.Join(unknown, ","))
		return
	}
	c.logger.Warn("Configured event types Keycloak does not know are left out; the others are fetched",
		"keycloak", c.keycloakHost(), "unknown_types", strings.Join(unknown, ","), "fetched_types", strings.Join(keep, ","))
}

// knownEventTypes reads the event types from server info, nil if it lists none.
func knownEventTypes(info *ServerInfoRepresentation) map[string]bool {
	names := info.Enums["eventType"]
	if len(names) == 0 {
		return nil
	}
	known := make(map[string]bool, len(names))
	for _, name := range names {
		known[name] = true
	}
	return known
}

// keycloakHost is the scheme and host of the Keycloak server, for log lines.
func (c *Client) keycloakHost() string {
	u, err := url.Parse(c.config.ServerURL)
	if err != nil || u.Host == "" {
		return "the Keycloak server"
	}
	return u.Scheme + "://" + u.Host
}
