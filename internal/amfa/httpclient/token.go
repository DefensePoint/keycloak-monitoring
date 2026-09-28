package httpclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// tokenRefreshSkew is how far before real expiry a cached token is treated as
// stale. A token that expires while in flight would come back as a 401 the
// caller cannot act on, so it is replaced slightly early instead.
const tokenRefreshSkew = 30 * time.Second

// TokenSource hands out bearer tokens for AMFA requests.
type TokenSource interface {
	Token(ctx context.Context) (string, error)
}

// clientCredentialsTokenSource fetches a service-account token from Keycloak
// using the OAuth2 client_credentials grant — the same grant, and the same
// client, KMT already uses for the Keycloak Admin API.
//
// Tokens are cached until shortly before expiry. Without caching, every poll
// cycle and every page of an event pull would mint a fresh token, turning one
// request into two and putting the token endpoint on the hot path.
type clientCredentialsTokenSource struct {
	tokenURL     string
	clientID     string
	clientSecret string
	httpClient   *http.Client

	mu     sync.Mutex
	token  string
	expiry time.Time
}

// NewClientCredentialsTokenSource builds a token source against one realm's
// token endpoint.
func NewClientCredentialsTokenSource(
	serverURL, realm, clientID, clientSecret string, httpClient *http.Client,
) TokenSource {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &clientCredentialsTokenSource{
		tokenURL: fmt.Sprintf(
			"%s/realms/%s/protocol/openid-connect/token",
			strings.TrimRight(serverURL, "/"), realm,
		),
		clientID:     clientID,
		clientSecret: clientSecret,
		httpClient:   httpClient,
	}
}

func (t *clientCredentialsTokenSource) Token(ctx context.Context) (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.token != "" && time.Now().Before(t.expiry) {
		return t.token, nil
	}

	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	form.Set("client_id", t.clientID)
	form.Set("client_secret", t.clientSecret)

	req, err := http.NewRequestWithContext(
		ctx, http.MethodPost, t.tokenURL, strings.NewReader(form.Encode()),
	)
	if err != nil {
		return "", fmt.Errorf("amfa token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return "", classifyHTTPError(err, "amfa token request")
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", classifyHTTPError(err, "amfa token response")
	}
	if err := classifyStatus(resp.StatusCode, body, "amfa token request"); err != nil {
		return "", err
	}

	var parsed struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("amfa token response: %w", err)
	}
	if parsed.AccessToken == "" {
		return "", fmt.Errorf("amfa token response: no access_token in response")
	}

	lifetime := time.Duration(parsed.ExpiresIn) * time.Second
	if lifetime > tokenRefreshSkew {
		lifetime -= tokenRefreshSkew
	}
	// A token with no usable lifetime is still returned, just never cached, so
	// a Keycloak that reports expires_in=0 degrades to per-request fetches
	// rather than failing outright.
	t.token = parsed.AccessToken
	t.expiry = time.Now().Add(lifetime)

	return t.token, nil
}

// StaticTokenSource returns a fixed token. For tests and for deployments that
// inject a token out of band.
func StaticTokenSource(token string) TokenSource {
	return staticTokenSource(token)
}

type staticTokenSource string

func (s staticTokenSource) Token(context.Context) (string, error) {
	if s == "" {
		return "", fmt.Errorf("amfa token: static token is empty")
	}
	return string(s), nil
}
