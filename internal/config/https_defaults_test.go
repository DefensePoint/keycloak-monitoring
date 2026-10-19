package config

import "testing"

// Deployments are served over HTTPS, so the session cookie is Secure and HSTS
// is sent unless a configuration says otherwise. Local development over plain
// HTTP turns the cookie setting off, by file or by environment.

func TestHTTPSDefaults(t *testing.T) {
	cfg, err := LoadConfigQuiet(writeConfig(t, ""))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !cfg.Auth.Session.Secure {
		t.Error("auth.session.secure defaults to false")
	}
	if !cfg.HTTP.SecurityHeaders.Enabled || cfg.HTTP.SecurityHeaders.HSTS != "max-age=31536000" {
		t.Errorf("security headers enabled %v, hsts %q; want enabled with max-age=31536000",
			cfg.HTTP.SecurityHeaders.Enabled, cfg.HTTP.SecurityHeaders.HSTS)
	}
}

func TestHTTPSDefaults_LocalDevelopmentTurnsThemOff(t *testing.T) {
	cfg, err := LoadConfigQuiet(writeConfig(t, "auth:\n  session:\n    secure: false\nhttp:\n  security_headers:\n    hsts: \"\"\n"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Auth.Session.Secure || cfg.HTTP.SecurityHeaders.HSTS != "" {
		t.Errorf("file overrides ignored: secure %v, hsts %q", cfg.Auth.Session.Secure, cfg.HTTP.SecurityHeaders.HSTS)
	}

	t.Setenv("MONITORING_AUTH_SESSION_SECURE", "false")
	cfg, err = LoadConfigQuiet(writeConfig(t, ""))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Auth.Session.Secure {
		t.Error("MONITORING_AUTH_SESSION_SECURE=false is ignored")
	}
}
