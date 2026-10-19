package fx

import (
	"testing"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/config"
)

// auth.session.state_max_age is documented as how long an SSO login may take,
// and used to be ignored in favour of a hardcoded 10 minutes.
func TestProvideLoginCookie_UsesStateMaxAge(t *testing.T) {
	for name, tc := range map[string]struct {
		configured, want time.Duration
	}{
		"configured":   {configured: 3 * time.Minute, want: 3 * time.Minute},
		"unset":        {configured: 0, want: defaultLoginMaxAge},
		"not positive": {configured: -time.Second, want: defaultLoginMaxAge},
	} {
		t.Run(name, func(t *testing.T) {
			cfg := &config.AppConfig{}
			cfg.Auth.Session.Secret = "0123456789abcdef0123456789abcdef"
			cfg.Auth.Session.Name = "kmt"
			cfg.Auth.Session.StateMaxAge = tc.configured
			c, err := provideLoginCookie(cfg)
			if err != nil {
				t.Fatal(err)
			}
			if c.MaxAge() != tc.want {
				t.Errorf("MaxAge = %v, want %v", c.MaxAge(), tc.want)
			}
			if c.Name() != "kmt_login" {
				t.Errorf("Name = %q, want the session name with _login", c.Name())
			}
		})
	}
}
