// Deactivating or blocking a user revokes their API tokens, and reactivating or
// unblocking them revokes any still live, so restoring the user does not bring
// the tokens back. Validation already refused a token while its user was
// inactive or blocked, but only that: reactivating or unblocking used to
// restore every unexpired token.
//
// Requires a real PostgreSQL instance via KMT_TEST_DATABASE_DSN; skipped when
// unset. Only rows with an upd- subject, and their tokens, are touched.
package postgres

import (
	"context"
	"fmt"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/pkg/database"
)

func seedToken(t *testing.T, db *gorm.DB, userID uint, name string, expiresAt, revokedAt *time.Time) uint {
	t.Helper()

	tok := &database.APIToken{
		UserID: userID, Name: name, TenantIDs: []byte(`["t1"]`),
		TokenDigest: fmt.Sprintf("%064x", time.Now().UnixNano()),
		ExpiresAt:   expiresAt, RevokedAt: revokedAt,
	}
	if err := db.Create(tok).Error; err != nil {
		t.Fatalf("seed token %s: %v", name, err)
	}
	t.Cleanup(func() { db.Delete(&database.APIToken{}, tok.ID) })
	return tok.ID
}

func storedToken(t *testing.T, db *gorm.DB, id uint) database.APIToken {
	t.Helper()

	var row database.APIToken
	if err := db.First(&row, id).Error; err != nil {
		t.Fatalf("read token: %v", err)
	}
	return row
}

func at(d time.Duration) *time.Time {
	v := time.Now().UTC().Add(d).Truncate(time.Microsecond)
	return &v
}

func TestDeactivatingThroughUpdateRevokesOnlyTheUsersLiveTokens(t *testing.T) {
	r, db := newUpdateFlagsRepo(t)
	u := seedUser(t, db, true, false)
	other := seedUser(t, db, true, false)

	live := seedToken(t, db, u.ID, "live", at(24*time.Hour), nil)
	noExpiry := seedToken(t, db, u.ID, "no-expiry", nil, nil)
	expired := seedToken(t, db, u.ID, "expired", at(-time.Hour), nil)
	earlier := at(-48 * time.Hour)
	revoked := seedToken(t, db, u.ID, "revoked", at(24*time.Hour), earlier)
	othersToken := seedToken(t, db, other.ID, "others", at(24*time.Hour), nil)

	u.IsActive = false
	if err := r.Update(context.Background(), u); err != nil {
		t.Fatalf("deactivate: %v", err)
	}

	if storedUser(t, db, u.ID).IsActive {
		t.Fatal("the user is still active")
	}
	for name, id := range map[string]uint{"live": live, "no-expiry": noExpiry} {
		if storedToken(t, db, id).RevokedAt == nil {
			t.Errorf("the %s token was not revoked", name)
		}
	}
	if got := storedToken(t, db, expired).RevokedAt; got != nil {
		t.Errorf("the expired token was marked revoked at %v; it should still read as expired", got)
	}
	if got := storedToken(t, db, revoked).RevokedAt; got == nil || !got.Equal(*earlier) {
		t.Errorf("the already revoked token's revoked_at = %v, want its original %v", got, *earlier)
	}
	if got := storedToken(t, db, othersToken).RevokedAt; got != nil {
		t.Errorf("another user's token was revoked at %v", got)
	}
}

func TestReactivatingDoesNotRestoreTheTokens(t *testing.T) {
	r, db := newUpdateFlagsRepo(t)
	u := seedUser(t, db, true, false)
	tok := seedToken(t, db, u.ID, "live", at(24*time.Hour), nil)
	ctx := context.Background()

	if err := r.Delete(ctx, u.ID); err != nil {
		t.Fatalf("deactivate: %v", err)
	}
	revokedAt := storedToken(t, db, tok).RevokedAt
	if revokedAt == nil {
		t.Fatal("deactivating through Delete did not revoke the token")
	}

	u.IsActive = true
	if err := r.Update(ctx, u); err != nil {
		t.Fatalf("reactivate: %v", err)
	}
	if !storedUser(t, db, u.ID).IsActive {
		t.Fatal("the user was not reactivated")
	}
	if got := storedToken(t, db, tok).RevokedAt; got == nil || !got.Equal(*revokedAt) {
		t.Errorf("after reactivation the token's revoked_at = %v, want it still %v", got, *revokedAt)
	}
}

