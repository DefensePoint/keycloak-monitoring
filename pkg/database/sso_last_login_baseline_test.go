package database

import (
	"testing"
	"time"
)

// baselineSSOLastLogin gives SSO users with no recorded sign-in a start for
// the MCP sign-in limit. These tests pin who it stamps, that a later run stamps
// only new NULL rows, and that it stops the email_verified migration from
// clearing a claim on every restart.

func lastLoginOf(t *testing.T, c *Client, id uint) *time.Time {
	t.Helper()
	var got User
	if err := c.db.Unscoped().First(&got, id).Error; err != nil {
		t.Fatalf("failed to reload user %d: %v", id, err)
	}
	return got.LastLoginAt
}

func TestBaselineSSOLastLogin_StampsOnlyLiveSSOUsersWithNone(t *testing.T) {
	c := newEmailVerifiedTestClient(t)

	signedIn := time.Now().Add(-48 * time.Hour).UTC().Truncate(time.Second)
	ssoNone := seedUser(t, c, "ev-migration-sso-none@example.com", "oauth", false, nil)
	ssoSignedIn := seedUser(t, c, "ev-migration-sso-signed-in@example.com", "oauth", false, &signedIn)
	localNone := seedUser(t, c, "ev-migration-local-none@example.com", "simple", false, nil)
	ssoDeleted := seedUser(t, c, "ev-migration-sso-deleted@example.com", "oauth", false, nil)
	if err := c.db.Exec(`UPDATE users SET deleted_at = now() WHERE id = ?`, ssoDeleted).Error; err != nil {
		t.Fatalf("soft-delete: %v", err)
	}

	before := time.Now().Add(-time.Second)
	if err := c.baselineSSOLastLogin(); err != nil {
		t.Fatalf("baselineSSOLastLogin: %v", err)
	}

	if got := lastLoginOf(t, c, ssoNone); got == nil || got.Before(before) {
		t.Errorf("SSO user with none: last_login_at = %v, want the startup time", got)
	}
	if got := lastLoginOf(t, c, ssoSignedIn); got == nil || !got.Equal(signedIn) {
		t.Errorf("SSO user who signed in: last_login_at = %v, want %v kept", got, signedIn)
	}
	if got := lastLoginOf(t, c, localNone); got != nil {
		t.Errorf("local user: last_login_at = %v, want it left NULL", got)
	}
	if got := lastLoginOf(t, c, ssoDeleted); got != nil {
		t.Errorf("deleted SSO user: last_login_at = %v, want it left NULL", got)
	}
}

// The limit lets an SSO user with no recorded sign-in through, so an account
// that appears without one must get a clock at the next startup, and a row
// stamped earlier must keep its time rather than be pushed forward on every
// boot.
func TestBaselineSSOLastLogin_StampsLaterAccountsAndKeepsEarlierStamps(t *testing.T) {
	c := newEmailVerifiedTestClient(t)
	first := seedUser(t, c, "ev-migration-sso-first@example.com", "oauth", false, nil)
	if err := c.baselineSSOLastLogin(); err != nil {
		t.Fatalf("first run: %v", err)
	}
	stamped := lastLoginOf(t, c, first)

	later := seedUser(t, c, "ev-migration-sso-later@example.com", "oauth", false, nil)
	time.Sleep(10 * time.Millisecond)
	if err := c.baselineSSOLastLogin(); err != nil {
		t.Fatalf("second run: %v", err)
	}
	if got := lastLoginOf(t, c, later); got == nil {
		t.Error("SSO user added later: last_login_at is NULL, want it stamped")
	}
	if got := lastLoginOf(t, c, first); got == nil || stamped == nil || !got.Equal(*stamped) {
		t.Errorf("SSO user stamped earlier: last_login_at = %v, want %v kept", got, stamped)
	}
}

// Before this release no SSO sign-in recorded last_login_at, so the
// email_verified migration, which runs on every boot, cleared every SSO user's
// claim on every restart. After the baseline it leaves a claim alone.
func TestBaselineSSOLastLogin_StopsTheEmailVerifiedResetOnRestart(t *testing.T) {
	c := newEmailVerifiedTestClient(t)
	id := seedUser(t, c, "ev-migration-sso-verified@example.com", "oauth", false, nil)

	// The upgrade boot, in AutoMigrate's order.
	if err := c.migrateUnprovenEmailVerified(); err != nil {
		t.Fatalf("migrateUnprovenEmailVerified: %v", err)
	}
	if err := c.baselineSSOLastLogin(); err != nil {
		t.Fatalf("baselineSSOLastLogin: %v", err)
	}
	// The IdP asserts the address on the user's next sign-in.
	if err := c.db.Exec(`UPDATE users SET email_verified = true WHERE id = ?`, id).Error; err != nil {
		t.Fatalf("record the claim: %v", err)
	}

	// A later boot.
	if err := c.migrateUnprovenEmailVerified(); err != nil {
		t.Fatalf("migrateUnprovenEmailVerified on restart: %v", err)
	}
	if !emailVerifiedOf(t, c, id) {
		t.Error("email_verified was cleared on restart, want the IdP's claim kept")
	}
}
