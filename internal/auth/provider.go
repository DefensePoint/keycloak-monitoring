package auth

import (
	"context"
	"sync"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
)

// OIDCProvider implements the Provider interface for OAuth2/OIDC authentication.
// This implementation is compatible with Keycloak and other OIDC providers.
type OIDCProvider struct {
	config       *Config
	oauth2Config *oauth2.Config
	verifier     *oidc.IDTokenVerifier
	provider     *oidc.Provider
	log          *logger.Logger

	// warnNoEmailVerified fires the claim-missing warning once per process
	// rather than once per login. See warnIfEmailVerifiedAbsent.
	warnNoEmailVerified sync.Once
}

// warnIfEmailVerifiedAbsent reports an IdP that does not send email_verified.
//
// Go decodes an absent bool claim as false, which is indistinguishable from an
// IdP saying the address is unconfirmed. Without this, a realm whose client is
// missing the email scope or its mapper would mark every user unverified and
// give no clue why: no error, no failed login, just a column that is quietly
// wrong and a badge nobody trusts.
//
// Keycloak sends the claim under the default scopes, so this should never fire.
// If it does, the realm's client configuration is the thing to look at.
//
// Once per process: this runs on every login, and a misconfiguration that
// repeats per request drowns the log rather than informing it.
func (p *OIDCProvider) warnIfEmailVerifiedAbsent(claims map[string]interface{}, source string) {
	if p.log == nil {
		return
	}
	if _, present := claims["email_verified"]; present {
		return
	}
	p.warnNoEmailVerified.Do(func() {
		p.log.Warn("Identity provider does not send the email_verified claim; every user will "+
			"be recorded as unverified. Check that the client has the email scope and its "+
			"mapper enabled",
			logger.Str("source", source),
			logger.Str("provider_url", p.config.ProviderURL),
			logger.Str("client_id", p.config.ClientID))
	})
}

// NewOIDCProvider creates a new OIDC authentication provider.
func NewOIDCProvider(ctx context.Context, cfg *Config, log *logger.Logger) (*OIDCProvider, error) {
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
		log:          log,
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

	// Decoded a second time as a map purely to tell "the IdP said false" from
	// "the IdP said nothing", which the struct above cannot express.
	var allClaims map[string]interface{}
	if err := idToken.Claims(&allClaims); err == nil {
		p.warnIfEmailVerifiedAbsent(allClaims, "id_token")
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

	var allClaims map[string]interface{}
	if err := userInfo.Claims(&allClaims); err == nil {
		p.warnIfEmailVerifiedAbsent(allClaims, "userinfo")
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
