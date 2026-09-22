// Coverage for the realm scoping the handlers rely on: ListOptions.RealmNames
// and the realm set GetStatistics aggregates over.
//
// Requires a real PostgreSQL instance. The connection DSN is taken from the
// KMT_TEST_DATABASE_DSN environment variable; the test is skipped when
// unset. Run with:
//
//	KMT_TEST_DATABASE_DSN="host=localhost port=5432 dbname=kmt_test user=kmt password=kmt sslmode=disable" \
//	    go test ./alerts/postgres/... -run TestRealmScope
package postgres

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/DefensePoint/keycloak-monitoring/internal/alerts"
	"github.com/DefensePoint/keycloak-monitoring/pkg/database"
)

// newRealmScopeTestRepo opens a connection to KMT_TEST_DATABASE_DSN and
// seeds one active alert in each of three realms of the same tenant.
func newRealmScopeTestRepo(t *testing.T) (*Repository, string) {
	t.Helper()

	dsn := os.Getenv("KMT_TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("KMT_TEST_DATABASE_DSN not set; skipping integration test")
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger:                 gormlogger.Default.LogMode(gormlogger.Silent),
		SkipDefaultTransaction: true,
	})
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	if err := db.AutoMigrate(&database.Alert{}); err != nil {
		t.Fatalf("failed to auto-migrate test tables: %v", err)
	}

	tenantID := fmt.Sprintf("realm-scope-tenant-%d", time.Now().UnixNano())
	t.Cleanup(func() {
		db.Unscoped().Where("tenant_id = ?", tenantID).Delete(&database.Alert{})
	})

	now := time.Now().UTC()
	for _, realm := range []string{"realmA", "realmB", "realmC"} {
		row := &database.Alert{
			TenantID:      tenantID,
			AlertID:       tenantID + "-" + realm,
			Source:        "configuration",
			Type:          "configuration",
			Severity:      "critical",
			Status:        "active",
			Title:         "test alert",
			ResourceType:  "realm",
			ResourceID:    realm,
			ResourceName:  realm,
			RealmName:     realm,
			FirstDetected: now,
			LastSeen:      now,
		}
		if err := db.Create(row).Error; err != nil {
			t.Fatalf("seed alert for %s: %v", realm, err)
		}
	}

	return &Repository{db: db}, tenantID
}

func TestRealmScope_ListFiltersOnRealmNames(t *testing.T) {
	repo, tenantID := newRealmScopeTestRepo(t)
	ctx := context.Background()

	tests := []struct {
		name  string
		opts  *alerts.ListOptions
		want  []string
		count int64
	}{
		{
			name:  "no realm filter returns every realm",
			opts:  &alerts.ListOptions{Limit: 10},
			want:  []string{"realmA", "realmB", "realmC"},
			count: 3,
		},
		{
			name:  "a realm set returns exactly those realms",
			opts:  &alerts.ListOptions{Limit: 10, RealmNames: []string{"realmB", "realmC"}},
			want:  []string{"realmB", "realmC"},
			count: 2,
		},
		{
			name:  "a realm set naming an unknown realm returns nothing",
			opts:  &alerts.ListOptions{Limit: 10, RealmNames: []string{"realmZ"}},
			count: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows, err := repo.List(ctx, tenantID, tt.opts)
			if err != nil {
				t.Fatalf("List: %v", err)
			}
			got := make(map[string]bool, len(rows))
			for _, row := range rows {
				got[row.RealmName] = true
			}
			if len(got) != len(tt.want) {
				t.Fatalf("realms = %v, want %v", got, tt.want)
			}
			for _, realm := range tt.want {
				if !got[realm] {
					t.Errorf("realms = %v, missing %q", got, realm)
				}
			}

			count, err := repo.Count(ctx, tenantID, tt.opts)
			if err != nil {
				t.Fatalf("Count: %v", err)
			}
			if count != tt.count {
				t.Errorf("Count = %d, want %d", count, tt.count)
			}
		})
	}
}

func TestRealmScope_StatisticsAggregateOverEveryRealmGiven(t *testing.T) {
	repo, tenantID := newRealmScopeTestRepo(t)
	ctx := context.Background()

	tests := []struct {
		name   string
		realms []string
		want   int
	}{
		{name: "no realm counts the whole tenant", want: 3},
		{name: "one realm counts only it", realms: []string{"realmB"}, want: 1},
		{name: "several realms count all of them", realms: []string{"realmB", "realmC"}, want: 2},
		{name: "an unknown realm counts nothing", realms: []string{"realmZ"}, want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stats, err := repo.GetStatistics(ctx, tenantID, tt.realms...)
			if err != nil {
				t.Fatalf("GetStatistics: %v", err)
			}
			if stats.TotalActive != tt.want {
				t.Errorf("TotalActive = %d, want %d", stats.TotalActive, tt.want)
			}
			if stats.BySeverity["critical"] != tt.want {
				t.Errorf("BySeverity[critical] = %d, want %d", stats.BySeverity["critical"], tt.want)
			}
			if stats.ByType["configuration"] != tt.want {
				t.Errorf("ByType[configuration] = %d, want %d", stats.ByType["configuration"], tt.want)
			}
		})
	}
}
