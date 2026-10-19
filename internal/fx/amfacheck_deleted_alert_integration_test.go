package fx

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"

	alertspg "github.com/DefensePoint/keycloak-monitoring/internal/alerts/postgres"
	"github.com/DefensePoint/keycloak-monitoring/internal/amfa"
	"github.com/DefensePoint/keycloak-monitoring/internal/amfacheck"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/internal/testdb"
	"github.com/DefensePoint/keycloak-monitoring/pkg/database"
)

// A per-event AMFA check rereads recent events, so an alert an operator
// deleted must stay deleted on every later poll. Run against both shapes of
// idx_alert_tenant: the partial one (WHERE deleted_at IS NULL) that
// migrateAlertIDIndex leaves on every database, and the full unique index
// GORM creates, which stays only when that migration fails.

type oneRejectedEvent struct {
	acRepoStub
	event amfa.EventRow
}

func (r oneRejectedEvent) ListRejectedEventsSince(_ context.Context, _ string, since time.Time) ([]amfa.EventRow, error) {
	if r.event.EventTime.After(since) {
		return []amfa.EventRow{r.event}, nil
	}
	return nil, nil
}

type countingAlertNotifier struct {
	mu   sync.Mutex
	sent int
}

func (n *countingAlertNotifier) NotifyAlert(context.Context, *domain.Alert) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.sent++
	return nil
}
func (n *countingAlertNotifier) NotifyAlertResolution(context.Context, *domain.Alert) error {
	return nil
}

// useAlertIndex rebuilds idx_alert_tenant with the given definition for the
// test, and restores the original afterwards.
func useAlertIndex(t *testing.T, db *gorm.DB, createSQL string) {
	t.Helper()
	var original string
	if err := db.Raw(`SELECT indexdef FROM pg_indexes WHERE indexname = 'idx_alert_tenant'`).Scan(&original).Error; err != nil || original == "" {
		t.Fatalf("read idx_alert_tenant: %q, %v", original, err)
	}
	for _, stmt := range []string{`DROP INDEX idx_alert_tenant`, createSQL} {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	t.Cleanup(func() {
		db.Exec(`DROP INDEX IF EXISTS idx_alert_tenant`)
		if err := db.Exec(original).Error; err != nil {
			t.Errorf("restore idx_alert_tenant: %v", err)
		}
	})
}

func TestAmfaCheck_DeletedAlertStaysDeleted(t *testing.T) {
	for _, tc := range []struct {
		name  string
		index string
	}{
		{name: "full index (migration failed)",
			index: `CREATE UNIQUE INDEX idx_alert_tenant ON configuration_alerts (tenant_id, alert_id)`},
		{name: "partial index",
			index: `CREATE UNIQUE INDEX idx_alert_tenant ON configuration_alerts (tenant_id, alert_id) WHERE deleted_at IS NULL`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dsn := testdb.DSN(t, "KMT_TEST_DATABASE_DSN")
			testdb.Lock(t, dsn)
			client, err := database.NewClient(testdb.DatabaseConfig(t, dsn), logger.NewNoop())
			if err != nil {
				t.Fatalf("NewClient: %v", err)
			}
			t.Cleanup(func() { _ = client.Close() })
			db := client.DB()
			useAlertIndex(t, db, tc.index)

			tenant := fmt.Sprintf("amfa-deleted-%d", time.Now().UnixNano())
			t.Cleanup(func() { db.Exec(`DELETE FROM configuration_alerts WHERE tenant_id = ?`, tenant) })

			repo := alertspg.NewRepository(db)
			events := oneRejectedEvent{event: amfa.EventRow{EventID: "ev-1", EventTime: time.Now().Add(-10 * time.Second)}}
			check := amfacheck.NewRiskRejectedCheck(tenant, func(context.Context) ([]string, error) { return []string{"r1"}, nil },
				events, time.Minute, noopAmfacheckLogger{})
			notifier := &countingAlertNotifier{}
			svc := amfacheck.NewService(tenant, []amfacheck.Check{check}, amfaCheckAlertStore{repo: repo}, notifier,
				noopAmfacheckLogger{}, time.Minute)
			ctx := context.Background()

			if err := svc.RunCheckNow(ctx); err != nil {
				t.Fatalf("first run: %v", err)
			}
			alerts, err := repo.List(ctx, tenant, nil)
			if err != nil || len(alerts) != 1 || notifier.sent != 1 {
				t.Fatalf("first run: %d alerts, %d notifications, err %v; want 1 and 1", len(alerts), notifier.sent, err)
			}

			if err := repo.Delete(ctx, tenant, alerts[0].AlertID); err != nil {
				t.Fatalf("delete: %v", err)
			}
			for range 2 {
				if err := svc.RunCheckNow(ctx); err != nil {
					t.Fatalf("rerun: %v", err)
				}
			}

			live, err := repo.List(ctx, tenant, nil)
			if err != nil {
				t.Fatalf("list: %v", err)
			}
			if len(live) != 0 || notifier.sent != 1 {
				t.Fatalf("after delete: %d live alerts, %d notifications; want 0 and 1", len(live), notifier.sent)
			}
			var rows int64
			db.Raw(`SELECT count(*) FROM configuration_alerts WHERE tenant_id = ?`, tenant).Scan(&rows)
			if rows != 1 {
				t.Fatalf("rows for the tenant, deleted included = %d, want 1", rows)
			}
		})
	}
}

