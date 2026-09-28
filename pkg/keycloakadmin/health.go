package keycloakadmin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"
)

// HealthMetrics represents comprehensive health metrics from Keycloak
type HealthMetrics struct {
	Status        string
	ResponseTime  int64
	ServerVersion string
	UptimeMillis  int64
	MemoryUsed    int64
	MemoryMax     int64
	MemoryFree    int64
	Error         string
}

// GetServerInfo fetches server information from Keycloak
func (c *Client) GetServerInfo(ctx context.Context) (*ServerInfoRepresentation, error) {
	path := "/admin/serverinfo"

	startTime := time.Now()
	resp, err := c.get(ctx, path)
	responseTime := time.Since(startTime).Milliseconds()

	if err != nil {
		return nil, fmt.Errorf("failed to get server info (response time: %dms): %w", responseTime, err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to get server info, status: %d, body: %s", resp.StatusCode, string(body))
	}

	var serverInfo ServerInfoRepresentation
	if err := json.Unmarshal(body, &serverInfo); err != nil {
		return nil, fmt.Errorf("failed to parse server info: %w", err)
	}

	return &serverInfo, nil
}

// CheckHealth performs a health check on the Keycloak server
// Returns status, response time, and any error
func (c *Client) CheckHealth(ctx context.Context) (status string, responseTime int64, err error) {
	startTime := time.Now()

	// Try to get server info as a health check
	serverInfo, err := c.GetServerInfo(ctx)
	responseTime = time.Since(startTime).Milliseconds()

	if err != nil {
		return "DOWN", responseTime, err
	}

	// Check if server info contains valid data
	if serverInfo == nil || serverInfo.SystemInfo == nil {
		return "DEGRADED", responseTime, fmt.Errorf("server info is incomplete")
	}

	return "UP", responseTime, nil
}

// GetHealthMetrics fetches comprehensive health metrics
func (c *Client) GetHealthMetrics(ctx context.Context) *HealthMetrics {
	metrics := &HealthMetrics{
		Status: "DOWN",
	}

	startTime := time.Now()
	serverInfo, err := c.GetServerInfo(ctx)
	metrics.ResponseTime = time.Since(startTime).Milliseconds()

	if err != nil {
		metrics.Error = err.Error()
		return metrics
	}

	if serverInfo == nil {
		metrics.Error = "server info is nil"
		return metrics
	}

	metrics.Status = "UP"

	// Extract system info
	if serverInfo.SystemInfo != nil {
		metrics.ServerVersion = serverInfo.SystemInfo.Version
		metrics.UptimeMillis = serverInfo.SystemInfo.UptimeMillis
	}

	// Extract memory info
	if serverInfo.MemoryInfo != nil {
		metrics.MemoryUsed = serverInfo.MemoryInfo.Used
		metrics.MemoryMax = serverInfo.MemoryInfo.Total
		metrics.MemoryFree = serverInfo.MemoryInfo.Free
	}

	return metrics
}