// Only a change of status revokes. A token an administrator issued while the
// user was inactive survives later edits that keep the user inactive (it ends
// at the reactivation), and an edit that leaves an active user active and
// unblocked revokes nothing.
func TestEditsThatKeepTheStatusRevokeNothing(t *testing.T) {
	r, db := newUpdateFlagsRepo(t)
	ctx := context.Background()

	inactive := seedUser(t, db, false, false)
	issuedWhileInactive := seedToken(t, db, inactive.ID, "issued-while-inactive", at(24*time.Hour), nil)
	inactive.Name = "Renamed"
	if err := r.Update(ctx, inactive); err != nil {
		t.Fatalf("edit inactive user: %v", err)
	}
	if err := r.Delete(ctx, inactive.ID); err != nil {
		t.Fatalf("deactivate an inactive user again: %v", err)
	}
	if got := storedToken(t, db, issuedWhileInactive).RevokedAt; got != nil {
		t.Errorf("editing or re-deactivating an inactive user revoked a token, at %v", got)
	}

	active := seedUser(t, db, true, false)
	tok := seedToken(t, db, active.ID, "live", at(24*time.Hour), nil)
	active.Name = "Renamed"
	if err := r.Update(ctx, active); err != nil {
		t.Fatalf("edit active user: %v", err)
	}
	if got := storedToken(t, db, tok).RevokedAt; got != nil {
		t.Errorf("an edit that left the user active and unblocked revoked a token, at %v", got)
	}
}

func TestBlockingThroughUpdateRevokesAndUnblockingDoesNotRestore(t *testing.T) {
	r, db := newUpdateFlagsRepo(t)
	ctx := context.Background()
	u := seedUser(t, db, true, false)
	tok := seedToken(t, db, u.ID, "live", at(24*time.Hour), nil)
	expired := seedToken(t, db, u.ID, "expired", at(-time.Hour), nil)

	u.IsBlocked = true
	if err := r.Update(ctx, u); err != nil {
		t.Fatalf("block: %v", err)
	}
	if row := storedUser(t, db, u.ID); !row.IsBlocked || !row.IsActive {
		t.Fatalf("after blocking: is_blocked=%v is_active=%v, want blocked and still active", row.IsBlocked, row.IsActive)
	}
	revokedAt := storedToken(t, db, tok).RevokedAt
	if revokedAt == nil {
		t.Fatal("blocking did not revoke the token")
	}
	if got := storedToken(t, db, expired).RevokedAt; got != nil {
		t.Errorf("the expired token was marked revoked at %v; it should still read as expired", got)
	}

	u.IsBlocked = false
	if err := r.Update(ctx, u); err != nil {
		t.Fatalf("unblock: %v", err)
	}
	if storedUser(t, db, u.ID).IsBlocked {
		t.Fatal("the user was not unblocked")
	}
	if got := storedToken(t, db, tok).RevokedAt; got == nil || !got.Equal(*revokedAt) {
		t.Errorf("after unblocking the token's revoked_at = %v, want it still %v", got, *revokedAt)
	}
}

// A token issued while the user was blocked survives later edits that keep
// the user blocked, as one issued while inactive does.
func TestEditingABlockedUserRevokesNothing(t *testing.T) {
	r, db := newUpdateFlagsRepo(t)
	ctx := context.Background()
	u := seedUser(t, db, true, true)
	tok := seedToken(t, db, u.ID, "issued-while-blocked", at(24*time.Hour), nil)

	u.Name = "Renamed"
	if err := r.Update(ctx, u); err != nil {
		t.Fatalf("edit blocked user: %v", err)
	}
	if err := r.BlockUser(ctx, u.Subject, "again"); err != nil {
		t.Fatalf("block again: %v", err)
	}
	if got := storedToken(t, db, tok).RevokedAt; got != nil {
		t.Errorf("editing or re-blocking a blocked user revoked a token, at %v", got)
	}
}

