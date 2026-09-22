package postgres

import (
	"errors"
	"testing"

	"github.com/DefensePoint/keycloak-monitoring/internal/users"
	"github.com/DefensePoint/keycloak-monitoring/pkg/database"
)

// A refusal that says only "the account was deleted" leaves an administrator
// with no way to tell which account, which is the first thing they will want to
// know. The repository already found the row in order to refuse on it, so it
// carries that row's id out with the error.

func TestRefusal_CarriesTheDeletedAccountID(t *testing.T) {
	r, db := newDeletedAccountRepo(t)

	gone, err := signIn(r, "del-id-subject", "del-id@example.invalid")
	if err != nil {
		t.Fatalf("first sign-in: %v", err)
	}
	if err := db.Delete(&database.User{}, gone.ID).Error; err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	_, err = signIn(r, "del-id-subject", "del-id@example.invalid")

	// Every existing caller only asks whether the account was deleted, so that
	// has to keep working regardless of the richer error underneath.
	if !errors.Is(err, users.ErrUserDeleted) {
		t.Fatalf("errors.Is(err, ErrUserDeleted) no longer holds, which breaks every caller "+
			"that only cares the account was deleted: %v", err)
	}

	var deleted *users.DeletedAccountError
	if !errors.As(err, &deleted) {
		t.Fatalf("the refusal carries no account id, so a log line about it can only say that "+
			"some account was deleted: %v", err)
	}
	if deleted.UserID != gone.ID {
		t.Errorf("refusal names account %d, want %d", deleted.UserID, gone.ID)
	}
}
