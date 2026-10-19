package keycloak

import (
	"context"
	"errors"
	"testing"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/pkg/keycloakadmin"
)

// syncTestUsers finds nobody, so SyncUser creates the user, and records the
// profile it creates.
type syncTestUsers struct{ created *domain.User }

func (u *syncTestUsers) GetBySubject(context.Context, string) (*domain.User, error) {
	return nil, errors.New("not found")
}
func (u *syncTestUsers) GetByEmail(context.Context, string) (*domain.User, error) {
	return nil, errors.New("not found")
}
func (u *syncTestUsers) GetByUsername(context.Context, string) (*domain.User, error) {
	return nil, errors.New("not found")
}
func (u *syncTestUsers) FindOrCreateBySubject(_ context.Context, user *domain.User) (*domain.User, error) {
	u.created = user
	created := *user
	created.ID = 1
	return &created, nil
}

type syncTestKeycloak struct{ AdminAPI }

func (syncTestKeycloak) GetUserRoleMappings(context.Context, string, string) (*keycloakadmin.RoleMappingsRepresentation, error) {
	return &keycloakadmin.RoleMappingsRepresentation{}, nil
}

// A user created from Keycloak signs in through Keycloak. Left unset, the
// column default stores them as a local "simple" account, whose identity
// fields and password an admin can then edit.
func TestRoleSync_CreatesKeycloakUsersAsOAuth(t *testing.T) {
	users := &syncTestUsers{}
	s := NewRoleSyncService(syncTestKeycloak{}, users, nil, nil, nil, logger.NewNoop())

	if err := s.SyncUser(context.Background(), "master", &keycloakadmin.UserRepresentation{
		ID: "kc-1", Username: "alice", Email: "alice@example.test", Enabled: true,
	}, ""); err != nil {
		t.Fatalf("SyncUser: %v", err)
	}

	if users.created == nil {
		t.Fatal("SyncUser created no user")
	}
	if users.created.AuthMethod != domain.AuthMethodOAuth {
		t.Errorf("AuthMethod = %q, want %q", users.created.AuthMethod, domain.AuthMethodOAuth)
	}
}
