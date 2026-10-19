package config

import (
	"strings"
	"testing"
)

// AMFA sslmode key reconciliation
//
// The AMFA block historically spelled the key "sslmode" while the main
// database block spells it "ssl_mode". An operator writing the AMFA block by
// analogy with the main one wrote a key that did not bind, so the field stayed
// empty and the connection silently used "require". Both spellings are now
// accepted, "ssl_mode" is canonical, and a disagreement between them is an
// error rather than a guess.

func amfaCfgWith(sslMode, legacy string) *AmfaTenantConfig {
	return &AmfaTenantConfig{
		Enabled: true,
		Database: AmfaDatabaseConfig{
			Host:          "amfa.internal",
			Database:      "adaptive_mfa",
			User:          "ro",
			SSLMode:       sslMode,
			LegacySSLMode: legacy,
		},
	}
}

func TestAmfaSSLModeKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		sslMode string
		legacy  string
		want    string
		wantErr bool
	}{
		{name: "canonical only", sslMode: "disable", want: "disable"},
		{name: "legacy only", legacy: "disable", want: "disable"},
		{name: "both agree", sslMode: "verify-full", legacy: "verify-full", want: "verify-full"},
		{name: "neither defaults to require", want: "require"},
		{name: "both disagree is an error", sslMode: "disable", legacy: "require", wantErr: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := amfaCfgWith(tt.sslMode, tt.legacy)
			err := cfg.Validate()

			if tt.wantErr {
				if err == nil {
					t.Fatal("want an error when the two spellings disagree, got nil")
				}
				// The operator has two lines in their config and needs to know
				// which to edit, so both keys must be named.
				for _, want := range []string{"ssl_mode", "sslmode", "disable", "require"} {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("error should mention %q, got: %v", want, err)
					}
				}
				return
			}

			if err != nil {
				t.Fatalf("Validate: %v", err)
			}
			if got := cfg.Database.SSLMode; got != tt.want {
				t.Errorf("SSLMode = %q, want %q", got, tt.want)
			}
		})
	}
}

// Validate mutates through the *AmfaTenantConfig pointer and is not guaranteed
// to run exactly once, so reconciliation must reach the same answer on a
// second pass rather than mistaking its own output for a conflict.
func TestAmfaSSLModeKey_ValidateIsIdempotent(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name    string
		sslMode string
		legacy  string
		want    string
	}{
		{name: "legacy only", legacy: "disable", want: "disable"},
		{name: "canonical only", sslMode: "disable", want: "disable"},
		{name: "neither", want: "require"},
	} {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := amfaCfgWith(tt.sslMode, tt.legacy)
			if err := cfg.Validate(); err != nil {
				t.Fatalf("first Validate: %v", err)
			}
			if err := cfg.Validate(); err != nil {
				t.Fatalf("second Validate: %v", err)
			}
			if got := cfg.Database.SSLMode; got != tt.want {
				t.Errorf("SSLMode after two passes = %q, want %q", got, tt.want)
			}
		})
	}
}
