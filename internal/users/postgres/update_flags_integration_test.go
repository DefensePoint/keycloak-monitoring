// Regression coverage for the admin edit path storing false.
//
// Update used Updates with a struct, which skips zero values, so an admin
// deactivating or unblocking a user through PUT /api/users/{id} was silently
// ignored while the API reported success.
//
// Requires a real PostgreSQL instance via KMT_TEST_DATABASE_DSN; skipped when
// unset. Only rows with an upd- subject are touched.
package postgres

import (
	"context"
	"fmt"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/internal/testdb"
	"github.com/DefensePoint/keycloak-monitoring/pkg/database"
)

func newUpdateFlagsRepo(t *testing.T) (*Repository, *gorm.DB) {
	t.Helper()

	dsn := testdb.DSN(t, "KMT_TEST_DATABASE_DSN")
	testdb.Lock(t, dsn)
	// The real client, so users has its production indexes.
	client, err := database.NewClient(deletedAccountDBConfig(t, dsn), logger.NewNoop())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	db := client.DB()
	return &Repository{db: db}, db
}

// seedUser writes the row through a map, so no column default can stand in for
// the state under test.
func seedUser(t *testing.T, db *gorm.DB, active, blocked bool) *domain.User {
	t.Helper()

	subject := fmt.Sprintf("upd-%d", time.Now().UnixNano())
	t.Cleanup(func() { db.Unscoped().Where("subject = ?", subject).Delete(&database.User{}) })
	if err := db.Model(&database.User{}).Create(map[string]any{
		"subject": subject, "email": subject + "@example.invalid", "name": "Before",
		"username": subject, "auth_method": "simple",
		"is_active": active, "is_blocked": blocked, "login_count": 1,
	}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	var row database.User
	if err := db.Where("subject = ?", subject).First(&row).Error; err != nil {
		t.Fatalf("read seed: %v", err)
	}
	return &domain.User{
		ID: row.ID, Subject: subject, Email: row.Email, Name: row.Name,
		Username: row.Username, AuthMethod: domain.AuthMethodSimple,
		IsActive: active, IsBlocked: blocked, LoginCount: 1, UpdatedAt: row.UpdatedAt,
	}
}

func storedUser(t *testing.T, db *gorm.DB, id uint) database.User {
	t.Helper()

	var row database.User
	if err := db.First(&row, id).Error; err != nil {
		t.Fatalf("read user: %v", err)
	}
	return row
}

func TestUpdateStoresTheFlagsBothWays(t *testing.T) {
	r, db := newUpdateFlagsRepo(t)
	u := seedUser(t, db, true, false)
	ctx := context.Background()

	for _, step := range []struct{ active, blocked bool }{
		{false, true}, // deactivate and block
		{true, false}, // reactivate and unblock: the case that was dropped
		{false, false},
	} {
		u.IsActive, u.IsBlocked = step.active, step.blocked
		if err := r.Update(ctx, u); err != nil {
			t.Fatalf("Update(%+v): %v", step, err)
		}
		row := storedUser(t, db, u.ID)
		if row.IsActive != step.active || row.IsBlocked != step.blocked {
			t.Errorf("after Update(%+v) stored is_active=%v is_blocked=%v", step, row.IsActive, row.IsBlocked)
		}
	}
}

func TestUpdateLeavesEmptyIdentityFieldsUnchanged(t *testing.T) {
	r, db := newUpdateFlagsRepo(t)
	u := seedUser(t, db, true, false)
	want := storedUser(t, db, u.ID)

	u.Email, u.Name = "", ""
	if err := r.Update(context.Background(), u); err != nil {
		t.Fatalf("Update: %v", err)
	}
	row := storedUser(t, db, u.ID)
	if row.Email != want.Email || row.Name != want.Name {
		t.Errorf("stored email=%q name=%q, want %q %q unchanged", row.Email, row.Name, want.Email, want.Name)
	}
}

// The handler reads the user, edits it and writes it back. A login landing in
// between must survive the write.
func TestUpdateKeepsAConcurrentLogin(t *testing.T) {
	r, db := newUpdateFlagsRepo(t)
	u := seedUser(t, db, true, false)
	before := storedUser(t, db, u.ID)

	if err := db.Model(&database.User{}).Where("id = ?", u.ID).
		UpdateColumn("login_count", 7).Error; err != nil {
		t.Fatalf("simulate login: %v", err)
	}
	u.Name = "After"
	if err := r.Update(context.Background(), u); err != nil {
		t.Fatalf("Update: %v", err)
	}
	row := storedUser(t, db, u.ID)
	if row.LoginCount != 7 {
		t.Errorf("login_count=%d, want the concurrent login's 7", row.LoginCount)
	}
	if row.Name != "After" {
		t.Errorf("name=%q, want After", row.Name)
	}
	if !row.UpdatedAt.After(before.UpdatedAt) {
		t.Errorf("updated_at did not advance: %v, was %v", row.UpdatedAt, before.UpdatedAt)
	}
}
