// Requires a real PostgreSQL instance via KMT_TEST_DATABASE_DSN; skipped when
// unset.
package postgres

import (
	"context"
	"fmt"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/testdb"
	"github.com/DefensePoint/keycloak-monitoring/pkg/database"
)

func TestStoredEventIDs_ReturnsTheTenantsStoredIDsOnly(t *testing.T) {
	dsn := testdb.DSN(t, "KMT_TEST_DATABASE_DSN")
	testdb.Lock(t, dsn)
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Silent)})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := db.AutoMigrate(&database.KeycloakEvent{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	tenant := fmt.Sprintf("stored-ids-%d", time.Now().UnixNano())
	other := tenant + "-other"
	t.Cleanup(func() { db.Where("tenant_id IN ?", []string{tenant, other}).Delete(&database.KeycloakEvent{}) })
	repo := NewEventRepository(db)
	now := time.Now()
	var events []*domain.KeycloakEvent
	for i := 0; i < 2500; i++ { // more than one lookup chunk
		events = append(events, &domain.KeycloakEvent{TenantID: tenant, EventID: fmt.Sprintf("r-%d-LOGIN", i), Time: now, EventType: "LOGIN", RealmName: "r"})
	}
	events = append(events, &domain.KeycloakEvent{TenantID: other, EventID: "only-other", Time: now, EventType: "LOGIN", RealmName: "r"})
	if err := repo.SaveEvents(context.Background(), events); err != nil {
		t.Fatalf("SaveEvents: %v", err)
	}

	ids := []string{"r-0-LOGIN", "r-1999-LOGIN", "r-2499-LOGIN", "r-9999-LOGIN", "only-other"}
	stored, err := repo.StoredEventIDs(context.Background(), tenant, ids, now.Add(-time.Second), now.Add(time.Second))
	if err != nil {
		t.Fatalf("StoredEventIDs: %v", err)
	}
	if len(stored) != 3 || !stored["r-0-LOGIN"] || !stored["r-1999-LOGIN"] || !stored["r-2499-LOGIN"] {
		t.Errorf("stored = %v; want the tenant's three, not the missing one or the other tenant's", stored)
	}
	var many []string
	for i := 0; i < 2500; i++ {
		many = append(many, fmt.Sprintf("r-%d-LOGIN", i))
	}
	if stored, err := repo.StoredEventIDs(context.Background(), tenant, many, now.Add(-time.Second), now.Add(time.Second)); err != nil || len(stored) != 2500 {
		t.Errorf("2500 ids over several chunks: %d found, err %v", len(stored), err)
	}
}

// The newest events stored under ids the poller made, legacy and hashed
// apart: legacy realm-millis-type ids (also with digits in the type, as in
// OAUTH2_DEVICE_CODE_TO_TOKEN) and "kcev-" ids count; Keycloak's UUIDs,
// other tenants and older events do not. StoredEventIDs only finds rows in
// its time range.
func TestIDlessEventTimes(t *testing.T) {
	dsn := testdb.DSN(t, "KMT_TEST_DATABASE_DSN")
	testdb.Lock(t, dsn)
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Silent)})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := db.AutoMigrate(&database.KeycloakEvent{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	tenant := fmt.Sprintf("idless-%d", time.Now().UnixNano())
	other := tenant + "-other"
	t.Cleanup(func() { db.Where("tenant_id IN ?", []string{tenant, other}).Delete(&database.KeycloakEvent{}) })
	repo := NewEventRepository(db)
	base := time.Now().Truncate(time.Second)
	ev := func(tid, id string, at time.Time) *domain.KeycloakEvent {
		return &domain.KeycloakEvent{TenantID: tid, EventID: id, Time: at, EventType: "LOGIN", RealmName: "r"}
	}
	if legacy, hashed, err := repo.IDlessEventTimes(context.Background(), tenant, base.Add(-time.Hour)); err != nil || !legacy.IsZero() || !hashed.IsZero() {
		t.Fatalf("empty tenant: %v, %v, %v; want zero, zero, nil", legacy, hashed, err)
	}
	if err := repo.SaveEvents(context.Background(), []*domain.KeycloakEvent{
		ev(tenant, "0e54b78f-7413-488b-bee2-c7608e253441-1791996699416-LOGIN", base.Add(-40*time.Minute)),
		ev(tenant, "e2e-1791996699500-OAUTH2_DEVICE_CODE_TO_TOKEN", base.Add(-20*time.Minute)), // digits in the type
		ev(tenant, "kcev-13c9a4151ac616e5233fcc525217c1659fdb2ad3", base.Add(-10*time.Minute)),
		ev(tenant, "f7103860-ea8f-46f7-941f-73a1fa632d22", base.Add(-time.Minute)), // Keycloak's own
		ev(tenant, "kcev-older", base.Add(-3*time.Hour)),
		ev(other, "kcev-other-tenant", base),
	}); err != nil {
		t.Fatalf("SaveEvents: %v", err)
	}
	legacy, hashed, err := repo.IDlessEventTimes(context.Background(), tenant, base.Add(-time.Hour))
	if err != nil || !legacy.Equal(base.Add(-20*time.Minute)) || !hashed.Equal(base.Add(-10*time.Minute)) {
		t.Errorf("legacy %v, hashed %v, err %v; want the OAUTH2 legacy row 20 minutes ago and the kcev row 10 minutes ago", legacy, hashed, err)
	}

	in, err := repo.StoredEventIDs(context.Background(), tenant, []string{"kcev-13c9a4151ac616e5233fcc525217c1659fdb2ad3"}, base.Add(-11*time.Minute), base.Add(-9*time.Minute))
	if err != nil || !in["kcev-13c9a4151ac616e5233fcc525217c1659fdb2ad3"] {
		t.Errorf("in range: %v, %v; want found", in, err)
	}
	out, err := repo.StoredEventIDs(context.Background(), tenant, []string{"kcev-13c9a4151ac616e5233fcc525217c1659fdb2ad3"}, base.Add(-5*time.Minute), base)
	if err != nil || len(out) != 0 {
		t.Errorf("out of range: %v, %v; want none", out, err)
	}
}
