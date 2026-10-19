package auth

import (
	"context"
	"sync"
	"time"

	"golang.org/x/oauth2"

	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
)

const (
	discoveryTimeout         = 10 * time.Second
	discoveryRetryAfter      = 10 * time.Second
	discoveryBackgroundRetry = 30 * time.Second
)

// SSOMetrics records whether the identity provider is connected.
type SSOMetrics interface {
	SetSSOProviderUp(up bool)
}

// RetryingOIDCProvider connects to the identity provider on first use and
// retries until discovery succeeds, so an identity provider that is down when
// the server starts does not leave SSO broken until the next restart.
type RetryingOIDCProvider struct {
	connect         func(ctx context.Context) (*OIDCProvider, error)
	providerURL     string
	timeout         time.Duration
	retryAfter      time.Duration
	backgroundRetry time.Duration
	now             func() time.Time
	log             *logger.Logger

	mu          sync.Mutex
	inner       *OIDCProvider
	lastAttempt time.Time
	lastErr     error
	metrics     SSOMetrics
}

// NewRetryingOIDCProvider validates cfg and returns a provider that runs OIDC
// discovery on demand. An invalid configuration is returned as an error and is
// never retried.
func NewRetryingOIDCProvider(cfg *Config, log *logger.Logger) (*RetryingOIDCProvider, error) {
	if cfg == nil {
		return nil, &AuthError{
			Code:    ErrCodeProviderError,
			Message: "configuration cannot be nil",
		}
	}
	if err := validateProviderConfig(cfg); err != nil {
		return nil, err
	}
	return &RetryingOIDCProvider{
		connect: func(ctx context.Context) (*OIDCProvider, error) {
			return NewOIDCProvider(ctx, cfg, log)
		},
		providerURL:     cfg.ProviderURL,
		timeout:         discoveryTimeout,
		retryAfter:      discoveryRetryAfter,
		backgroundRetry: discoveryBackgroundRetry,
		now:             time.Now,
		log:             log,
	}, nil
}

// SetMetrics attaches the gauge and publishes the current state.
func (p *RetryingOIDCProvider) SetMetrics(m SSOMetrics) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.metrics = m
	p.publishLocked()
}

func (p *RetryingOIDCProvider) publishLocked() {
	if p.metrics != nil {
		p.metrics.SetSSOProviderUp(p.inner != nil)
	}
}

// Run retries discovery every backgroundRetry until it succeeds or ctx ends,
// so SSO recovers before the first login rather than on it.
func (p *RetryingOIDCProvider) Run(ctx context.Context) {
	ticker := time.NewTicker(p.backgroundRetry)
	defer ticker.Stop()
	for {
		p.mu.Lock()
		connected := p.inner != nil
		p.mu.Unlock()
		if connected {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		if _, err := p.get(ctx); err != nil && ctx.Err() == nil && p.log != nil {
			p.log.Warn("SSO is unavailable: the OIDC provider is still unreachable",
				logger.Err(err),
				logger.Str("provider_url", p.providerURL),
				logger.Str("next_retry_in", p.backgroundRetry.String()))
		}
	}
}

// Ready runs discovery if it has not succeeded yet. Attempts are spaced at
// least retryAfter apart; in between, the last error is returned without
// contacting the identity provider.
func (p *RetryingOIDCProvider) Ready(ctx context.Context) error {
	_, err := p.get(ctx)
	return err
}

func (p *RetryingOIDCProvider) get(ctx context.Context) (*OIDCProvider, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.inner != nil {
		return p.inner, nil
	}
	if !p.lastAttempt.IsZero() && p.now().Sub(p.lastAttempt) < p.retryAfter {
		return nil, p.lastErr
	}

	attemptCtx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	inner, err := p.connect(attemptCtx)
	// Measured from the end of the attempt: callers queued behind an attempt
	// that hung for the full timeout must get its error, not start another.
	p.lastAttempt = p.now()
	if err != nil {
		p.lastErr = err
		p.publishLocked()
		return nil, err
	}

	p.inner = inner
	p.lastErr = nil
	p.publishLocked()
	if p.log != nil {
		p.log.Info("OAuth2/OIDC provider initialized successfully")
	}
	return inner, nil
}

// GetAuthCodeURL returns "" while discovery has not succeeded. Callers check
// Ready first.
func (p *RetryingOIDCProvider) GetAuthCodeURL(state string, opts ...oauth2.AuthCodeOption) string {
	inner, err := p.get(context.Background())
	if err != nil {
		return ""
	}
	return inner.GetAuthCodeURL(state, opts...)
}

// Exchange exchanges an authorization code for OAuth2 tokens.
func (p *RetryingOIDCProvider) Exchange(ctx context.Context, code string, opts ...oauth2.AuthCodeOption) (*oauth2.Token, error) {
	inner, err := p.get(ctx)
	if err != nil {
		return nil, err
	}
	return inner.Exchange(ctx, code, opts...)
}

// VerifyIDToken verifies an ID token and extracts user information.
func (p *RetryingOIDCProvider) VerifyIDToken(ctx context.Context, rawIDToken string) (*UserInfo, error) {
	inner, err := p.get(ctx)
	if err != nil {
		return nil, err
	}
	return inner.VerifyIDToken(ctx, rawIDToken)
}

// VerifyLoginIDToken verifies a login's ID token and its nonce.
func (p *RetryingOIDCProvider) VerifyLoginIDToken(ctx context.Context, rawIDToken, nonce string) (*UserInfo, error) {
	inner, err := p.get(ctx)
	if err != nil {
		return nil, err
	}
	return inner.VerifyLoginIDToken(ctx, rawIDToken, nonce)
}

// RefreshToken refreshes an expired access token using a refresh token.
func (p *RetryingOIDCProvider) RefreshToken(ctx context.Context, refreshToken string) (*oauth2.Token, error) {
	inner, err := p.get(ctx)
	if err != nil {
		return nil, err
	}
	return inner.RefreshToken(ctx, refreshToken)
}
