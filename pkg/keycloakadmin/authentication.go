package keycloakadmin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
)

// GetAuthenticationFlows returns the realm's top-level authentication flows.
func (c *Client) GetAuthenticationFlows(ctx context.Context, realmName string) ([]AuthenticationFlowRepresentation, error) {
	realmSeg, err := pathSegment(realmName)
	if err != nil {
		return nil, err
	}
	path := fmt.Sprintf("/admin/realms/%s/authentication/flows", realmSeg)

	resp, err := c.get(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("failed to get authentication flows: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf(
			"failed to get authentication flows for realm %q, status: %d%s",
			realmName, resp.StatusCode, c.replyDetail("get authentication flows", resp.StatusCode, body),
		)
	}

	var flows []AuthenticationFlowRepresentation
	if err := json.Unmarshal(body, &flows); err != nil {
		return nil, fmt.Errorf("failed to parse authentication flows: %w", err)
	}

	return flows, nil
}

// GetFlowExecutions returns the flattened list of executions for a realm's
// authentication flow, identified by its alias (e.g. "browser"). The list
// includes nested subflow executions; authenticator rows carry a ProviderID.
func (c *Client) GetFlowExecutions(ctx context.Context, realmName, flowAlias string) ([]AuthenticationExecutionInfoRepresentation, error) {
	realmSeg, err := pathSegment(realmName)
	if err != nil {
		return nil, err
	}
	flowSeg, err := pathSegment(flowAlias)
	if err != nil {
		return nil, err
	}
	path := fmt.Sprintf("/admin/realms/%s/authentication/flows/%s/executions", realmSeg, flowSeg)

	resp, err := c.get(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("failed to get flow executions: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf(
			"failed to get flow executions for realm %q flow %q, status: %d%s",
			realmName, flowAlias, resp.StatusCode, c.replyDetail("get flow executions", resp.StatusCode, body),
		)
	}

	var executions []AuthenticationExecutionInfoRepresentation
	if err := json.Unmarshal(body, &executions); err != nil {
		return nil, fmt.Errorf("failed to parse flow executions: %w", err)
	}

	return executions, nil
}
