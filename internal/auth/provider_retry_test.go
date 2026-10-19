package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	neturl "net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
)

type fakeIdP struct {
	*httptest.Server
	up          atomic.Bool
	discoveries atomic.Int32
}

func newFakeIdP(t *testing.T) *fakeIdP {
	t.Helper()
	idp := &fakeIdP{}
	idp.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/openid-configuration" {
			http.NotFound(w, r)
			return
		}
		idp.discoveries.Add(1)
		if !idp.up.Load() {
			http.Error(w, "no available server", http.StatusServiceUnavailable)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                                idp.URL,
			"authorization_endpoint":                idp.URL + "/auth",
			"token_endpoint":                        idp.URL + "/token",
			"jwks_uri":                              idp.URL + "/certs",
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	}))
	t.Cleanup(idp.Close)
	return idp
}

func (idp *fakeIdP) config() *Config {
	return &Config{
		ProviderURL:  idp.URL,
		ClientID:     "kmt",
		ClientSecret: "secret",
		RedirectURL:  "https://kmt.invalid/auth/callback",
		Scopes:       []string{"openid", "profile", "email"},
	}
}

func newTestRetryingProvider(t *testing.T, idp *fakeIdP) (*RetryingOIDCProvider, *time.Time) {
	t.Helper()
	p, err := NewRetryingOIDCProvider(idp.config(), logger.NewNoop())
	if err != nil {
		t.Fatal(err)
	}
	clock := time.Date(2026, 10, 9, 14, 7, 6, 0, time.UTC)
	p.now = func() time.Time { return clock }
	return p, &clock
}

func TestRetryingOIDCProvider_RecoversOnceTheIdPIsUp(t *testing.T) {
	idp := newFakeIdP(t)
	p, clock := newTestRetryingProvider(t, idp)
	ctx := context.Background()

	if err := p.Ready(ctx); err == nil {
		t.Fatal("Ready succeeded while the identity provider was down")
	}
	if got := p.GetAuthCodeURL("state"); got != "" {
		t.Fatalf("GetAuthCodeURL = %q while discovery has not succeeded, want empty", got)
	}

	idp.up.Store(true)
	*clock = clock.Add(p.retryAfter)

	if err := p.Ready(ctx); err != nil {
		t.Fatalf("Ready after the identity provider came back: %v", err)
	}
	if got := p.GetAuthCodeURL("state"); !strings.HasPrefix(got, idp.URL+"/auth?") {
		t.Fatalf("GetAuthCodeURL = %q, want the identity provider's authorization endpoint", got)
	}

	before := idp.discoveries.Load()
	*clock = clock.Add(time.Hour)
	if err := p.Ready(ctx); err != nil {
		t.Fatal(err)
	}
	if after := idp.discoveries.Load(); after != before {
		t.Fatalf("discovery ran again after it succeeded: %d requests, want %d", after, before)
	}
}

func TestRetryingOIDCProvider_SpacesOutAttemptsWhileTheIdPIsDown(t *testing.T) {
	idp := newFakeIdP(t)
	p, clock := newTestRetryingProvider(t, idp)
	ctx := context.Background()

	_ = p.Ready(ctx)
	idp.up.Store(true)
	*clock = clock.Add(p.retryAfter - time.Second)

	if err := p.Ready(ctx); err == nil {
		t.Fatal("Ready contacted the identity provider again inside the retry interval")
	}
	if got := idp.discoveries.Load(); got != 1 {
		t.Fatalf("discovery requests = %d inside the retry interval, want 1", got)
	}
}

func TestNewRetryingOIDCProvider_RejectsInvalidConfig(t *testing.T) {
	idp := newFakeIdP(t)
	cfg := idp.config()
	cfg.Scopes = []string{"profile"}

	if _, err := NewRetryingOIDCProvider(cfg, logger.NewNoop()); err == nil {
		t.Fatal("a configuration without the openid scope was accepted")
	}
	if got := idp.discoveries.Load(); got != 0 {
		t.Fatalf("discovery ran %d times for an invalid configuration, want 0", got)
	}
}

func TestServiceLogin_FailsUntilTheProviderIsReady(t *testing.T) {
	idp := newFakeIdP(t)
	p, clock := newTestRetryingProvider(t, idp)
	svc := NewService(p, nil, nil, logger.NewNoop(), idp.config())
	ctx := context.Background()

	if url, _, err := svc.Login(ctx); err == nil {
		t.Fatalf("Login succeeded with url %q while the identity provider was down", url)
	}

	idp.up.Store(true)
	*clock = clock.Add(p.retryAfter)

	url, attempt, err := svc.Login(ctx)
	if err != nil {
		t.Fatalf("Login after the identity provider came back: %v", err)
	}
	if !strings.HasPrefix(url, idp.URL+"/auth?") || !strings.Contains(url, "state="+neturl.QueryEscape(attempt.State)) {
		t.Fatalf("Login url = %q, want the authorization endpoint with the attempt's state", url)
	}
}

