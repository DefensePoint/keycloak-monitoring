package postgres

import (
	"context"
	"testing"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/pkg/database"
)

// An identity provider's subject is stable only while the provider is. These
// tests cover the case where it is not — and, at greater length, every case
// where a familiar-looking address must not be enough, because that is where
// linking by email stops being a convenience and becomes a way in.

// signInAs is signIn with the verified flag under the test's control; the
// address being verified is the only thing separating adoption from a takeover.
func signInAs(r *Repository, subject, email string, verified bool) (*domain.User, error) {
	return r.FindOrCreateBySubject(context.Background(), &domain.User{
		Subject: subject, Email: email, EmailVerified: verified,
		AuthMethod: domain.AuthMethodOAuth, IsActive: true,
	})
}

func TestLinkByEmail_ARebuiltRealmKeepsTheSameAccount(t *testing.T) {
	r, db := newDeletedAccountRepo(t)

	first, err := signInAs(r, "del-old-sub", "del-person@example.invalid", true)
	if err != nil {
		t.Fatalf("first sign-in: %v", err)
	}

	// The realm is rebuilt: same person, same address, new sub claim.
	second, err := signInAs(r, "del-new-sub", "del-person@example.invalid", true)
	if err != nil {
		t.Fatalf("sign-in after the realm was rebuilt: %v", err)
	}

	if second.ID != first.ID {
		t.Errorf("a new subject produced a second account (id %d then %d); the person's roles "+
			"and history stay with the row nobody can reach any more", first.ID, second.ID)
	}
	if second.Subject != "del-new-sub" {
		t.Errorf("the account kept the old subject %q, so the next login will not match it either",
			second.Subject)
	}

	var n int64
	db.Model(&database.User{}).Where("email = ?", "del-person@example.invalid").Count(&n)
	if n != 1 {
		t.Errorf("expected one account for the address, found %d", n)
	}
}

// The condition the whole feature rests on. An unverified address is a claim
// somebody made about themselves.
func TestLinkByEmail_AnUnverifiedAddressLinksToNothing(t *testing.T) {
	r, db := newDeletedAccountRepo(t)

	victim, err := signInAs(r, "del-victim", "del-shared@example.invalid", true)
	if err != nil {
		t.Fatalf("first sign-in: %v", err)
	}

	// Somebody else arrives claiming the same address, unverified.
	attacker, err := signInAs(r, "del-attacker", "del-shared@example.invalid", false)
	if err == nil && attacker.ID == victim.ID {
		t.Fatal("an unverified address inherited somebody else's account")
	}

	// Whatever happens, the original account must still belong to its subject.
	var stored database.User
	if err := db.Where("id = ?", victim.ID).First(&stored).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if stored.Subject != "del-victim" {
		t.Errorf("the account was taken over: subject is now %q", stored.Subject)
	}
}

// Adopting a username-and-password account would let an SSO login take over
// local credentials: a wider trust boundary than a realm rebuild needs.
func TestLinkByEmail_ASimpleAuthAccountIsNotAdopted(t *testing.T) {
	r, db := newDeletedAccountRepo(t)

	local := &database.User{
		Subject: "del-local", Email: "del-local@example.invalid", EmailVerified: true,
		Username: "del-local", PasswordHash: "x", AuthMethod: "simple", IsActive: true,
	}
	if err := db.Create(local).Error; err != nil {
		t.Fatalf("seed local account: %v", err)
	}

	got, err := signInAs(r, "del-sso", "del-local@example.invalid", true)
	if err == nil && got.ID == local.ID {
		t.Fatal("an SSO login took over a username-and-password account")
	}

	var stored database.User
	if err := db.Where("id = ?", local.ID).First(&stored).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if stored.Subject != "del-local" || stored.AuthMethod != "simple" {
		t.Errorf("the local account was altered: subject=%q auth_method=%q",
			stored.Subject, stored.AuthMethod)
	}
}

// Accounts with no address are not all the same person.
func TestLinkByEmail_ABlankAddressLinksToNothing(t *testing.T) {
	r, db := newDeletedAccountRepo(t)

	first, err := signInAs(r, "del-blank-1", "", true)
	if err != nil {
		t.Fatalf("first sign-in: %v", err)
	}
	second, err := signInAs(r, "del-blank-2", "", true)
	if err != nil {
		t.Fatalf("second sign-in: %v", err)
	}

	if second.ID == first.ID {
		t.Error("two accounts with no address were treated as the same person")
	}
	var n int64
	db.Model(&database.User{}).Where("email = ?", "").Count(&n)
	if n != 2 {
		t.Errorf("expected two accounts with no address, found %d", n)
	}
}

// Adoption must not resurrect somebody an administrator removed: the deleted
// check runs first, and this pins that ordering.
func TestLinkByEmail_ADeletedAccountIsNotAdopted(t *testing.T) {
	r, db := newDeletedAccountRepo(t)

	gone, err := signInAs(r, "del-gone-sub", "del-gone@example.invalid", true)
	if err != nil {
		t.Fatalf("first sign-in: %v", err)
	}
	if err := db.Delete(&database.User{}, gone.ID).Error; err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	if _, err := signInAs(r, "del-gone-newsub", "del-gone@example.invalid", true); err == nil {
		t.Fatal("a deleted account was adopted under a new subject, so deleting somebody stops " +
			"working the moment their realm is rebuilt")
	}
}

// A first sign-in is still a first sign-in.
func TestLinkByEmail_ANewPersonStillGetsAnAccount(t *testing.T) {
	r, _ := newDeletedAccountRepo(t)

	u, err := signInAs(r, "del-newcomer-sub", "del-newcomer@example.invalid", true)
	if err != nil {
		t.Fatalf("a new person could not sign in: %v", err)
	}
	if u.Subject != "del-newcomer-sub" {
		t.Errorf("unexpected subject %q", u.Subject)
	}
}

// role_sync builds its profile without an AuthMethod and the update writes that
// blank through, so every account Keycloak has synced sits at "" until the next
// restart repairs it. Keying adoption on auth_method therefore skipped exactly
// the accounts a Keycloak deployment has most of, and did so silently: the
// person simply got a second account, which is the outcome this feature exists
// to prevent.
func TestLinkByEmail_ARoleSyncedAccountIsStillAdopted(t *testing.T) {
	r, db := newDeletedAccountRepo(t)

	first, err := signInAs(r, "del-sync-old", "del-sync@example.invalid", true)
	if err != nil {
		t.Fatalf("first sign-in: %v", err)
	}

	// A role sync passes through, shaped exactly as keycloak/role_sync.go does.
	if _, err := r.FindOrCreateBySubject(context.Background(), &domain.User{
		Subject: "del-sync-old", Email: "del-sync@example.invalid",
		EmailVerified: true, IsActive: true,
	}); err != nil {
		t.Fatalf("role sync: %v", err)
	}

	var authMethod string
	db.Raw("SELECT auth_method FROM users WHERE id = ?", first.ID).Scan(&authMethod)
	if authMethod != "" {
		t.Logf("auth_method survived the sync as %q; the premise of this test no longer "+
			"holds, though the assertion below is still the one that matters", authMethod)
	}

	second, err := signInAs(r, "del-sync-new", "del-sync@example.invalid", true)
	if err != nil {
		t.Fatalf("sign-in after the realm was rebuilt: %v", err)
	}
	if second.ID != first.ID {
		t.Errorf("a role-synced account was not adopted (id %d then %d), so anyone Keycloak has "+
			"synced still gets a second account after a rebuild", first.ID, second.ID)
	}
}
