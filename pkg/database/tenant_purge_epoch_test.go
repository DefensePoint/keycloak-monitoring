package database

import (
	"context"
	"testing"
	"time"
)

// A Keycloak event with a zero time lands as telemetry stamped at the Unix
// epoch, decades older than any other row; on TimescaleDB it sits in a chunk of
// its own. The drain must still remove it, and the tenant's queue entry with it.
func TestDrainRemovesTelemetryAtTheEpoch(t *testing.T) {
	c := newPurgeTestClient(t)
	seedTenant(t, c, "epoch")
	if err := c.DB().Exec(
		`INSERT INTO keycloak_metrics (tenant_id, time, realm_name) VALUES ('epoch', ?, 'z')`,
		time.UnixMilli(0)).Error; err != nil {
		t.Fatalf("seed epoch row: %v", err)
	}
	if _, err := c.PurgeTenant(context.Background(), "epoch"); err != nil {
		t.Fatalf("PurgeTenant: %v", err)
	}
	if _, err := c.DrainPendingPurges(context.Background()); err != nil {
		t.Fatalf("drain: %v", err)
	}
	if n := countIn(t, c, "keycloak_metrics", "epoch"); n != 0 {
		t.Errorf("epoch row survived, %d remain", n)
	}
	var queued int64
	c.DB().Model(&PendingTenantPurge{}).Where("tenant_id = ?", "epoch").Count(&queued)
	if queued != 0 {
		t.Errorf("queue row survived")
	}
}
