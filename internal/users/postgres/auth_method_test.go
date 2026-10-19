package postgres

import (
	"context"
	"testing"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

// auth_method gates whether an account's identity fields and password can be
// edited locally: user_handlers refuses those edits only for "oauth". A caller
// of FindOrCreateBySubject that leaves AuthMethod unset must therefore leave
// the stored value alone, not blank it and switch that guard off.

func storedAuthMethod(t *testing.T, r *Repository, userID uint) string {
	t.Helper()
	var authMethod string
	if err := r.db.Raw("SELECT auth_method FROM users WHERE id = ?", userID).Scan(&authMethod).Error; err != nil {
		t.Fatalf("read auth_method: %v", err)
	}
	return authMethod
}

func TestAuthMethod_OmittedOnUpdateKeepsOAuth(t *testing.T) {
	r, _ := newDeletedAccountRepo(t)

	user, err := signInAs(r, "del-am-oauth", "del-am-oauth@example.invalid", true)
	if err != nil {
		t.Fatalf("sign-in: %v", err)
	}

	if _, err := r.FindOrCreateBySubject(context.Background(), &domain.User{
		Subject: "del-am-oauth", Email: "del-am-oauth@example.invalid",
		EmailVerified: true, IsActive: true,
	}); err != nil {
		t.Fatalf("update without AuthMethod: %v", err)
	}

	if got := storedAuthMethod(t, r, user.ID); got != string(domain.AuthMethodOAuth) {
		t.Errorf("auth_method = %q after an update that did not set it, want %q", got, domain.AuthMethodOAuth)
	}
}

func TestAuthMethod_OmittedOnUpdateKeepsSimple(t *testing.T) {
	r, _ := newDeletedAccountRepo(t)

	user := &domain.User{
		Subject: "del-am-simple", Username: "del-am-simple", Email: "del-am-simple@example.invalid",
		PasswordHash: "$2a$10$notarealhashnotarealhashnotarealhashnotarealhashnotar",
		AuthMethod:   domain.AuthMethodSimple, IsActive: true,
	}
	if err := r.CreateSimpleAuthUser(context.Background(), user); err != nil {
		t.Fatalf("create simple user: %v", err)
	}

	if _, err := r.FindOrCreateBySubject(context.Background(), &domain.User{
		Subject: "del-am-simple", Email: "del-am-simple@example.invalid", IsActive: true,
	}); err != nil {
		t.Fatalf("update without AuthMethod: %v", err)
	}

	if got := storedAuthMethod(t, r, user.ID); got != string(domain.AuthMethodSimple) {
		t.Errorf("auth_method = %q after an update that did not set it, want %q", got, domain.AuthMethodSimple)
	}
}

// A caller that does pass AuthMethod still has it written: a sign-in repairs a
// row the column was blank on.
func TestAuthMethod_SetOnUpdateIsWritten(t *testing.T) {
	r, _ := newDeletedAccountRepo(t)

	user, err := signInAs(r, "del-am-repair", "del-am-repair@example.invalid", true)
	if err != nil {
		t.Fatalf("first sign-in: %v", err)
	}
	if err := r.db.Exec("UPDATE users SET auth_method = '' WHERE id = ?", user.ID).Error; err != nil {
		t.Fatalf("blank auth_method: %v", err)
	}

	if _, err := signInAs(r, "del-am-repair", "del-am-repair@example.invalid", true); err != nil {
		t.Fatalf("second sign-in: %v", err)
	}

	if got := storedAuthMethod(t, r, user.ID); got != string(domain.AuthMethodOAuth) {
		t.Errorf("auth_method = %q after a sign-in that set it, want %q", got, domain.AuthMethodOAuth)
	}
}
