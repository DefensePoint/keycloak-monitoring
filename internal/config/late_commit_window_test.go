package config

import (
	"testing"
	"time"
)

// late_commit_window defaults to Keycloak's own transaction timeout, 5m, and
// takes the value set in the config file.
func TestLateCommitWindow_DefaultsToKeycloaksTransactionTimeout(t *testing.T) {
	cfg, err := LoadConfigQuiet(writeConfig(t, `
keycloak:
  global:
    realms: ["acme"]
`))
	if err != nil {
		t.Fatalf("LoadConfigQuiet: %v", err)
	}
	if got := cfg.Keycloak.Global.Events.LateCommitWindow; got != 5*time.Minute {
		t.Errorf("default late_commit_window = %v, want 5m", got)
	}

	cfg, err = LoadConfigQuiet(writeConfig(t, `
keycloak:
  global:
    events:
      late_commit_window: 15m
`))
	if err != nil {
		t.Fatalf("LoadConfigQuiet: %v", err)
	}
	if got := cfg.Keycloak.Global.Events.LateCommitWindow; got != 15*time.Minute {
		t.Errorf("late_commit_window = %v, want 15m", got)
	}
}
