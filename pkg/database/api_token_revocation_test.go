package database

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// Users cut off before tokens were revoked with the account kept their tokens
// unrevoked, to come back on reactivation or unblocking. The one-shot
// revocations end them, once, and nothing else.

func clearCutOffTokenRevocations(t *testing.T, c *Client) {
	t.Helper()
	for _, rev := range cutOffTokenRevocations {
		if err := c.DB().Exec(`DELETE FROM data_migrations WHERE key = ?`, rev.key).Error; err != nil {
			t.Fatalf("clear marker: %v", err)
		}
	}
}

func seedRevocationUser(t *testing.T, c *Client, name string, active, blocked bool) uint {
	t.Helper()
	var id uint
	if err := c.DB().Raw(`INSERT INTO users (created_at, updated_at, subject, email, name, username, auth_method, is_active, is_blocked)
		VALUES (now(), now(), ?, ?, ?, ?, 'simple', ?, ?) RETURNING id`,
		name, name+"@example.invalid", name, name, active, blocked).Scan(&id).Error; err != nil {
		t.Fatalf("seed user %s: %v", name, err)
	}
	return id
}

func seedRevocationToken(t *testing.T, c *Client, userID uint, name string, expiresIn time.Duration, revokedAt *time.Time) uint {
	t.Helper()
	exp := time.Now().UTC().Add(expiresIn)
	tok := &APIToken{UserID: userID, Name: name, TenantIDs: []byte(`["t1"]`),
		TokenDigest: fmt.Sprintf("%064x", time.Now().UnixNano()), ExpiresAt: &exp, RevokedAt: revokedAt}
	if err := c.DB().Create(tok).Error; err != nil {
		t.Fatalf("seed token %s: %v", name, err)
	}
	return tok.ID
}

func tokenRevokedAt(t *testing.T, c *Client, id uint) *time.Time {
	t.Helper()
	var tok APIToken
	if err := c.DB().First(&tok, id).Error; err != nil {
		t.Fatalf("read token: %v", err)
	}
	return tok.RevokedAt
}

func revocationMarkers(t *testing.T, c *Client) int64 {
	t.Helper()
	var n int64
	keys := make([]string, 0, len(cutOffTokenRevocations))
	for _, rev := range cutOffTokenRevocations {
		keys = append(keys, rev.key)
	}
	if err := c.DB().Model(&DataMigration{}).Where("key IN ?", keys).Count(&n).Error; err != nil {
		t.Fatalf("count markers: %v", err)
	}
	return n
}

func TestRevokeCutOffUsersTokens_RevokesOnlyTheirLiveTokens(t *testing.T) {
	c := newPurgeTestClient(t)
	clearCutOffTokenRevocations(t, c)

	inactive := seedRevocationUser(t, c, "inactive", false, false)
	blocked := seedRevocationUser(t, c, "blocked", true, true)
	blockedLive := seedRevocationToken(t, c, blocked, "blocked-live", 24*time.Hour, nil)
	active := seedRevocationUser(t, c, "active", true, false)
	live := seedRevocationToken(t, c, inactive, "inactive-live", 24*time.Hour, nil)
	expired := seedRevocationToken(t, c, inactive, "inactive-expired", -time.Hour, nil)
	earlier := time.Now().UTC().Add(-48 * time.Hour).Truncate(time.Microsecond)
	revoked := seedRevocationToken(t, c, inactive, "inactive-revoked", 24*time.Hour, &earlier)
	activeLive := seedRevocationToken(t, c, active, "active-live", 24*time.Hour, nil)

	if err := c.revokeCutOffUsersTokens(); err != nil {
		t.Fatalf("revoke: %v", err)
	}

	if tokenRevokedAt(t, c, live) == nil {
		t.Error("the inactive user's live token was not revoked")
	}
	if tokenRevokedAt(t, c, blockedLive) == nil {
		t.Error("the blocked user's live token was not revoked")
	}
	if got := tokenRevokedAt(t, c, expired); got != nil {
		t.Errorf("the expired token was marked revoked at %v; it should still read as expired", got)
	}
	if got := tokenRevokedAt(t, c, revoked); got == nil || !got.Equal(earlier) {
		t.Errorf("the already revoked token's revoked_at = %v, want its original %v", got, earlier)
	}
	if got := tokenRevokedAt(t, c, activeLive); got != nil {
		t.Errorf("an active user's token was revoked at %v", got)
	}
	if n := revocationMarkers(t, c); n != int64(len(cutOffTokenRevocations)) {
		t.Errorf("markers = %d, want %d", n, len(cutOffTokenRevocations))
	}
}

