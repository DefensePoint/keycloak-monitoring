//go:build integration

// End-to-end coverage for refusing a deactivated or blocked account on the SSO
// path, against a real Keycloak.
//
// auth/service_callback_test.go already covers both states, but it stubs the
// provider, so it proves the guard reads the persisted row and nothing about
// what someone signing in through a real IdP experiences. That distinction is
// the whole point of the fix: the profile assembled from the IdP claims always
// carries IsActive true, because the same struct doubles as the create payload
// for a first-time SSO user. A test that cannot tell the two apart cannot see
// the bug.
//
// The sibling suite covers a deleted account. Deactivated and blocked are
// different columns on a live row and were the ones the OAuth2 path ignored.
package main_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/auth"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/pkg/database"
	userspg "github.com/DefensePoint/keycloak-monitoring/users/postgres"
)

// accountStatusCase is one way an account can be unusable while its row is
// still live, with the column the platform sets to make it so.
//
// There is deliberately no per-case expected message: refuseUnusableAccount
// answers both states with the single string "account is inactive or blocked",
// which contains both words, so asserting per-case text would look like it
// distinguished them and would in fact pass either way. What separates the two
// cases is which column is set, and the guard refusing on it.
type accountStatusCase struct {
	name   string
	column string
	value  any
}

func accountStatusCases() []accountStatusCase {
	return []accountStatusCase{
		{name: "deactivated", column: "is_active", value: false},
		{name: "blocked", column: "is_blocked", value: true},
	}
}

// The case the fix exists for: a real Keycloak issues a real token for somebody
// this platform has deactivated or blocked, and the sign-in must be refused.
//
// Verified as broken before the fix: a user with is_active=false completed the
// authorization-code flow and /auth/userinfo returned their full identity with
// HTTP 200.
func TestAuthOIDCE2E_DeactivatedOrBlockedAccountIsRefused(t *testing.T) {
	for _, tc := range accountStatusCases() {
		t.Run(tc.name, func(t *testing.T) {
			kcURL, dsn := e2eEnv(t)
			ctx := context.Background()

			admin := newKCAdmin(t, kcURL)
			admin.provisionRealm(t)

			_, db := newBootedClient(t, dsn)

			provider, err := auth.NewOIDCProvider(ctx, &auth.Config{
				ProviderURL:  kcURL + "/realms/" + e2eRealm,
				ClientID:     e2eClientID,
				ClientSecret: e2eClientSecret,
				RedirectURL:  "http://localhost:7880/callback",
				Scopes:       []string{"openid", "email", "profile"},
			}, logger.NewNoop())
			if err != nil {
				t.Fatalf("NewOIDCProvider: %v", err)
			}
			svc := auth.NewService(provider, auth.NewMemorySessionStore(time.Hour),
				userspg.NewRepository(db), logger.NewNoop(), &auth.Config{})

			u := e2eUsers[0]

			// First sign-in creates the row, exactly as a new SSO user would.
			if _, err := svc.FindOrCreateUser(ctx,
				mustVerify(t, ctx, provider, idTokenFor(t, kcURL, u.username))); err != nil {
				t.Fatalf("first sign-in: %v", err)
			}

			// The platform deactivates or blocks them. Keycloak is untouched and
			// keeps issuing perfectly valid tokens, which is the point: the IdP
			// answers who someone is, only our row answers whether they may come in.
			if err := db.Model(&database.User{}).Where("email = ?", u.email).
				Update(tc.column, tc.value).Error; err != nil {
				t.Fatalf("set %s: %v", tc.column, err)
			}

			_, err = svc.FindOrCreateUser(ctx,
				mustVerify(t, ctx, provider, idTokenFor(t, kcURL, u.username)))
			if err == nil {
				t.Fatalf("a %s account signed in through SSO; %s somebody has to stop them "+
					"getting in, or it is not a revocation", tc.name, tc.name)
			}
			// The refusal must name the account status rather than surface a
			// database error or a fabricated server fault, which is what makes
			// the 401 at the HTTP layer honest.
			if !strings.Contains(strings.ToLower(err.Error()), "inactive or blocked") {
				t.Errorf("the refusal should name the account status, got: %v", err)
			}

			// The status must not have been quietly reset on the way through, and
			// no second account created to get around it.
			var rows int64
			db.Model(&database.User{}).Where("email = ? AND "+tc.column+" = ?", u.email, tc.value).
				Count(&rows)
			if rows != 1 {
				t.Errorf("expected exactly one row still %s for %s, got %d", tc.name, u.email, rows)
			}
		})
	}
}

