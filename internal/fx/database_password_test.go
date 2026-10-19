package fx

import (
	"strings"
	"testing"

	"go.uber.org/fx/fxtest"

	"github.com/DefensePoint/keycloak-monitoring/internal/config"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
)

// Port 1 refuses connections, so an error that is not the password refusal
// would be a connection attempt the refusal should have prevented.
func unreachableDatabase(password string) config.DatabaseConfig {
	return config.DatabaseConfig{Host: "127.0.0.1", Port: 1, Database: "monitoring", User: "monitoring",
		Password: password, SSLMode: "disable", MaxConns: 2, Timeout: 1}
}

func TestProvideDatabase_RefusesAMissingPassword(t *testing.T) {
	for _, password := range []string{"", "${PMP_UNSET_DB_PASSWORD}"} {
		cfg := &config.AppConfig{Database: unreachableDatabase(password)}
		_, err := provideDatabase(fxtest.NewLifecycle(t), cfg, logger.NewNoop())
		if err == nil || !strings.Contains(err.Error(), "SECURITY ERROR: database.password") {
			t.Errorf("password %q: err = %v, want the database.password refusal", password, err)
		}
	}
}

// The password the local stacks publish is allowed, with a warning, so the
// error here is the connection to port 1 failing, not a refusal.
func TestProvideDatabase_AllowsThePublishedDevelopmentPassword(t *testing.T) {
	cfg := &config.AppConfig{Database: unreachableDatabase(config.ExampleDatabasePassword)}
	_, err := provideDatabase(fxtest.NewLifecycle(t), cfg, logger.NewNoop())
	if err == nil || strings.Contains(err.Error(), "SECURITY ERROR") {
		t.Fatalf("err = %v, want a connection error, not a refusal", err)
	}
}

func TestProvideReadOnlyDatabase_RefusesAMissingPassword(t *testing.T) {
	for _, password := range []string{"", "${PMP_UNSET_DB_PASSWORD}"} {
		db := unreachableDatabase(password)
		cfg := &config.AppConfig{}
		cfg.MCP.Database = &db
		_, err := provideReadOnlyDatabase(fxtest.NewLifecycle(t), cfg, logger.NewNoop())
		if err == nil || !strings.Contains(err.Error(), "SECURITY ERROR: mcp.database.password") {
			t.Errorf("password %q: err = %v, want the mcp.database.password refusal", password, err)
		}
	}
}
