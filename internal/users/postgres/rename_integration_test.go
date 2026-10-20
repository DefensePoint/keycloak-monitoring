// Renaming a simple-auth user frees their old username and subject: looking
// either up then reports users.ErrUserNotFound, and a new account can take the
// name. The new account gets its own ID, which is what lets the auth
// middleware tell it from the account an old session was issued for.
//
// Requires a real PostgreSQL instance via KMT_TEST_DATABASE_DSN; skipped when
// unset. Only rows with an upd- subject are touched.
package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/users"
	"github.com/DefensePoint/keycloak-monitoring/pkg/database"
)

func TestRenameFreesTheUsernameForAnotherAccount(t *testing.T) {
	r, db := newUpdateFlagsRepo(t)
	ctx := context.Background()

	alice := seedUser(t, db, true, false)
	oldName := alice.Username
	newName := oldName + "-renamed"
	t.Cleanup(func() { db.Unscoped().Where("subject = ?", newName).Delete(&database.User{}) })

	alice.Username, alice.PreferredUsername, alice.Subject = newName, newName, newName
	if err := r.Update(ctx, alice); err != nil {
		t.Fatalf("rename: %v", err)
	}

	if _, err := r.GetByUsername(ctx, oldName); !errors.Is(err, users.ErrUserNotFound) {
		t.Fatalf("old username after the rename: err=%v, want users.ErrUserNotFound", err)
	}
	if _, err := r.GetBySubject(ctx, oldName); !errors.Is(err, users.ErrUserNotFound) {
		t.Fatalf("old subject after the rename: err=%v, want users.ErrUserNotFound", err)
	}
	got, err := r.GetByUsername(ctx, newName)
	if err != nil || got.ID != alice.ID || got.Subject != newName {
		t.Fatalf("new username: user=%+v err=%v, want account %d with subject %s", got, err, alice.ID, newName)
	}

	// A new account takes the freed name.
	reuse := &domain.User{
		Subject: oldName, Username: oldName, PreferredUsername: oldName,
		Email: oldName + "-new@example.invalid", AuthMethod: domain.AuthMethodSimple,
		IsActive: true, PasswordHash: "x",
	}
	if err := r.CreateSimpleAuthUser(ctx, reuse); err != nil {
		t.Fatalf("create an account with the freed name: %v", err)
	}
	holder, err := r.GetBySubject(ctx, oldName)
	if err != nil {
		t.Fatalf("freed subject after reuse: %v", err)
	}
	if holder.ID == alice.ID || holder.ID == 0 {
		t.Fatalf("the new account has ID %d, the renamed one %d; want a different, non-zero ID", holder.ID, alice.ID)
	}
}
