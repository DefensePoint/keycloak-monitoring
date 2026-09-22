package keycloakadmin

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Logger interface for decoupling from concrete logger implementation
type Logger interface {
	Info(msg string, fields ...any)
	Warn(msg string, fields ...any)
	Error(msg string, fields ...any)
	Debug(msg string, fields ...any)
}

// Client represents a Keycloak Admin API client
type Client struct {
	config *ClientConfig
	logger Logger

	httpClient *http.Client
	baseURL    string

	// Token management
	tokenMu     sync.RWMutex
	accessToken string
	tokenExpiry time.Time
}

// NewClient creates a new Keycloak Admin API client. The supplied ctx bounds
// the base-URL auto-detection authentication calls below; callers on a tight
// startup budget (e.g. Fx OnStart) should pass a context with its own
// deadline rather than relying on the HTTP client's Timeout, since a server
// that accepts the TCP connection but never responds is bounded only by
// context cancellation, not by dial-level timeouts.
func NewClient(ctx context.Context, cfg *ClientConfig, logger Logger) (*Client, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is required")
	}

	// Apply defaults for unset values
	cfg.ApplyDefaults()

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	// Use the caller-supplied client when available (e.g. SSRF-aware client
	// from pkg/saferequest); otherwise fall back to the legacy default.
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		transport := &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: cfg.SkipTLSVerify,
			},
		}
		httpClient = &http.Client{
			Timeout:   cfg.Timeout,
			Transport: transport,
		}
	}

	baseURL := strings.TrimRight(cfg.ServerURL, "/")

	client := &Client{
		config:     cfg,
		logger:     logger,
		httpClient: httpClient,
		baseURL:    baseURL,
	}

	// Auto-detect correct base URL (with or without /auth)
	// Keycloak < 17 requires /auth, Keycloak 17+ does not
	if err := client.authenticate(ctx); err != nil {
		// If authentication fails, try with /auth prefix
		if !strings.HasSuffix(baseURL, "/auth") {
			logger.Debug("Authentication failed, retrying with /auth prefix")
			client.baseURL = baseURL + "/auth"
			if err := client.authenticate(ctx); err != nil {
				return nil, fmt.Errorf("failed to authenticate with Keycloak: %w", err)
			}
			logger.Info("Auto-detected Keycloak version < 17 (using /auth prefix)")
		} else {
			// Already has /auth, try without it
			logger.Debug("Authentication failed, retrying without /auth prefix")
			client.baseURL = strings.TrimSuffix(baseURL, "/auth")
			if err := client.authenticate(ctx); err != nil {
				return nil, fmt.Errorf("failed to authenticate with Keycloak: %w", err)
			}
			logger.Info("Auto-detected Keycloak version 17+ (no /auth prefix)")
		}
	} else {
		// First attempt succeeded
		if strings.HasSuffix(baseURL, "/auth") {
			logger.Info("Using Keycloak with /auth prefix (version < 17)")
		} else {
			logger.Info("Using Keycloak without /auth prefix (version 17+)")
		}
	}

	logger.Info("Keycloak client initialized successfully",
		"base_url", client.baseURL,
		"admin_realm", cfg.AdminRealm)

	return client, nil
}

// authenticate obtains an access token from Keycloak
func (c *Client) authenticate(ctx context.Context) error {
	c.tokenMu.Lock()
	defer c.tokenMu.Unlock()

	// Check if we have a valid token
	if c.accessToken != "" && time.Now().Before(c.tokenExpiry) {
		return nil
	}

	c.logger.Debug("Authenticating with Keycloak")

	// Build token endpoint URL
	tokenURL := fmt.Sprintf("%s/realms/%s/protocol/openid-connect/token",
		c.baseURL, c.config.AdminRealm)

	// Prepare form data for the OAuth2 client_credentials grant. The monitoring
	// platform authenticates as a confidential client's service account.
	data := url.Values{}
	data.Set("grant_type", "client_credentials")
	data.Set("client_id", c.config.ClientID)
	data.Set("client_secret", c.config.ClientSecret)

	// Create request
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("failed to create token request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	// Send request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send token request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read token response: %w", err)
	}

	// Check status code
	if resp.StatusCode != http.StatusOK {
		var errResp ErrorResponse
		if err := json.Unmarshal(body, &errResp); err == nil {
			return fmt.Errorf("authentication failed: %s - %s", errResp.Error, errResp.ErrorDescription)
		}
		return fmt.Errorf("authentication failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Parse token response
	var tokenResp TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return fmt.Errorf("failed to parse token response: %w", err)
	}

	// Store token and expiry time (refresh 30 seconds before actual expiry)
	c.accessToken = tokenResp.AccessToken
	c.tokenExpiry = time.Now().Add(time.Duration(tokenResp.ExpiresIn-30) * time.Second)

	c.logger.Debug("Successfully authenticated with Keycloak",
		"expires_in", tokenResp.ExpiresIn)

	return nil
}