func TestBlockUserRevokes(t *testing.T) {
	r, db := newUpdateFlagsRepo(t)
	ctx := context.Background()
	u := seedUser(t, db, true, false)
	tok := seedToken(t, db, u.ID, "live", at(24*time.Hour), nil)

	if err := r.BlockUser(ctx, u.Subject, "misuse"); err != nil {
		t.Fatalf("BlockUser: %v", err)
	}
	row := storedUser(t, db, u.ID)
	if !row.IsBlocked || row.BlockedReason != "misuse" {
		t.Errorf("after BlockUser: is_blocked=%v reason=%q, want blocked for misuse", row.IsBlocked, row.BlockedReason)
	}
	if storedToken(t, db, tok).RevokedAt == nil {
		t.Error("BlockUser did not revoke the token")
	}

	if err := r.BlockUser(ctx, "upd-no-such-subject", "nobody"); err != nil {
		t.Errorf("BlockUser of an unknown subject = %v, want it to do nothing, as before", err)
	}
	if err := r.UnblockUser(ctx, "upd-no-such-subject"); err != nil {
		t.Errorf("UnblockUser of an unknown subject = %v, want it to do nothing, as before", err)
	}
}

// Cutting the user off and the revocation commit together or not at all: a
// deactivation or block that committed while its revocation failed would leave
// the tokens to come back later, the very thing this prevents.
func TestAFailedRevocationLeavesTheUserActive(t *testing.T) {
	r, db := newUpdateFlagsRepo(t)
	ctx := context.Background()

	for name, deactivate := range map[string]func(u *domain.User) error{
		"update": func(u *domain.User) error {
			u.IsActive = false
			return r.Update(ctx, u)
		},
		"delete": func(u *domain.User) error { return r.Delete(ctx, u.ID) },
		"block through update": func(u *domain.User) error {
			u.IsBlocked = true
			return r.Update(ctx, u)
		},
		"BlockUser": func(u *domain.User) error { return r.BlockUser(ctx, u.Subject, "misuse") },
	} {
		t.Run(name, func(t *testing.T) {
			u := seedUser(t, db, true, false)
			tok := seedToken(t, db, u.ID, "live", at(24*time.Hour), nil)
			failTokenUpdatesFor(t, db, u.ID)

			if err := deactivate(u); err == nil {
				t.Fatal("cutting the user off reported success although revoking the tokens failed")
			}
			if row := storedUser(t, db, u.ID); !row.IsActive || row.IsBlocked {
				t.Errorf("is_active=%v is_blocked=%v: the user was cut off although revoking their tokens failed", row.IsActive, row.IsBlocked)
			}
			if got := storedToken(t, db, tok).RevokedAt; got != nil {
				t.Errorf("token revoked at %v despite the failure", got)
			}
		})
	}
}

// restorations are the ways a cut-off user is restored, each with the status
// the user starts in.
func restorations(r *Repository, ctx context.Context) map[string]struct {
	active, blocked bool
	restore         func(u *domain.User) error
} {
	return map[string]struct {
		active, blocked bool
		restore         func(u *domain.User) error
	}{
		"reactivate through update": {false, false, func(u *domain.User) error {
			u.IsActive = true
			return r.Update(ctx, u)
		}},
		"unblock through update": {true, true, func(u *domain.User) error {
			u.IsBlocked = false
			return r.Update(ctx, u)
		}},
		"UnblockUser": {true, true, func(u *domain.User) error {
			return r.UnblockUser(ctx, u.Subject)
		}},
	}
}

