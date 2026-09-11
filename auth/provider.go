package auth

import (
	"context"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// OIDCProvider implements the Provider interface for OAuth2/OIDC authentication.
// This implementation is compatible with Keycloak and other OIDC providers.
type OIDCProvider struct {
	config       *Config
	oauth2Config *oauth2.Config
	verifier     *oidc.IDTokenVerifier
	provider     *oidc.Provider
}

// NewOIDCProvider creates a new OIDC authentication provider.
func NewOIDCProvider(ctx context.Context, cfg *Config) (*OIDCProvider, error) {
	if cfg == nil {
		return nil, &AuthError{
			Code:    ErrCodeProviderError,
			Message: "configuration cannot be nil",
		}
	}

	if err := validateProviderConfig(cfg); err != nil {
		return nil, err
	}

	// Initialize OIDC provider (discovers endpoints from .well-known/openid-configuration)
	provider, err := oidc.NewProvider(ctx, cfg.ProviderURL)
	if err != nil {
		return nil, &AuthError{
			Code:    ErrCodeProviderError,
			Message: "failed to initialize OIDC provider",
			Err:     err,
		}
	}

	// Configure OAuth2
	oauth2Config := &oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  cfg.RedirectURL,
		Endpoint:     provider.Endpoint(),
		Scopes:       cfg.Scopes,
	}

	// Configure ID token verifier
	verifierConfig := &oidc.Config{
		ClientID:        cfg.ClientID,
		SkipIssuerCheck: cfg.SkipIssuerCheck,
		SkipExpiryCheck: cfg.SkipExpiryCheck,
	}
	verifier := provider.Verifier(verifierConfig)

	return &OIDCProvider{
		config:       cfg,
		oauth2Config: oauth2Config,
		verifier:     verifier,
		provider:     provider,
	}, nil
}

// GetAuthCodeURL returns the URL for initiating the OAuth2 authorization code flow.
func (p *OIDCProvider) GetAuthCodeURL(state string) string {
	return p.oauth2Config.AuthCodeURL(state)
}

// Exchange exchanges an authorization code for OAuth2 tokens.
func (p *OIDCProvider) Exchange(ctx context.Context, code string) (*oauth2.Token, error) {
	token, err := p.oauth2Config.Exchange(ctx, code)
	if err != nil {
		return nil, &AuthError{
			Code:    ErrCodeExchangeFailed,
			Message: "failed to exchange authorization code",
			Err:     err,
		}
	}
	return token, nil
}

// VerifyIDToken verifies an ID token and extracts user information.
func (p *OIDCProvider) VerifyIDToken(ctx context.Context, rawIDToken string) (*UserInfo, error) {
	idToken, err := p.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return nil, &AuthError{
			Code:    ErrCodeInvalidToken,
			Message: "failed to verify ID token",
			Err:     err,
		}
	}

	var claims struct {
		Subject           string `json:"sub"`
		Email             string `json:"email"`
		EmailVerified     bool   `json:"email_verified"`
		Name              string `json:"name"`
		GivenName         string `json:"given_name"`
		FamilyName        string `json:"family_name"`
		PreferredUsername string `json:"preferred_username"`
		Locale            string `json:"locale"`
		UpdatedAt         int64  `json:"updated_at"`
	}

	if err := idToken.Claims(&claims); err != nil {
		return nil, &AuthError{
			Code:    ErrCodeInvalidToken,
			Message: "failed to parse ID token claims",
			Err:     err,
		}
	}

	userInfo := &UserInfo{
		Subject:           claims.Subject,
		Email:             claims.Email,
		EmailVerified:     claims.EmailVerified,
		Name:              claims.Name,
		GivenName:         claims.GivenName,
		FamilyName:        claims.FamilyName,
		PreferredUsername: claims.PreferredUsername,
		Locale:            claims.Locale,
	}

	if claims.UpdatedAt > 0 {
		userInfo.UpdatedAt = timeFromUnix(claims.UpdatedAt)
	}

	return userInfo, nil
}

