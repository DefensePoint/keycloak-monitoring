package fx

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	chihttp "github.com/DefensePoint/keycloak-monitoring/internal/http/chi"
	"github.com/DefensePoint/keycloak-monitoring/internal/users"
)

type subjectRepo struct {
	users.Repository
	user *domain.User
	err  error
}

func (r subjectRepo) GetBySubject(context.Context, string) (*domain.User, error) {
	return r.user, r.err
}

// The repository's "no such account" reaches the middleware as
// ErrUnknownAccount, which refuses the session; any other error stays one.
func TestUserLookupAdapter_TellsAMissingAccountFromAFailure(t *testing.T) {
	missing := &userLookupAdapter{repo: subjectRepo{err: fmt.Errorf("%w: bob", users.ErrUserNotFound)}}
	if _, err := missing.GetUserBySubject(context.Background(), "bob"); !errors.Is(err, chihttp.ErrUnknownAccount) {
		t.Errorf("missing account: err = %v, want ErrUnknownAccount", err)
	}
	failing := &userLookupAdapter{repo: subjectRepo{err: errors.New("connection refused")}}
	if _, err := failing.GetUserBySubject(context.Background(), "bob"); err == nil || errors.Is(err, chihttp.ErrUnknownAccount) {
		t.Errorf("failed lookup: err = %v, want the failure itself", err)
	}
	found := &userLookupAdapter{repo: subjectRepo{user: &domain.User{ID: 7, IsActive: true}}}
	if u, err := found.GetUserBySubject(context.Background(), "bob"); err != nil || u.ID != 7 || !u.IsActive {
		t.Errorf("found: %v, %v", u, err)
	}
}
