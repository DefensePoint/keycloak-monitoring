package auth

import (
	"context"
	"testing"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
)

// usernameRepo answers GetByUsername with its user.
type usernameRepo struct {
	*callbackUserRepoStub
	user *domain.User
}

func (r usernameRepo) GetByUsername(context.Context, string) (*domain.User, error) {
	return r.user, nil
}

// The epoch comes from the row whose password was checked, so a password
// changed after the check leaves the session older than the account.
func TestAuthenticate_ReturnsTheSessionEpochReadWithThePassword(t *testing.T) {
	hash, err := NewBcryptPasswordHasher(4).Hash("correct-horse-1A!")
	if err != nil {
		t.Fatal(err)
	}
	repo := usernameRepo{callbackUserRepoStub: &callbackUserRepoStub{}, user: &domain.User{
		ID: 1, Username: "admin", Subject: "admin@example.com", Email: "admin@example.com",
		PasswordHash: hash, AuthMethod: domain.AuthMethodSimple, IsActive: true, SessionEpoch: 5,
	}}
	svc := NewSimpleAuthService(repo, nil, logger.NewNoop())
	info, err := svc.Authenticate(context.Background(), "admin", "correct-horse-1A!")
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if info.SessionEpoch == nil || *info.SessionEpoch != 5 {
		t.Errorf("SessionEpoch = %v, want 5 from the user's row", info.SessionEpoch)
	}
}