// Startup narrows idx_alert_tenant to live rows on every database, so the
// lookup that counts deleted alerts always gets its own index: on an upgraded
// database whose index is already partial, and on a fresh one whose full
// index GORM created at first start.
func TestAlertLookupIndexFollowsTheAlertIndexShape(t *testing.T) {
	for _, tc := range []struct {
		name  string
		index string
	}{
		{name: "full index (fresh database)",
			index: `CREATE UNIQUE INDEX idx_alert_tenant ON configuration_alerts (tenant_id, alert_id)`},
		{name: "partial index (upgraded database)",
			index: `CREATE UNIQUE INDEX idx_alert_tenant ON configuration_alerts (tenant_id, alert_id) WHERE deleted_at IS NULL`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dsn := testdb.DSN(t, "KMT_TEST_DATABASE_DSN")
			testdb.Lock(t, dsn)
			setup, err := database.NewClient(testdb.DatabaseConfig(t, dsn), logger.NewNoop())
			if err != nil {
				t.Fatalf("NewClient: %v", err)
			}
			t.Cleanup(func() { _ = setup.Close() })
			db := setup.DB()
			db.Exec(`DROP INDEX IF EXISTS idx_alerts_tenant_alert_id`)
			t.Cleanup(func() { db.Exec(`DROP INDEX IF EXISTS idx_alerts_tenant_alert_id`) })
			useAlertIndex(t, db, tc.index)

			migrated, err := database.NewClient(testdb.DatabaseConfig(t, dsn), logger.NewNoop())
			if err != nil {
				t.Fatalf("NewClient after switching the index: %v", err)
			}
			_ = migrated.Close()

			var alertIndex string
			db.Raw(`SELECT indexdef FROM pg_indexes WHERE indexname = 'idx_alert_tenant'`).Scan(&alertIndex)
			if !strings.Contains(alertIndex, "deleted_at IS NULL") {
				t.Fatalf("idx_alert_tenant after startup = %q, want it scoped to live rows", alertIndex)
			}
			var valid []bool
			db.Raw(`SELECT i.indisvalid FROM pg_index i JOIN pg_class ic ON ic.oid = i.indexrelid
				WHERE ic.relname = 'idx_alerts_tenant_alert_id'`).Scan(&valid)
			if len(valid) != 1 || !valid[0] {
				t.Fatalf("idx_alerts_tenant_alert_id valid = %v, want one valid index", valid)
			}

			var plan []string
			err = db.Transaction(func(tx *gorm.DB) error {
				tx.Exec(`SET LOCAL enable_seqscan = off`)
				return tx.Raw(`EXPLAIN SELECT EXISTS (SELECT 1 FROM configuration_alerts WHERE tenant_id = 't' AND alert_id = 'a')`).Scan(&plan).Error
			})
			if err != nil {
				t.Fatalf("explain: %v", err)
			}
			joined := fmt.Sprint(plan)
			if !strings.Contains(joined, "idx_alert_tenant") && !strings.Contains(joined, "idx_alerts_tenant_alert_id") {
				t.Fatalf("the lookup uses no (tenant_id, alert_id) index:\n%s", joined)
			}
		})
	}
}

