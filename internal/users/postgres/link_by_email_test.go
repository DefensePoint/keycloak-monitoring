package postgres

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"gorm.io/gorm"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/users"
	"github.com/DefensePoint/keycloak-monitoring/pkg/database"
)

// An identity provider's subject is stable only while the provider is. These
// tests cover the case where it is not — and, at greater length, every case
// where a familiar-looking address must not be enough, because that is where
// linking by email stops being a convenience and becomes a way in.

// newLinkingRepo is newDeletedAccountRepo with linking by verified email
// switched on, as an administrator would for a planned rebuild. Every condition
// below must hold even then.
func newLinkingRepo(t *testing.T) (*Repository, *gorm.DB) {
	t.Helper()
	r, db := newDeletedAccountRepo(t)
	r.linkByVerifiedEmail = true
	return r, db
}

// signInAs is signIn with the verified flag under the test's control; the
// address being verified is the only thing separating adoption from a takeover.
func signInAs(r *Repository, subject, email string, verified bool) (*domain.User, error) {
	return r.FindOrCreateBySubject(context.Background(), &domain.User{
		Subject: subject, Email: email, EmailVerified: verified,
		AuthMethod: domain.AuthMethodOAuth, IsActive: true,
	})
}

func TestLinkByEmail_ARebuiltRealmKeepsTheSameAccount(t *testing.T) {
	r, db := newLinkingRepo(t)

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
	r, db := newLinkingRepo(t)

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
	r, db := newLinkingRepo(t)

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
	r, db := newLinkingRepo(t)

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
	r, db := newLinkingRepo(t)

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
	r, _ := newLinkingRepo(t)

	u, err := signInAs(r, "del-newcomer-sub", "del-newcomer@example.invalid", true)
	if err != nil {
		t.Fatalf("a new person could not sign in: %v", err)
	}
	if u.Subject != "del-newcomer-sub" {
		t.Errorf("unexpected subject %q", u.Subject)
	}
}

// auth_method is a label that can lag behind the account: rows written before
// the column existed, or with it left unset, read "" or the "simple" default
// until the boot-time migration repairs them. Adoption keys on the password, so
// a stale label must not stop it: the person would silently get a second
// account, which is the outcome this feature exists to prevent.
func TestLinkByEmail_AStaleAuthMethodIsStillAdopted(t *testing.T) {
	for _, stale := range []string{"", "simple"} {
		t.Run(fmt.Sprintf("auth_method=%q", stale), func(t *testing.T) {
			r, db := newLinkingRepo(t)

			first, err := signInAs(r, "del-sync-old", "del-sync@example.invalid", true)
			if err != nil {
				t.Fatalf("first sign-in: %v", err)
			}
			if err := db.Exec("UPDATE users SET auth_method = ? WHERE id = ?", stale, first.ID).Error; err != nil {
				t.Fatalf("set stale auth_method: %v", err)
			}

			second, err := signInAs(r, "del-sync-new", "del-sync@example.invalid", true)
			if err != nil {
				t.Fatalf("sign-in after the realm was rebuilt: %v", err)
			}
			if second.ID != first.ID {
				t.Errorf("an account with auth_method %q was not adopted (id %d then %d), so a stale "+
					"label still splits the person into two accounts after a rebuild", stale, first.ID, second.ID)
			}
		})
	}
}

// Off by default, because a verified address does not show the login is the
// person holding the account: a realm allowing duplicate emails, a trusted
// identity broker, or a realm user administrator can each produce a second
// identity carrying it. With linking off, a new subject arriving with the
// address of an existing account is refused by name, and the account is left
// exactly as it was.
func TestLinkByEmail_OffByDefault_ANewSubjectIsRefusedNotAdopted(t *testing.T) {
	r, db := newDeletedAccountRepo(t)

	victim, err := signInAs(r, "del-off-victim", "del-off@example.invalid", true)
	if err != nil {
		t.Fatalf("first sign-in: %v", err)
	}

	got, err := signInAs(r, "del-off-other", "del-off@example.invalid", true)
	if err == nil {
		t.Fatalf("a different subject with the same verified address was let in as account %d", got.ID)
	}
	var inUse *users.EmailInUseError
	if !errors.As(err, &inUse) {
		t.Fatalf("refused, but not as ErrEmailInUse: %v", err)
	}
	if inUse.UserID != victim.ID {
		t.Errorf("the refusal names account %d, want the one holding the address, %d", inUse.UserID, victim.ID)
	}

	var stored database.User
	if err := db.Where("id = ?", victim.ID).First(&stored).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if stored.Subject != "del-off-victim" {
		t.Errorf("the account was taken over: subject is now %q", stored.Subject)
	}
	var n int64
	db.Model(&database.User{}).Where("email = ?", "del-off@example.invalid").Count(&n)
	if n != 1 {
		t.Errorf("expected one account for the address, found %d", n)
	}

	// The original identity still gets in.
	again, err := signInAs(r, "del-off-victim", "del-off@example.invalid", true)
	if err != nil || again.ID != victim.ID {
		t.Errorf("the account holder could not sign in afterwards: id %v, err %v", again, err)
	}
}

// An unverified address colliding is refused the same way, by name, rather
// than as a constraint violation from the insert.
func TestLinkByEmail_OffByDefault_AnUnverifiedCollisionIsNamed(t *testing.T) {
	r, _ := newDeletedAccountRepo(t)

	if _, err := signInAs(r, "del-offu-victim", "del-offu@example.invalid", true); err != nil {
		t.Fatalf("first sign-in: %v", err)
	}
	_, err := signInAs(r, "del-offu-other", "del-offu@example.invalid", false)
	if !errors.Is(err, users.ErrEmailInUse) {
		t.Fatalf("want ErrEmailInUse, got %v", err)
	}
}

// Switching it off changes nothing for anyone signing in as themselves.
func TestLinkByEmail_OffByDefault_ANewPersonStillGetsAnAccount(t *testing.T) {
	r, _ := newDeletedAccountRepo(t)

	u, err := signInAs(r, "del-offnew-sub", "del-offnew@example.invalid", true)
	if err != nil {
		t.Fatalf("a new person could not sign in: %v", err)
	}
	again, err := signInAs(r, "del-offnew-sub", "del-offnew@example.invalid", true)
	if err != nil || again.ID != u.ID {
		t.Fatalf("a returning person did not get their account back: %v, %v", again, err)
	}
}
