// FindOrCreateBySubject stores the is_active it is given. The column's
// default:true made GORM leave a false value out of the insert, so a user role
// sync created from a disabled Keycloak account was stored as active.
//
// Requires a real PostgreSQL instance via KMT_TEST_DATABASE_DSN; skipped when
// unset. Only rows with an upd- subject are touched.
package postgres

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/pkg/database"
)

func TestFindOrCreateStoresTheRequestedActiveState(t *testing.T) {
	r, db := newUpdateFlagsRepo(t)

	for _, active := range []bool{false, true} {
		t.Run(fmt.Sprintf("active=%v", active), func(t *testing.T) {
			subject := fmt.Sprintf("upd-%d", time.Now().UnixNano())
			t.Cleanup(func() { db.Unscoped().Where("subject = ?", subject).Delete(&database.User{}) })

			got, err := r.FindOrCreateBySubject(context.Background(), &domain.User{
				Subject: subject, Email: subject + "@example.invalid", Username: subject,
				AuthMethod: domain.AuthMethodOAuth, IsActive: active,
			})
			if err != nil {
				t.Fatalf("FindOrCreateBySubject: %v", err)
			}
			if got.IsActive != active {
				t.Errorf("returned IsActive=%v, want %v", got.IsActive, active)
			}
			if row := storedUser(t, db, got.ID); row.IsActive != active {
				t.Errorf("stored is_active=%v, want %v", row.IsActive, active)
			}
		})
	}
}

// Signing in never changes is_active, so a user deactivated by role sync or by
// an admin stays inactive and the login is refused.
func TestSignInLeavesAnInactiveUserInactive(t *testing.T) {
	r, db := newUpdateFlagsRepo(t)
	u := seedUser(t, db, false, false)

	got, err := r.FindOrCreateBySubject(context.Background(), &domain.User{
		Subject: u.Subject, Email: u.Email, IsActive: true,
	})
	if err != nil {
		t.Fatalf("FindOrCreateBySubject: %v", err)
	}
	if got.IsActive || storedUser(t, db, u.ID).IsActive {
		t.Error("signing in reactivated an inactive user")
	}
}
