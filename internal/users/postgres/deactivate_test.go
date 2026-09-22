package postgres

import (
	"context"
	"testing"

	"github.com/DefensePoint/keycloak-monitoring/pkg/database"
)

// Repository.Delete is named for the interface it satisfies and for the HTTP
// verb that reaches it, and it deactivates. Everything a person reads now says
// so, and these tests keep the code honest about it: a rename that turned this
// into a real deletion would change what "delete a user" means for every
// operator, and should not happen by accident.

func TestDelete_DeactivatesAndRemovesNothing(t *testing.T) {
	r, db := newDeletedAccountRepo(t)

	u := &database.User{
		Subject: "del-deact", Email: "del-deact@example.invalid",
		AuthMethod: "oauth", IsActive: true,
	}
	if err := db.Create(u).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := r.Delete(context.Background(), u.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	var stored database.User
	if err := db.Unscoped().First(&stored, u.ID).Error; err != nil {
		t.Fatalf("the row is gone; this operation is supposed to keep it: %v", err)
	}

	if stored.IsActive {
		t.Error("the account is still active, so nothing was revoked at all")
	}
	if stored.DeletedAt.Valid {
		t.Error("deleted_at was set. Nothing else in the platform sets it, the documented " +
			"behaviour is that an account is only ever deactivated, and setting it here would " +
			"release the account's email and subject for reuse and make its next login refused " +
			"as a deleted account")
	}
	if stored.Email != "del-deact@example.invalid" || stored.Subject != "del-deact" {
		t.Errorf("the account's identity was altered: subject=%q email=%q",
			stored.Subject, stored.Email)
	}
}

// Reversible is the property that makes "deactivate" the honest word.
func TestDelete_IsReversible(t *testing.T) {
	r, db := newDeletedAccountRepo(t)

	u := &database.User{
		Subject: "del-reactivate", Email: "del-reactivate@example.invalid",
		AuthMethod: "oauth", IsActive: true,
	}
	if err := db.Create(u).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := r.Delete(context.Background(), u.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	if err := db.Model(&database.User{}).Where("id = ?", u.ID).
		Update("is_active", true).Error; err != nil {
		t.Fatalf("reactivate: %v", err)
	}

	var stored database.User
	if err := db.First(&stored, u.ID).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if !stored.IsActive || stored.ID != u.ID {
		t.Errorf("reactivating did not restore the same account: id=%d active=%v",
			stored.ID, stored.IsActive)
	}
}

// Deactivating somebody who is not there should say so rather than report
// success over nothing.
func TestDelete_UnknownUserIsReported(t *testing.T) {
	r, _ := newDeletedAccountRepo(t)

	if err := r.Delete(context.Background(), 987654321); err == nil {
		t.Error("deactivating a user who does not exist reported success")
	}
}