// RefreshToken refreshes an expired access token using a refresh token.
func (p *OIDCProvider) RefreshToken(ctx context.Context, refreshToken string) (*oauth2.Token, error) {
	tokenSource := p.oauth2Config.TokenSource(ctx, &oauth2.Token{
		RefreshToken: refreshToken,
	})

	newToken, err := tokenSource.Token()
	if err != nil {
		return nil, &AuthError{
			Code:    ErrCodeInvalidToken,
			Message: "failed to refresh token",
			Err:     err,
		}
	}

	return newToken, nil
}

// GetUserInfo retrieves additional user information from the userinfo endpoint.
func (p *OIDCProvider) GetUserInfo(ctx context.Context, accessToken string) (*UserInfo, error) {
	userInfo, err := p.provider.UserInfo(ctx, oauth2.StaticTokenSource(&oauth2.Token{
		AccessToken: accessToken,
	}))
	if err != nil {
		return nil, &AuthError{
			Code:    ErrCodeProviderError,
			Message: "failed to get user info",
			Err:     err,
		}
	}

	var claims struct {
		Subject           string `json:"sub"`
		Email             string `json:"email"`
		EmailVerified     bool   `json:"email_verified"`
		Name              string `json:"name"`
		GivenName         string `json:"given_name"`
		FamilyName        string `json:"family_name"`
		PreferredUsername string `json:"preferred_username"`
		Locale            string `json:"locale"`
		UpdatedAt         int64  `json:"updated_at"`
	}

	if err := userInfo.Claims(&claims); err != nil {
		return nil, &AuthError{
			Code:    ErrCodeProviderError,
			Message: "failed to parse userinfo claims",
			Err:     err,
		}
	}

	result := &UserInfo{
		Subject:           claims.Subject,
		Email:             claims.Email,
		EmailVerified:     claims.EmailVerified,
		Name:              claims.Name,
		GivenName:         claims.GivenName,
		FamilyName:        claims.FamilyName,
		PreferredUsername: claims.PreferredUsername,
		Locale:            claims.Locale,
	}

	if claims.UpdatedAt > 0 {
		result.UpdatedAt = timeFromUnix(claims.UpdatedAt)
	}

	return result, nil
}

// validateProviderConfig validates the OAuth2/OIDC configuration.
func validateProviderConfig(cfg *Config) error {
	if cfg.ProviderURL == "" {
		return &AuthError{
			Code:    ErrCodeProviderError,
			Message: "provider URL is required",
		}
	}

	if cfg.ClientID == "" {
		return &AuthError{
			Code:    ErrCodeProviderError,
			Message: "client ID is required",
		}
	}

	if cfg.ClientSecret == "" {
		return &AuthError{
			Code:    ErrCodeProviderError,
			Message: "client secret is required",
		}
	}

	if cfg.RedirectURL == "" {
		return &AuthError{
			Code:    ErrCodeProviderError,
			Message: "redirect URL is required",
		}
	}

	if len(cfg.Scopes) == 0 {
		return &AuthError{
			Code:    ErrCodeProviderError,
			Message: "at least one scope is required",
		}
	}

	// Ensure "openid" scope is present (required for OIDC)
	hasOpenID := false
	for _, scope := range cfg.Scopes {
		if scope == "openid" {
			hasOpenID = true
			break
		}
	}
	if !hasOpenID {
		return &AuthError{
			Code:    ErrCodeProviderError,
			Message: "'openid' scope is required for OIDC authentication",
		}
	}

	return nil
}

// JWTTokenValidator validates JWT access tokens.
type JWTTokenValidator struct {
	provider *OIDCProvider
}

// NewJWTTokenValidator creates a new JWT token validator.
func NewJWTTokenValidator(provider *OIDCProvider) *JWTTokenValidator {
	return &JWTTokenValidator{provider: provider}
}

// ValidateToken validates an access token and returns user info.
func (v *JWTTokenValidator) ValidateToken(ctx context.Context, token string) (*UserInfo, error) {
	return v.provider.VerifyIDToken(ctx, token)
}
