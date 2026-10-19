// Regression coverage for the prepared statements SaveEvents leaves cached.
//
// The events poller used to save every poll's whole batch in one INSERT. With
// PrepareStmt on, each new batch size cached another, larger statement, so a
// realm whose event count kept changing grew the server's heap all day.
//
// Requires a real PostgreSQL instance via KMT_TEST_DATABASE_DSN; skipped when
// unset.
package postgres

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/DefensePoint/keycloak-monitoring/internal/testdb"
	"github.com/DefensePoint/keycloak-monitoring/pkg/database"
)

func TestSaveEventsKeepsCachedStatementsBounded(t *testing.T) {
	dsn := testdb.DSN(t, "KMT_TEST_DATABASE_DSN")
	testdb.Lock(t, dsn)
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger:                 gormlogger.Default.LogMode(gormlogger.Silent),
		SkipDefaultTransaction: true,
		PrepareStmt:            true,
	})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := db.AutoMigrate(&database.KeycloakEvent{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	tenantID := fmt.Sprintf("save-batches-%d", time.Now().UnixNano())
	t.Cleanup(func() {
		db.Where("tenant_id = ?", tenantID).Delete(&database.KeycloakEvent{})
	})

	ctx := context.Background()
	repo := NewEventRepository(db)
	countRows := func() int64 {
		t.Helper()
		var n int64
		if err := db.Model(&database.KeycloakEvent{}).Where("tenant_id = ?", tenantID).Count(&n).Error; err != nil {
			t.Fatalf("count: %v", err)
		}
		return n
	}

	start := time.Now().UTC().Truncate(time.Millisecond)
	day := keycloakEvents(tenantID, "day", 250, start)
	for range 2 {
		if err := repo.SaveEvents(ctx, day); err != nil {
			t.Fatalf("SaveEvents: %v", err)
		}
	}
	if n := countRows(); n != 250 {
		t.Fatalf("stored %d rows after saving the same 250 events twice, want 250", n)
	}

	for size := 1; size <= 150; size++ {
		batch := keycloakEvents(tenantID, fmt.Sprintf("size%03d", size), size, start.Add(time.Hour))
		if err := repo.SaveEvents(ctx, batch); err != nil {
			t.Fatalf("SaveEvents(%d events): %v", size, err)
		}
	}

	cached := 0
	for _, sql := range db.ConnPool.(*gorm.PreparedStmtDB).Stmts.Keys() {
		if strings.HasPrefix(sql, `INSERT INTO "keycloak_events"`) {
			cached++
		}
	}
	if cached > saveEventsBatchSize {
		t.Errorf("%d keycloak_events inserts cached after batches of 1 to 150 events, want at most %d", cached, saveEventsBatchSize)
	}
}
