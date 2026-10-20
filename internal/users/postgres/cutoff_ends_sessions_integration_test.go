// Deactivating, blocking, unblocking, reactivating and changing the password
// end the user's web sessions by moving their session epoch, in the same
// transaction as the change. A session cookie older than the epoch is refused
// by the auth middleware. Editing anything else leaves sessions alone.
//
// Requires a real PostgreSQL instance via KMT_TEST_DATABASE_DSN; skipped when
// unset. Only rows with an upd- subject are touched.
package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/users"
)

func TestCuttingOffAccessEndsTheUsersSessions(t *testing.T) {
	r, db := newUpdateFlagsRepo(t)
	ctx := context.Background()

	for _, tc := range []struct {
		name          string
		active, block bool
		change        func(t *testing.T, id uint, subject string)
		ends          bool
	}{
		{"deactivate through Update", true, false, func(t *testing.T, id uint, _ string) {
			u := storedDomainUser(t, r, id)
			u.IsActive = false
			must(t, r.Update(ctx, u))
		}, true},
		{"block through Update", true, false, func(t *testing.T, id uint, _ string) {
			u := storedDomainUser(t, r, id)
			u.IsBlocked = true
			must(t, r.Update(ctx, u))
		}, true},
		{"reactivate through Update", false, false, func(t *testing.T, id uint, _ string) {
			u := storedDomainUser(t, r, id)
			u.IsActive = true
			must(t, r.Update(ctx, u))
		}, true},
		{"deactivate through Delete", true, false, func(t *testing.T, id uint, _ string) { must(t, r.Delete(ctx, id)) }, true},
		{"BlockUser", true, false, func(t *testing.T, _ uint, s string) { must(t, r.BlockUser(ctx, s, "misuse")) }, true},
		{"UnblockUser", true, true, func(t *testing.T, _ uint, s string) { must(t, r.UnblockUser(ctx, s)) }, true},
		{"password through Update", true, false, func(t *testing.T, id uint, _ string) {
			u := storedDomainUser(t, r, id)
			u.PasswordHash = "$2a$10$new-hash"
			must(t, r.Update(ctx, u))
		}, true},
		{"UpdatePassword", true, false, func(t *testing.T, id uint, _ string) { must(t, r.UpdatePassword(ctx, id, "$2a$10$new-hash")) }, true},
		{"UpdatePasswordWithFlags", true, false, func(t *testing.T, id uint, _ string) {
			now := time.Now()
			must(t, r.UpdatePasswordWithFlags(ctx, id, "$2a$10$new-hash", &now, false))
		}, true},
		{"edit the name only", true, false, func(t *testing.T, id uint, _ string) {
			u := storedDomainUser(t, r, id)
			u.Name = "After"
			must(t, r.Update(ctx, u))
		}, false},
		{"Delete an already inactive user", false, false, func(t *testing.T, id uint, _ string) { must(t, r.Delete(ctx, id)) }, false},
		{"BlockUser an already blocked user", true, true, func(t *testing.T, _ uint, s string) { must(t, r.BlockUser(ctx, s, "again")) }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			u := seedUser(t, db, tc.active, tc.block)
			must(t, db.Exec("UPDATE users SET password_hash = '$2a$10$old-hash' WHERE id = ?", u.ID).Error)
			other := seedUser(t, db, true, false)
			before, othersBefore := storedUser(t, db, u.ID).SessionEpoch, storedUser(t, db, other.ID).SessionEpoch

			tc.change(t, u.ID, u.Subject)

			after := storedUser(t, db, u.ID).SessionEpoch
			if ended := after > before; ended != tc.ends {
				t.Errorf("session epoch %d -> %d; want sessions ended: %v", before, after, tc.ends)
			}
			if got := storedUser(t, db, other.ID).SessionEpoch; got != othersBefore {
				t.Errorf("another user's epoch moved %d -> %d", othersBefore, got)
			}
		})
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func storedDomainUser(t *testing.T, r *Repository, id uint) *domain.User {
	t.Helper()
	u, err := r.GetByID(context.Background(), id)
	if err != nil || u == nil {
		t.Fatalf("read user %d: %v", id, err)
	}
	return u
}

// An unknown subject is ErrUserNotFound, which the middleware refuses on, and
// keeps its message.
func TestGetBySubject_AnUnknownSubjectIsErrUserNotFound(t *testing.T) {
	r, _ := newUpdateFlagsRepo(t)
	_, err := r.GetBySubject(context.Background(), "upd-nobody-has-this")
	if !errors.Is(err, users.ErrUserNotFound) || err.Error() != "user not found: upd-nobody-has-this" {
		t.Errorf("err = %v, want ErrUserNotFound with the old message", err)
	}
}
