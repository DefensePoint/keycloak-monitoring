package mcp

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/DefensePoint/keycloak-monitoring/apitoken"
	"github.com/DefensePoint/keycloak-monitoring/internal/config"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

// twoTokensOneUser validates two distinct PATs that both belong to user 7.
func twoTokensOneUser() *mockTokenValidator {
	return &mockTokenValidator{
		validateFn: func(_ context.Context, plaintext string) (*apitoken.Identity, error) {
			user := &domain.User{ID: 7, Subject: "user-7", IsActive: true}
			switch plaintext {
			case "pat_one":
				return &apitoken.Identity{TokenID: 1, User: user, TenantIDs: []string{"tenant-a"}}, nil
			case "pat_two":
				return &apitoken.Identity{TokenID: 2, User: user, TenantIDs: []string{"tenant-a"}}, nil
			}
			return nil, apitoken.ErrTokenNotFound
		},
	}
}

func rateCfg(perUser, perIP, burst int) *config.AppConfig {
	cfg := defaultTestCfg()
	cfg.MCP.Rate = config.MCPRateConfig{
		PerUserPerMinute: perUser,
		PerIPPerMinute:   perIP,
		Burst:            burst,
	}
	return cfg
}

func TestPerUserBudgetSharedAcrossTokens(t *testing.T) {
	ts := newTestServerCustom(t, rateCfg(1, 0, 1), testLogger(), nil,
		twoTokensOneUser(), &mockPermissionService{}, &mockTenantReader{}, &mockRealmReader{},
		&mockAlertReader{}, &mockAmfaStatsReader{}, &mockEventReader{})

	first := connectClient(t, ts, "pat_one")
	res, err := first.CallTool(context.Background(), &mcpsdk.CallToolParams{Name: "whoami"})
	if err != nil {
		t.Fatalf("first whoami call failed: %v", err)
	}
	if res.IsError {
		t.Fatalf("first whoami should pass, got tool error: %+v", res.Content)
	}

	second := connectClient(t, ts, "pat_two")
	res, err = second.CallTool(context.Background(), &mcpsdk.CallToolParams{Name: "whoami"})
	if err != nil {
		t.Fatalf("second whoami call failed: %v", err)
	}
	if text := errTextOf(t, res, "whoami"); text != ErrRateLimited.Error() {
		t.Fatalf("error = %q, want %q", text, ErrRateLimited.Error())
	}
}

func TestPerIPRateLimitBeforeAuth(t *testing.T) {
	ts := newTestServerCustom(t, rateCfg(0, 1, 2), testLogger(), nil,
		acceptingValidator("pat_good"), &mockPermissionService{}, &mockTenantReader{}, &mockRealmReader{},
		&mockAlertReader{}, &mockAmfaStatsReader{}, &mockEventReader{})

	for i := 0; i < 2; i++ {
		resp, err := http.Post(ts.URL+Endpoint, "application/json", nil)
		if err != nil {
			t.Fatalf("request %d: %v", i+1, err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("request %d without token: status %d, want %d", i+1, resp.StatusCode, http.StatusUnauthorized)
		}
	}

	resp, err := http.Post(ts.URL+Endpoint, "application/json", nil)
	if err != nil {
		t.Fatalf("third request: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("third request: status %d, want %d (pre-auth per-IP limit)", resp.StatusCode, http.StatusTooManyRequests)
	}

	for _, path := range []string{"/health", "/ready"} {
		resp, err := http.Get(ts.URL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("GET %s after MCP limit exhausted: status %d, want 200 (probes must not be rate limited)", path, resp.StatusCode)
		}
	}
}

func TestIPBucketKeyShapes(t *testing.T) {
	cases := []struct {
		name       string
		remoteAddr string
		want       string
	}{
		{"ipv4 keys per address", "192.0.2.10:52011", "192.0.2.10"},
		{"ipv4-mapped ipv6 keys as ipv4", "[::ffff:192.0.2.10]:52011", "192.0.2.10"},
		{"ipv6 keys on its /64", "[2001:db8:1:2:aaaa:bbbb:cccc:dddd]:443", "2001:db8:1:2::/64"},
		{"unparseable host keys on the raw string", "not-an-address", "not-an-address"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ipBucketKey(tc.remoteAddr); got != tc.want {
				t.Fatalf("ipBucketKey(%q) = %q, want %q", tc.remoteAddr, got, tc.want)
			}
		})
	}
}

func TestIPBucketKeyIPv6SharesSlash64(t *testing.T) {
	if ipBucketKey("[2001:db8:1:2::1]:1") != ipBucketKey("[2001:db8:1:2:ffff::2]:9") {
		t.Fatal("addresses in the same /64 must share a bucket")
	}
	if ipBucketKey("[2001:db8:1:2::1]:1") == ipBucketKey("[2001:db8:1:3::1]:1") {
		t.Fatal("addresses in different /64s must not share a bucket")
	}
	if ipBucketKey("192.0.2.10:1") == ipBucketKey("192.0.2.11:1") {
		t.Fatal("distinct IPv4 addresses must not share a bucket")
	}
}

func TestKeyLimiterConcurrentAccess(t *testing.T) {
	const (
		perMinute  = 1
		burst      = 5
		goroutines = 8
		iterations = 200
	)
	l := newKeyLimiter[string](perMinute, burst, 1024)

	var admitted atomic.Int64
	start := time.Now()
	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				if l.Allow("hot") {
					admitted.Add(1)
				}
				l.Allow(fmt.Sprintf("key-%d-%d", g, i%64))
			}
		}(g)
	}
	wg.Wait()

	maxAdmitted := int64(burst) + int64(time.Since(start).Minutes()*perMinute) + 1
	if got := admitted.Load(); got < burst || got > maxAdmitted {
		t.Fatalf("hot key admitted %d, want between %d and %d", got, burst, maxAdmitted)
	}
}

func TestKeyLimiterBoundedByLRUEviction(t *testing.T) {
	l := newKeyLimiter[uint](60, 1, 4)

	for i := uint(0); i < 10; i++ {
		l.Allow(i)
	}
	if got := l.buckets.Len(); got != 4 {
		t.Fatalf("tracked keys = %d, want capacity 4", got)
	}

	if !l.Allow(0) {
		t.Fatal("evicted key should restart with a full burst")
	}
	if l.Allow(9) {
		t.Fatal("cached key with a spent burst should be denied")
	}
}

func TestKeyLimiterDisabledWhenRateNonPositive(t *testing.T) {
	if l := newKeyLimiter[string](0, 30, 10); l != nil {
		t.Fatal("perMinute 0 should disable the limiter")
	}
	var l *keyLimiter[string]
	if !l.Allow("anything") {
		t.Fatal("nil limiter must admit everything")
	}
}
