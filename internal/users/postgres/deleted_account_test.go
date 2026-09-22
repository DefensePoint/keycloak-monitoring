package postgres

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/DefensePoint/keycloak-monitoring/internal/config"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/internal/testdb"
	"github.com/DefensePoint/keycloak-monitoring/internal/users"
	"github.com/DefensePoint/keycloak-monitoring/pkg/database"
)

// A soft delete hides the row from every ordinary query, so a returning user
// looks like somebody the platform has never seen and would otherwise be handed
// a second account. These tests cover the refusal and, just as importantly, its
// limits: a check that matched too loosely would lock out people who were never
// deleted at all.

func newDeletedAccountRepo(t *testing.T) (*Repository, *gorm.DB) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping database test in short mode")
	}
	dsn := os.Getenv("KMT_DESTRUCTIVE_TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("KMT_DESTRUCTIVE_TEST_DATABASE_DSN not set; skipping destructive integration test")
	}
	testdb.RequireDisposable(t, dsn)

	// Boot the real client rather than auto-migrating the model alone. The
	// unique indexes on users only reach their production shape through the
	// migrations AutoMigrate runs on top of GORM's output, and without them
	// every OAuth user here collides on an empty username — a property of the
	// harness, not of the behaviour under test.
	client, err := database.NewClient(deletedAccountDBConfig(t, dsn), logger.NewNoop())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	// Both connections are closed at the end. Without this, a run with -count
	// exhausts PostgreSQL's connection slots long before it exhausts the
	// interleavings it was trying to explore.
	t.Cleanup(func() { _ = client.Close() })

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})

	clean := func() {
		db.Unscoped().Where("subject LIKE ? OR email LIKE ?", "del-%", "del-%").Delete(&database.User{})
	}
	clean()
	t.Cleanup(clean)

	db.Exec(`SELECT setval(pg_get_serial_sequence('users', 'id'),
		GREATEST((SELECT COALESCE(MAX(id), 0) FROM users), 1))`)

	return &Repository{db: db}, db
}

func deletedAccountDBConfig(t *testing.T, dsn string) *config.DatabaseConfig {
	t.Helper()
	parsed, err := pgconn.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse KMT_DESTRUCTIVE_TEST_DATABASE_DSN: %v", err)
	}
	sslMode := "require"
	if parsed.TLSConfig == nil {
		sslMode = "disable"
	}
	return &config.DatabaseConfig{
		Host: parsed.Host, Port: int(parsed.Port), Database: parsed.Database,
		User: parsed.User, Password: parsed.Password, SSLMode: sslMode,
		MaxConns: 5, MinConns: 1, Timeout: 30 * time.Second,
	}
}

// signIn is what the OAuth paths do: hand the repository a profile built from
// the IdP's claims and let it find or create the row.
func signIn(r *Repository, subject, email string) (*domain.User, error) {
	return r.FindOrCreateBySubject(context.Background(), &domain.User{
		Subject: subject, Email: email, AuthMethod: domain.AuthMethodOAuth, IsActive: true,
	})
}

func TestFindOrCreate_RefusesADeletedAccount(t *testing.T) {
	r, db := newDeletedAccountRepo(t)

	if _, err := signIn(r, "del-subject", "del-person@example.invalid"); err != nil {
		t.Fatalf("first sign-in: %v", err)
	}
	if err := db.Where("subject = ?", "del-subject").Delete(&database.User{}).Error; err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	_, err := signIn(r, "del-subject", "del-person@example.invalid")
	if !errors.Is(err, users.ErrUserDeleted) {
		t.Fatalf("a deleted account signed back in, or failed for the wrong reason: %v", err)
	}

	var live int64
	db.Model(&database.User{}).Where("subject = ?", "del-subject").Count(&live)
	if live != 0 {
		t.Errorf("a replacement account was created while refusing: found %d live rows", live)
	}
}

// A rebuilt realm hands a familiar person a new subject. The address is what
// still identifies them, and it is the identifier an administrator recognises,
// so a deletion has to survive that.
func TestFindOrCreate_RefusesADeletedAccountUnderANewSubject(t *testing.T) {
	r, db := newDeletedAccountRepo(t)

	if _, err := signIn(r, "del-old-subject", "del-same@example.invalid"); err != nil {
		t.Fatalf("first sign-in: %v", err)
	}
	if err := db.Where("subject = ?", "del-old-subject").Delete(&database.User{}).Error; err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	if _, err := signIn(r, "del-new-subject", "del-same@example.invalid"); !errors.Is(err, users.ErrUserDeleted) {
		t.Fatalf("a new subject let a deleted person back in: %v", err)
	}
}

func TestFindOrCreate_UnrelatedPeopleAreUnaffected(t *testing.T) {
	r, db := newDeletedAccountRepo(t)

	if _, err := signIn(r, "del-gone", "del-gone@example.invalid"); err != nil {
		t.Fatalf("first sign-in: %v", err)
	}
	if err := db.Where("subject = ?", "del-gone").Delete(&database.User{}).Error; err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	if _, err := signIn(r, "del-newcomer", "del-newcomer@example.invalid"); err != nil {
		t.Errorf("somebody else's deletion blocked an unrelated sign-in: %v", err)
	}
}

// The columns matched on are blank for whole classes of account, and a deleted
// row carrying a blank one must not stand for every future account of that
// shape. This is the check's most dangerous failure mode: it would refuse
// logins forever and look like the feature working.
func TestFindOrCreate_ABlankColumnMatchesNobody(t *testing.T) {
	r, db := newDeletedAccountRepo(t)

	// A deleted account that never had an email, as Keycloak accounts often
	// do not.
	deleted := &database.User{Subject: "del-noemail", Email: "", AuthMethod: "oauth", IsActive: true}
	if err := db.Create(deleted).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := db.Delete(deleted).Error; err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	for i := 0; i < 2; i++ {
		subject := fmt.Sprintf("del-fresh-%d", i)
		if _, err := signIn(r, subject, ""); err != nil {
			t.Fatalf("an account with no email was refused because a different account with no "+
				"email had been deleted: %v", err)
		}
	}
}
