package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

// The SSO callback refuses an inactive or blocked account only after
// FindOrCreateBySubject returns, so a refused sign-in still reaches it. The
// MCP token check reads last_login_at as the last sign-in that let the user in,
// so a refused one must leave it alone, and an accepted one must move it.
func TestFindOrCreateRecordsTheSignInTimeOnlyForAUsableAccount(t *testing.T) {
	repo, db := newUpdateFlagsRepo(t)
	ctx := context.Background()
	old := time.Now().Add(-20 * 24 * time.Hour).UTC().Truncate(time.Second)

	for _, tc := range []struct {
		name         string
		active       bool
		blocked      bool
		wantRecorded bool
	}{
		{"active", true, false, true},
		{"blocked", true, true, false},
		{"deactivated", false, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			seeded := seedUser(t, db, tc.active, tc.blocked)
			if err := db.Exec(`UPDATE users SET last_login_at = ? WHERE id = ?`, old, seeded.ID).Error; err != nil {
				t.Fatalf("set an old sign-in: %v", err)
			}

			now := time.Now()
			if _, err := repo.FindOrCreateBySubject(ctx, &domain.User{
				Subject: seeded.Subject, Email: seeded.Email, Name: seeded.Name,
				AuthMethod: domain.AuthMethodOAuth, IsActive: true,
				LastLoginAt: &now, LastAccessedAt: &now,
			}); err != nil {
				t.Fatalf("FindOrCreateBySubject: %v", err)
			}

			got := storedUser(t, db, seeded.ID).LastLoginAt
			switch {
			case got == nil:
				t.Fatal("last_login_at is NULL after the sign-in")
			case tc.wantRecorded && !got.After(old):
				t.Errorf("last_login_at = %v, want this sign-in recorded", got)
			case !tc.wantRecorded && !got.Equal(old):
				t.Errorf("last_login_at = %v, want %v kept: a refused sign-in is not a sign-in", got, old)
			}
		})
	}
}
