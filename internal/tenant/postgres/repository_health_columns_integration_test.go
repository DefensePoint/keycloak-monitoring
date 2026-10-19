// Coverage for Update leaving the health and error columns to their writers.
//
// Requires a real PostgreSQL instance, taken from KMT_TEST_DATABASE_DSN the
// same way the enabled tests in this package do.
package postgres

import (
	"context"
	"testing"
	"time"
)

// A rename copies the tenant before a health check may write it. gorm's Save
// writes every column, so saving that copy put the old health back in the
// database, undoing the check until the next one.
func TestUpdateLeavesTheHealthAndErrorColumnsAlone(t *testing.T) {
	repo, db := newEnabledTestRepo(t)
	ctx := context.Background()

	tenant := newTestTenant(t, db, "health-columns", true)
	tenant.HealthStatus = "healthy"
	tenant.HealthMessage = "ok"
	if err := repo.Create(ctx, tenant); err != nil {
		t.Fatalf("Create: %v", err)
	}
	stale := *tenant // the copy a rename takes, before the check below

	if err := repo.UpdateHealth(ctx, tenant.TenantID, "unhealthy", "connection refused"); err != nil {
		t.Fatalf("UpdateHealth: %v", err)
	}
	if err := repo.UpdateError(ctx, tenant.TenantID, "token request failed"); err != nil {
		t.Fatalf("UpdateError: %v", err)
	}

	stale.Name = "Renamed"
	if err := repo.Update(ctx, &stale); err != nil {
		t.Fatalf("Update: %v", err)
	}

	var row struct {
		Name            string
		HealthStatus    string
		HealthMessage   string
		LastError       string
		LastHealthCheck time.Time
	}
	if err := db.Raw(`SELECT name, health_status, health_message, last_error, last_health_check
		FROM keycloak_tenants WHERE tenant_id = ?`, tenant.TenantID).Scan(&row).Error; err != nil {
		t.Fatalf("read back: %v", err)
	}
	if row.Name != "Renamed" {
		t.Errorf("name = %q, want the rename saved", row.Name)
	}
	if row.HealthStatus != "unhealthy" || row.HealthMessage != "connection refused" {
		t.Errorf("health = %q %q, want the check's unhealthy, connection refused", row.HealthStatus, row.HealthMessage)
	}
	if row.LastError != "token request failed" {
		t.Errorf("last_error = %q, want the error write kept", row.LastError)
	}
	if row.LastHealthCheck.IsZero() {
		t.Error("last_health_check was cleared")
	}
}
