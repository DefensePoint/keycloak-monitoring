package fx

import (
	"strings"
	"testing"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/config"
)

func TestProvideCookieSessionManager_StopsOnThePublishedSecret(t *testing.T) {
	for _, secret := range []string{"", "change-this-to-a-random-secret-key-at-least-32-chars"} {
		cfg := &config.AppConfig{}
		cfg.Auth.Session.Secret = secret
		cfg.Auth.Session.Name = "kmt"
		cfg.Auth.Session.MaxAge = time.Hour
		mgr, err := provideCookieSessionManager(cfg)
		if err == nil || mgr != nil {
			t.Fatalf("secret %q: manager=%v err=%v, want a startup error", secret, mgr, err)
		}
		if !strings.Contains(err.Error(), "SECURITY ERROR") {
			t.Fatalf("secret %q: err = %v, want a SECURITY ERROR", secret, err)
		}
	}
}

func TestProvideCookieSessionManager_AcceptsARealSecret(t *testing.T) {
	cfg := &config.AppConfig{}
	cfg.Auth.Session.Secret = "0123456789abcdef0123456789abcdef"
	cfg.Auth.Session.Name = "kmt"
	cfg.Auth.Session.MaxAge = time.Hour
	mgr, err := provideCookieSessionManager(cfg)
	if err != nil || mgr == nil {
		t.Fatalf("manager=%v err=%v, want a manager", mgr, err)
	}
}

// The variable binds only to a key config.yaml already carries. When the key is
// missing the secret reads as empty although the operator did set the
// variable; the error must say why it had no effect.
func TestProvideCookieSessionManager_ExplainsAnIgnoredVariable(t *testing.T) {
	t.Setenv("MONITORING_AUTH_SESSION_SECRET", "0123456789abcdef0123456789abcdef")
	cfg := &config.AppConfig{}
	cfg.Auth.Session.Name = "kmt"
	cfg.Auth.Session.MaxAge = time.Hour
	_, err := provideCookieSessionManager(cfg)
	if err == nil || !strings.Contains(err.Error(), "has no effect") {
		t.Fatalf("err = %v, want it to explain the variable had no effect", err)
	}
}

func TestProvideCookieSessionManager_StopsOnAnUnexpandedPlaceholder(t *testing.T) {
	cfg := &config.AppConfig{}
	cfg.Auth.Session.Secret = "${MONITORING_AUTH_SESSION_SECRET}"
	cfg.Auth.Session.Name = "kmt"
	cfg.Auth.Session.MaxAge = time.Hour
	if _, err := provideCookieSessionManager(cfg); err == nil || !strings.Contains(err.Error(), "placeholder") {
		t.Fatalf("err = %v, want a placeholder refusal", err)
	}
}