// The other half: somebody active and unblocked must still get in. A guard that
// refuses everyone would pass the test above and lock the platform out.
func TestAuthOIDCE2E_AnActiveAccountStillSignsIn(t *testing.T) {
	kcURL, dsn := e2eEnv(t)
	ctx := context.Background()

	admin := newKCAdmin(t, kcURL)
	admin.provisionRealm(t)

	_, db := newBootedClient(t, dsn)

	provider, err := auth.NewOIDCProvider(ctx, &auth.Config{
		ProviderURL:  kcURL + "/realms/" + e2eRealm,
		ClientID:     e2eClientID,
		ClientSecret: e2eClientSecret,
		RedirectURL:  "http://localhost:7880/callback",
		Scopes:       []string{"openid", "email", "profile"},
	}, logger.NewNoop())
	if err != nil {
		t.Fatalf("NewOIDCProvider: %v", err)
	}
	svc := auth.NewService(provider, auth.NewMemorySessionStore(time.Hour),
		userspg.NewRepository(db), logger.NewNoop(), &auth.Config{})

	u := e2eUsers[0]

	if _, err := svc.FindOrCreateUser(ctx,
		mustVerify(t, ctx, provider, idTokenFor(t, kcURL, u.username))); err != nil {
		t.Fatalf("first sign-in: %v", err)
	}
	user, err := svc.FindOrCreateUser(ctx,
		mustVerify(t, ctx, provider, idTokenFor(t, kcURL, u.username)))
	if err != nil {
		t.Fatalf("an active, unblocked account was refused: %v", err)
	}
	if user == nil {
		t.Fatal("no user returned for an active account")
	}
}

// Reactivating has to restore access, or deactivation is a one-way door and an
// operator who deactivates by mistake has no way back.
func TestAuthOIDCE2E_ReactivatedAccountCanSignInAgain(t *testing.T) {
	kcURL, dsn := e2eEnv(t)
	ctx := context.Background()

	admin := newKCAdmin(t, kcURL)
	admin.provisionRealm(t)

	_, db := newBootedClient(t, dsn)

	provider, err := auth.NewOIDCProvider(ctx, &auth.Config{
		ProviderURL:  kcURL + "/realms/" + e2eRealm,
		ClientID:     e2eClientID,
		ClientSecret: e2eClientSecret,
		RedirectURL:  "http://localhost:7880/callback",
		Scopes:       []string{"openid", "email", "profile"},
	}, logger.NewNoop())
	if err != nil {
		t.Fatalf("NewOIDCProvider: %v", err)
	}
	svc := auth.NewService(provider, auth.NewMemorySessionStore(time.Hour),
		userspg.NewRepository(db), logger.NewNoop(), &auth.Config{})

	u := e2eUsers[0]

	if _, err := svc.FindOrCreateUser(ctx,
		mustVerify(t, ctx, provider, idTokenFor(t, kcURL, u.username))); err != nil {
		t.Fatalf("first sign-in: %v", err)
	}
	if err := db.Model(&database.User{}).Where("email = ?", u.email).
		Update("is_active", false).Error; err != nil {
		t.Fatalf("deactivate: %v", err)
	}
	if _, err := svc.FindOrCreateUser(ctx,
		mustVerify(t, ctx, provider, idTokenFor(t, kcURL, u.username))); err == nil {
		t.Fatal("a deactivated account signed in")
	}

	if err := db.Model(&database.User{}).Where("email = ?", u.email).
		Update("is_active", true).Error; err != nil {
		t.Fatalf("reactivate: %v", err)
	}
	if _, err := svc.FindOrCreateUser(ctx,
		mustVerify(t, ctx, provider, idTokenFor(t, kcURL, u.username))); err != nil {
		t.Fatalf("a reactivated account was still refused: %v", err)
	}
}