type gaugeSpy struct {
	mu     sync.Mutex
	values []bool
}

func (g *gaugeSpy) SetSSOProviderUp(up bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.values = append(g.values, up)
}

func (g *gaugeSpy) last() (bool, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if len(g.values) == 0 {
		return false, false
	}
	return g.values[len(g.values)-1], true
}

func readyConcurrently(p *RetryingOIDCProvider, n int) []error {
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = p.Ready(context.Background())
		}(i)
	}
	wg.Wait()
	return errs
}

// Logins queued behind an attempt that used its whole timeout must share its
// error. Measuring the retry window from the start of the attempt let each of
// them run its own attempt in turn.
func TestRetryingOIDCProvider_QueuedLoginsShareAnAttemptThatTimedOut(t *testing.T) {
	idp := newFakeIdP(t)
	p, clock := newTestRetryingProvider(t, idp)
	var attempts atomic.Int32
	p.connect = func(context.Context) (*OIDCProvider, error) {
		attempts.Add(1)
		*clock = clock.Add(p.timeout)
		return nil, context.DeadlineExceeded
	}

	for i, err := range readyConcurrently(p, 5) {
		if err == nil {
			t.Fatalf("login %d succeeded while the identity provider hung", i)
		}
	}
	if got := attempts.Load(); got != 1 {
		t.Fatalf("discovery attempts = %d for 5 queued logins, want 1", got)
	}
}

func TestRetryingOIDCProvider_HangingIdPFailsQueuedLoginsTogether(t *testing.T) {
	release := make(chan struct{})
	var requests atomic.Int32
	idp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(func() { close(release); idp.Close() })

	p, err := NewRetryingOIDCProvider(&Config{
		ProviderURL:  idp.URL,
		ClientID:     "kmt",
		ClientSecret: "secret",
		RedirectURL:  "https://kmt.invalid/auth/callback",
		Scopes:       []string{"openid"},
	}, logger.NewNoop())
	if err != nil {
		t.Fatal(err)
	}
	p.timeout = 300 * time.Millisecond
	p.retryAfter = 300 * time.Millisecond

	start := time.Now()
	readyConcurrently(p, 5)
	if elapsed := time.Since(start); elapsed > 3*p.timeout {
		t.Fatalf("5 queued logins took %v against a hanging identity provider, want about one timeout (%v)", elapsed, p.timeout)
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("discovery requests = %d, want 1", got)
	}
}

func TestRetryingOIDCProvider_RunRecoversInTheBackground(t *testing.T) {
	idp := newFakeIdP(t)
	var logs syncBuffer
	p, err := NewRetryingOIDCProvider(idp.config(), logger.New(zerolog.New(&logs)))
	if err != nil {
		t.Fatal(err)
	}
	p.backgroundRetry = 20 * time.Millisecond
	p.retryAfter = 0
	gauge := &gaugeSpy{}
	_ = p.Ready(context.Background())
	p.SetMetrics(gauge)
	if up, set := gauge.last(); !set || up {
		t.Fatalf("gauge = %v (set %v) after a failed startup discovery, want 0", up, set)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done := make(chan struct{})
	go func() { defer close(done); p.Run(ctx) }()

	for idp.discoveries.Load() < 3 {
		if ctx.Err() != nil {
			t.Fatal("Run did not retry discovery in the background")
		}
		time.Sleep(5 * time.Millisecond)
	}
	idp.up.Store(true)

	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("Run did not return after the identity provider came back")
	}
	if up, _ := gauge.last(); !up {
		t.Fatal("gauge is not 1 after the background retry connected")
	}
	if !strings.Contains(logs.String(), "SSO is unavailable") {
		t.Fatal("no warning was logged for the failed background retries")
	}
	if got := p.GetAuthCodeURL("state"); !strings.HasPrefix(got, idp.URL+"/auth?") {
		t.Fatalf("GetAuthCodeURL = %q after the background retry, want the authorization endpoint", got)
	}
}

func TestRetryingOIDCProvider_RunStopsWithItsContext(t *testing.T) {
	idp := newFakeIdP(t)
	p, _ := newTestRetryingProvider(t, idp)
	p.backgroundRetry = time.Hour

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); p.Run(ctx) }()
	cancel()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run kept going after its context was cancelled")
	}
}

type syncBuffer struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