// Restoring a user revokes whatever is still live: a token issued while the
// user was cut off, or one an older release left unrevoked when it cut the
// user off. A restored user starts with no live tokens.
func TestRestoringRevokesTokensStillLive(t *testing.T) {
	r, db := newUpdateFlagsRepo(t)
	ctx := context.Background()

	for name, tc := range restorations(r, ctx) {
		t.Run(name, func(t *testing.T) {
			u := seedUser(t, db, tc.active, tc.blocked)
			stillLive := seedToken(t, db, u.ID, "still-live", at(24*time.Hour), nil)
			expired := seedToken(t, db, u.ID, "expired", at(-time.Hour), nil)

			if err := tc.restore(u); err != nil {
				t.Fatalf("restore: %v", err)
			}
			if row := storedUser(t, db, u.ID); !row.IsActive || row.IsBlocked {
				t.Fatalf("after restoring: is_active=%v is_blocked=%v, want active and unblocked", row.IsActive, row.IsBlocked)
			}
			if storedToken(t, db, stillLive).RevokedAt == nil {
				t.Error("a token still live at the restore was not revoked")
			}
			if got := storedToken(t, db, expired).RevokedAt; got != nil {
				t.Errorf("the expired token was marked revoked at %v; it should still read as expired", got)
			}

			fresh := seedToken(t, db, u.ID, "issued-after-restore", at(24*time.Hour), nil)
			u.IsActive, u.IsBlocked, u.Name = true, false, "Renamed"
			if err := r.Update(ctx, u); err != nil {
				t.Fatalf("edit the restored user: %v", err)
			}
			if got := storedToken(t, db, fresh).RevokedAt; got != nil {
				t.Errorf("a token issued after the restore was revoked at %v by an edit", got)
			}
		})
	}
}

func TestAFailedRevocationLeavesTheUserCutOff(t *testing.T) {
	r, db := newUpdateFlagsRepo(t)
	ctx := context.Background()

	for name, tc := range restorations(r, ctx) {
		t.Run(name, func(t *testing.T) {
			u := seedUser(t, db, tc.active, tc.blocked)
			tok := seedToken(t, db, u.ID, "still-live", at(24*time.Hour), nil)
			failTokenUpdatesFor(t, db, u.ID)

			if err := tc.restore(u); err == nil {
				t.Fatal("restoring reported success although revoking the tokens failed")
			}
			if row := storedUser(t, db, u.ID); row.IsActive != tc.active || row.IsBlocked != tc.blocked {
				t.Errorf("is_active=%v is_blocked=%v: the user was restored although revoking their tokens failed", row.IsActive, row.IsBlocked)
			}
			if got := storedToken(t, db, tok).RevokedAt; got != nil {
				t.Errorf("token revoked at %v despite the failure", got)
			}
		})
	}
}

func TestDeactivatingAMissingUserFails(t *testing.T) {
	r, _ := newUpdateFlagsRepo(t)
	if err := r.Delete(context.Background(), 0); err == nil {
		t.Error("Delete of a user that does not exist reported success")
	}
}

// failTokenUpdatesFor makes any update of this user's API tokens fail until
// the test ends.
func failTokenUpdatesFor(t *testing.T, db *gorm.DB, userID uint) {
	t.Helper()

	fn := fmt.Sprintf("test_fail_token_update_%d", userID)
	for _, stmt := range []string{
		fmt.Sprintf(`CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $$
			BEGIN RAISE EXCEPTION 'injected failure revoking tokens'; END $$`, fn),
		fmt.Sprintf(`CREATE TRIGGER %s BEFORE UPDATE ON api_tokens
			FOR EACH ROW WHEN (OLD.user_id = %d) EXECUTE FUNCTION %s()`, fn, userID, fn),
	} {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("inject failure: %v", err)
		}
	}
	t.Cleanup(func() {
		db.Exec(fmt.Sprintf(`DROP TRIGGER IF EXISTS %s ON api_tokens`, fn))
		db.Exec(fmt.Sprintf(`DROP FUNCTION IF EXISTS %s()`, fn))
	})
}