// getAccessToken returns a valid access token, refreshing if necessary
func (c *Client) getAccessToken(ctx context.Context) (string, error) {
	c.tokenMu.RLock()
	if c.accessToken != "" && time.Now().Before(c.tokenExpiry) {
		token := c.accessToken
		c.tokenMu.RUnlock()
		return token, nil
	}
	c.tokenMu.RUnlock()

	// Token expired or doesn't exist, re-authenticate
	if err := c.authenticate(ctx); err != nil {
		return "", err
	}

	c.tokenMu.RLock()
	defer c.tokenMu.RUnlock()
	return c.accessToken, nil
}

// doRequest performs an authenticated HTTP request to Keycloak Admin API
func (c *Client) doRequest(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	// Get valid access token
	token, err := c.getAccessToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get access token: %w", err)
	}

	// Build URL
	requestURL := fmt.Sprintf("%s%s", c.baseURL, path)

	// Create request
	req, err := http.NewRequestWithContext(ctx, method, requestURL, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	// Send request with retry logic
	var resp *http.Response
	maxRetries := c.config.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 1
	}

	for attempt := 0; attempt < maxRetries; attempt++ {
		resp, err = c.httpClient.Do(req)
		if err == nil && resp.StatusCode < 500 {
			// Success or client error (4xx) - don't retry
			break
		}

		if err != nil {
			c.logger.Warn("Request failed, retrying",
				"attempt", attempt+1,
				"max_retries", maxRetries,
				"error", err)
		} else {
			c.logger.Warn("Request returned server error, retrying",
				"attempt", attempt+1,
				"max_retries", maxRetries,
				"status_code", resp.StatusCode)
			_ = resp.Body.Close()
		}

		// Wait before retry, but abandon the wait if the caller has gone away.
		// A plain time.Sleep here is invisible to context cancellation, so a
		// shutdown had to sit out the full backoff (5s by default, growing per
		// attempt) even though the request it was retrying had already been
		// abandoned. That was enough to park a monitor's events poller past
		// the 10s drain in Monitor.Stop.
		if attempt < maxRetries-1 {
			backoff := c.config.RetryBackoff
			if backoff <= 0 {
				backoff = 5 * time.Second
			}
			timer := time.NewTimer(backoff * time.Duration(attempt+1))
			select {
			case <-timer.C:
			case <-ctx.Done():
				timer.Stop()
				return nil, ctx.Err()
			}
		}
	}

	if err != nil {
		return nil, fmt.Errorf("request failed after %d attempts: %w", maxRetries, err)
	}

	return resp, nil
}

// get performs a GET request to Keycloak Admin API
func (c *Client) get(ctx context.Context, path string) (*http.Response, error) {
	return c.doRequest(ctx, http.MethodGet, path, nil)
}

// Close closes the HTTP client
func (c *Client) Close() error {
	c.logger.Info("Closing Keycloak client")
	c.httpClient.CloseIdleConnections()
	return nil
}

// GetRealms returns the list of realms to monitor from configuration
func (c *Client) GetRealms() []string {
	if len(c.config.Realms) > 0 {
		return c.config.Realms
	}
	// If no realms specified, return empty slice (caller should fetch all realms)
	return []string{}
}

// IsHealthy checks if the Keycloak client is healthy
func (c *Client) IsHealthy(ctx context.Context) bool {
	if err := c.authenticate(ctx); err != nil {
		c.logger.Error("Keycloak client health check failed", "error", err)
		return false
	}
	return true
}

// Config returns the client configuration (read-only)
func (c *Client) Config() *ClientConfig {
	return c.config
}

// BaseURL returns the base URL being used
func (c *Client) BaseURL() string {
	return c.baseURL
}

// HTTPClient returns the underlying HTTP client for direct access if needed
func (c *Client) HTTPClient() *http.Client {
	return c.httpClient
}

// buildMetricsURL constructs the metrics endpoint URL, using custom port if configured
func (c *Client) buildMetricsURL() string {
	if c.config.InfinispanPort > 0 {
		// Parse the server URL to replace the port
		parsedURL, err := url.Parse(c.config.ServerURL)
		if err == nil {
			hostname := strings.Split(parsedURL.Host, ":")[0]
			parsedURL.Host = fmt.Sprintf("%s:%d", hostname, c.config.InfinispanPort)
			return fmt.Sprintf("%s/metrics", parsedURL.String())
		}
	}
	// Default: use server_url/metrics
	return strings.TrimSuffix(c.config.ServerURL, "/") + "/metrics"
}
