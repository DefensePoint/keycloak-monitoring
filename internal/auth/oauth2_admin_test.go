package auth

import (
	"context"
	"testing"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
)

// admin_users grants the admin role by email address. An address the identity
// provider has not verified proves nothing about who holds it: a realm with
// self-registration or self-service profile editing lets anyone type one in.

type adminRBACSpy struct {
	admins   map[uint]bool
	assigned []uint
}

func (s *adminRBACSpy) IsAdmin(_ context.Context, userID uint) (bool, error) {
	return s.admins[userID], nil
}

func (s *adminRBACSpy) GetRoleByName(_ context.Context, name string) (*Role, error) {
	return &Role{ID: 1, Name: name}, nil
}

func (s *adminRBACSpy) AssignRoleToUser(_ context.Context, userID, _ uint, _ *string, _ string, _ *time.Time) error {
	s.assigned = append(s.assigned, userID)
	return nil
}

func TestEnsureAdminRole(t *testing.T) {
	for _, tc := range []struct {
		name         string
		email        string
		verified     bool
		alreadyAdmin bool
		wantAssign   bool
	}{
		{name: "listed and verified is granted", email: "admin@example.com", verified: true, wantAssign: true},
		{name: "listed but unverified is not granted", email: "admin@example.com", verified: false},
		{name: "address match ignores case", email: "Admin@Example.COM", verified: true, wantAssign: true},
		{name: "not listed is not granted", email: "someone@example.com", verified: true},
		{name: "existing admin is left alone, verified or not", email: "admin@example.com", verified: false, alreadyAdmin: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rbacSpy := &adminRBACSpy{admins: map[uint]bool{7: tc.alreadyAdmin}}
			svc := NewOAuth2AdminService(rbacSpy, []string{"admin@example.com"}, logger.NewNoop())

			err := svc.EnsureAdminRole(context.Background(), &domain.User{ID: 7, Email: tc.email, EmailVerified: tc.verified})
			if err != nil {
				t.Fatalf("EnsureAdminRole: %v, want nil: the sign-in itself must still succeed", err)
			}
			if got := len(rbacSpy.assigned) == 1; got != tc.wantAssign {
				t.Errorf("admin role assigned = %v, want %v", got, tc.wantAssign)
			}
		})
	}
}
