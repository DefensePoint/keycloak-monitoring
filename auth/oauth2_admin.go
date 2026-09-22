package auth

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
)

// OAuth2AdminRBACService defines the interface for RBAC operations needed by OAuth2AdminService.
// This avoids import cycles by not directly importing the rbac package.
type OAuth2AdminRBACService interface {
	IsAdmin(ctx context.Context, userID uint) (bool, error)
	GetRoleByName(ctx context.Context, name string) (*Role, error)
	AssignRoleToUser(ctx context.Context, userID, roleID uint, tenantID *string, assignedBy string, expiresAt *time.Time) error
}

// Role represents a minimal role structure for OAuth2AdminService.
type Role struct {
	ID   uint
	Name string
}

// OAuth2AdminService handles automatic admin role assignment for OAuth2 users.
// Users whose email matches the configured admin_users list will be automatically
// assigned the admin role on their first login.
type OAuth2AdminService struct {
	rbacService OAuth2AdminRBACService
	adminEmails []string
	logger      *logger.Logger
}

// NewOAuth2AdminService creates a new OAuth2 admin service.
func NewOAuth2AdminService(
	rbacService OAuth2AdminRBACService,
	adminEmails []string,
	log *logger.Logger,
) *OAuth2AdminService {
	// Normalize emails to lowercase for case-insensitive comparison
	normalized := make([]string, 0, len(adminEmails))
	for _, email := range adminEmails {
		email = strings.ToLower(strings.TrimSpace(email))
		if email != "" {
			normalized = append(normalized, email)
		}
	}

	return &OAuth2AdminService{
		rbacService: rbacService,
		adminEmails: normalized,
		logger:      log.WithComponent("oauth2_admin"),
	}
}

// ShouldBeAdmin checks if a user email is in the admin users list.
func (s *OAuth2AdminService) ShouldBeAdmin(email string) bool {
	if len(s.adminEmails) == 0 {
		return false
	}

	normalizedEmail := strings.ToLower(strings.TrimSpace(email))
	for _, adminEmail := range s.adminEmails {
		if adminEmail == normalizedEmail {
			return true
		}
	}
	return false
}

// EnsureAdminRole ensures that a user has the admin role if they should be an admin.
// This is called after OAuth2 authentication to automatically assign admin privileges
// to designated users on their first login.
func (s *OAuth2AdminService) EnsureAdminRole(ctx context.Context, user *domain.User) error {
	// Check if user should be admin
	if !s.ShouldBeAdmin(user.Email) {
		s.logger.Debug("User is not in admin users list",
			logger.Str("email", user.Email))
		return nil
	}

	// Check if user already has admin role
	isAdmin, err := s.rbacService.IsAdmin(ctx, user.ID)
	if err != nil {
		s.logger.Error("Failed to check if user is admin",
			logger.Uint("user_id", user.ID),
			logger.Str("email", user.Email),
			logger.Err(err))
		return err
	}

	if isAdmin {
		s.logger.Debug("User already has admin role",
			logger.Uint("user_id", user.ID),
			logger.Str("email", user.Email))
		return nil
	}

	// Get admin role
	role, err := s.rbacService.GetRoleByName(ctx, "admin")
	if err != nil {
		s.logger.Error("Failed to get admin role",
			logger.Err(err))
		return err
	}

	if role == nil {
		s.logger.Error("Admin role not found")
		return fmt.Errorf("admin role not found")
	}

	// Assign admin role
	s.logger.Info("Assigning admin role to OAuth2 user (configured as initial admin)",
		logger.Uint("user_id", user.ID),
		logger.Str("email", user.Email))

	err = s.rbacService.AssignRoleToUser(ctx, user.ID, role.ID, nil, "system", nil)
	if err != nil {
		s.logger.Error("Failed to assign admin role to OAuth2 user",
			logger.Uint("user_id", user.ID),
			logger.Str("email", user.Email),
			logger.Err(err))
		return err
	}

	s.logger.Warn("SECURITY NOTICE: Admin role assigned to OAuth2 user",
		logger.Uint("user_id", user.ID),
		logger.Str("email", user.Email),
		logger.Str("message", "User configured as initial admin via oauth2.admin_users"))

	return nil
}

// GetAdminEmails returns the list of configured admin emails.
func (s *OAuth2AdminService) GetAdminEmails() []string {
	return s.adminEmails
}