// A database upgraded by the release that revokes on deactivation has run that
// revocation already. Its blocked users still need theirs.
func TestRevokeCutOffUsersTokens_BlockedUsersAfterTheDeactivationRelease(t *testing.T) {
	c := newPurgeTestClient(t)
	clearCutOffTokenRevocations(t, c)
	if err := c.DB().Create(&DataMigration{Key: "api_tokens.revoke_inactive_users", AppliedAt: time.Now()}).Error; err != nil {
		t.Fatalf("mark the deactivation release's revocation applied: %v", err)
	}

	blocked := seedRevocationUser(t, c, "blocked", true, false)
	if err := c.DB().Exec(`UPDATE users SET is_blocked = true WHERE id = ?`, blocked).Error; err != nil {
		t.Fatalf("block: %v", err)
	}
	blockedLive := seedRevocationToken(t, c, blocked, "blocked-live", 24*time.Hour, nil)
	inactive := seedRevocationUser(t, c, "inactive", false, false)
	issuedSince := seedRevocationToken(t, c, inactive, "issued-since-the-deactivation-release", 24*time.Hour, nil)

	if err := c.revokeCutOffUsersTokens(); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if tokenRevokedAt(t, c, blockedLive) == nil {
		t.Error("the blocked user's live token was not revoked")
	}
	if got := tokenRevokedAt(t, c, issuedSince); got != nil {
		t.Errorf("the deactivation revocation, already applied, ran again and revoked a token at %v", got)
	}
	if n := revocationMarkers(t, c); n != 2 {
		t.Errorf("markers = %d, want 2", n)
	}
}

// Once applied it never runs again: a token an administrator issues to an
// inactive user afterwards is deliberate, and is left alone.
func TestRevokeCutOffUsersTokens_RunsOnce(t *testing.T) {
	c := newPurgeTestClient(t)
	clearCutOffTokenRevocations(t, c)
	inactive := seedRevocationUser(t, c, "inactive", false, false)

	if err := c.revokeCutOffUsersTokens(); err != nil {
		t.Fatalf("first run: %v", err)
	}
	later := seedRevocationToken(t, c, inactive, "issued-after-upgrade", 24*time.Hour, nil)
	if err := c.revokeCutOffUsersTokens(); err != nil {
		t.Fatalf("second run: %v", err)
	}
	if err := c.AutoMigrate(); err != nil {
		t.Fatalf("a later startup: %v", err)
	}
	if got := tokenRevokedAt(t, c, later); got != nil {
		t.Errorf("a token issued after the revocation ran was revoked at %v by a later run", got)
	}
}

// Instances starting together revoke once and record one marker each: the
// rest wait for the first and find its marker, rather than failing on a
// duplicate one. Released at the same instant and repeated, so the attempts
// really overlap.
func TestRevokeCutOffUsersTokens_ConcurrentStartups(t *testing.T) {
	c := newPurgeTestClient(t)
	inactive := seedRevocationUser(t, c, "inactive", false, false)

	for round := 0; round < 15; round++ {
		clearCutOffTokenRevocations(t, c)
		tok := seedRevocationToken(t, c, inactive, fmt.Sprintf("live-%d", round), 24*time.Hour, nil)

		start := make(chan struct{})
		var wg sync.WaitGroup
		errs := make([]error, 12)
		for i := range errs {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				<-start
				errs[i] = c.revokeCutOffUsersTokens()
			}(i)
		}
		close(start)
		wg.Wait()
		for i, err := range errs {
			if err != nil {
				t.Fatalf("round %d, startup %d: %v", round, i, err)
			}
		}
		if tokenRevokedAt(t, c, tok) == nil {
			t.Fatalf("round %d: the token was not revoked", round)
		}
		if n := revocationMarkers(t, c); n != int64(len(cutOffTokenRevocations)) {
			t.Fatalf("round %d: markers = %d, want %d", round, n, len(cutOffTokenRevocations))
		}
	}
}

// A failed attempt leaves no marker and revokes nothing, so the next startup
// retries it.
func TestRevokeCutOffUsersTokens_AFailureIsRetried(t *testing.T) {
	c := newPurgeTestClient(t)
	clearCutOffTokenRevocations(t, c)
	inactive := seedRevocationUser(t, c, "inactive", false, false)
	tok := seedRevocationToken(t, c, inactive, "live", 24*time.Hour, nil)

	for _, stmt := range []string{
		`CREATE FUNCTION test_fail_token_revocation() RETURNS trigger LANGUAGE plpgsql AS $$
			BEGIN RAISE EXCEPTION 'injected failure revoking tokens'; END $$`,
		`CREATE TRIGGER test_fail_token_revocation BEFORE UPDATE ON api_tokens
			FOR EACH ROW EXECUTE FUNCTION test_fail_token_revocation()`,
	} {
		if err := c.DB().Exec(stmt).Error; err != nil {
			t.Fatalf("inject failure: %v", err)
		}
	}
	dropTrigger := func() {
		c.DB().Exec(`DROP TRIGGER IF EXISTS test_fail_token_revocation ON api_tokens`)
		c.DB().Exec(`DROP FUNCTION IF EXISTS test_fail_token_revocation()`)
	}
	t.Cleanup(dropTrigger)

	if err := c.revokeCutOffUsersTokens(); err == nil {
		t.Fatal("a failed revocation reported success")
	}
	if n := revocationMarkers(t, c); n != 0 {
		t.Fatalf("a failed revocation left %d markers, so it would never be retried", n)
	}
	if got := tokenRevokedAt(t, c, tok); got != nil {
		t.Fatalf("token revoked at %v by an attempt that failed", got)
	}

	dropTrigger()
	if err := c.revokeCutOffUsersTokens(); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if tokenRevokedAt(t, c, tok) == nil {
		t.Error("the retry did not revoke the token")
	}
}