// Two instances starting together on an upgraded database must both finish:
// the one building the lookup index under the schema lock must not wait on the
// other, which is waiting for that lock.
func TestAlertLookupIndexTwoInstancesStartTogether(t *testing.T) {
	dsn := testdb.DSN(t, "KMT_TEST_DATABASE_DSN")
	testdb.Lock(t, dsn)
	setup, err := database.NewClient(testdb.DatabaseConfig(t, dsn), logger.NewNoop())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = setup.Close() })
	db := setup.DB()
	db.Exec(`DROP INDEX IF EXISTS idx_alerts_tenant_alert_id`)
	t.Cleanup(func() { db.Exec(`DROP INDEX IF EXISTS idx_alerts_tenant_alert_id`) })
	useAlertIndex(t, db, `CREATE UNIQUE INDEX idx_alert_tenant ON configuration_alerts (tenant_id, alert_id) WHERE deleted_at IS NULL`)

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	gate, err := sqlDB.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = gate.Close() }()
	if _, err := gate.ExecContext(ctx, `SELECT pg_advisory_lock(hashtext('kmt.schema')::bigint)`); err != nil {
		t.Fatalf("hold the schema lock: %v", err)
	}

	done := make(chan error, 2)
	for range 2 {
		go func() {
			c, err := database.NewClient(testdb.DatabaseConfig(t, dsn), logger.NewNoop())
			if err == nil {
				_ = c.Close()
			}
			done <- err
		}()
	}
	time.Sleep(time.Second)
	if _, err := gate.ExecContext(ctx, `SELECT pg_advisory_unlock(hashtext('kmt.schema')::bigint)`); err != nil {
		t.Fatalf("release the schema lock: %v", err)
	}

	timeout := time.After(60 * time.Second)
	for range 2 {
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("an instance failed to start: %v", err)
			}
		case <-timeout:
			db.Exec(`SELECT pg_terminate_backend(pid) FROM pg_stat_activity
				WHERE datname = current_database() AND pid <> pg_backend_pid()
				AND (query ILIKE '%pg_advisory_lock%' OR query ILIKE '%CREATE INDEX CONCURRENTLY%')`)
			t.Fatal("the two instances did not finish starting within 60s")
		}
	}

	var valid []bool
	db.Raw(`SELECT i.indisvalid FROM pg_index i JOIN pg_class ic ON ic.oid = i.indexrelid
		WHERE ic.relname = 'idx_alerts_tenant_alert_id'`).Scan(&valid)
	if len(valid) != 1 || !valid[0] {
		t.Fatalf("idx_alerts_tenant_alert_id valid = %v, want one valid index", valid)
	}
}

// The lookup index only makes a query faster, so a failed build must not stop
// startup; the next start tries again.
func TestAlertLookupIndexBuildFailureDoesNotStopStartup(t *testing.T) {
	dsn := testdb.DSN(t, "KMT_TEST_DATABASE_DSN")
	testdb.Lock(t, dsn)
	setup, err := database.NewClient(testdb.DatabaseConfig(t, dsn), logger.NewNoop())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = setup.Close() })
	db := setup.DB()
	db.Exec(`DROP INDEX IF EXISTS idx_alerts_tenant_alert_id`)
	useAlertIndex(t, db, `CREATE UNIQUE INDEX idx_alert_tenant ON configuration_alerts (tenant_id, alert_id) WHERE deleted_at IS NULL`)
	if err := db.Exec(`CREATE TABLE idx_alerts_tenant_alert_id (id int)`).Error; err != nil {
		t.Fatalf("block the index name: %v", err)
	}
	t.Cleanup(func() { db.Exec(`DROP TABLE IF EXISTS idx_alerts_tenant_alert_id`) })

	c, err := database.NewClient(testdb.DatabaseConfig(t, dsn), logger.NewNoop())
	if err != nil {
		t.Fatalf("startup failed because the lookup index could not be built: %v", err)
	}
	_ = c.Close()

	if err := db.Exec(`DROP TABLE idx_alerts_tenant_alert_id`).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Exec(`DROP INDEX IF EXISTS idx_alerts_tenant_alert_id`) })
	retried, err := database.NewClient(testdb.DatabaseConfig(t, dsn), logger.NewNoop())
	if err != nil {
		t.Fatalf("NewClient on the next start: %v", err)
	}
	_ = retried.Close()
	var valid []bool
	db.Raw(`SELECT i.indisvalid FROM pg_index i JOIN pg_class ic ON ic.oid = i.indexrelid
		WHERE ic.relname = 'idx_alerts_tenant_alert_id'`).Scan(&valid)
	if len(valid) != 1 || !valid[0] {
		t.Fatalf("idx_alerts_tenant_alert_id after the next start = %v, want one valid index", valid)
	}
}
