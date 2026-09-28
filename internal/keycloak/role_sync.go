package keycloak

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/pkg/keycloakadmin"
)

// Default role names for role sync configuration.
// These match the role names defined in the rbac domain.
const (
	DefaultRoleAdmin    = "admin"
	DefaultRoleOperator = "operator"
	DefaultRoleViewer   = "viewer"
)

// RoleMapping defines how a Keycloak role maps to a platform role
type RoleMapping struct {
	KeycloakRole string
	PlatformRole string
}

// RoleSyncConfig configures role synchronization
type RoleSyncConfig struct {
	// RoleMappings defines how Keycloak roles map to platform roles
	RoleMappings []RoleMapping

	// SyncInterval defines how often to sync roles automatically (0 = manual only)
	SyncInterval time.Duration

	// SyncOnStartup determines if roles should be synced on service startup
	SyncOnStartup bool

	// CreateMissingUsers determines if users should be created if they don't exist
	CreateMissingUsers bool

	// DefaultRealmForSync is the default realm to sync from (if not specified per tenant)
	DefaultRealmForSync string
}

// RoleSyncService handles synchronization of roles from Keycloak to the platform
type RoleSyncService struct {
	client       AdminAPI
	userRepo     UserFinder
	userRoleRepo UserRoleManager
	roleRepo     RoleFinder
	config       *RoleSyncConfig
	logger       *logger.Logger

	// For periodic sync
	stopCh  chan struct{}
	wg      sync.WaitGroup
	mu      sync.RWMutex
	running bool
}

// NewRoleSyncService creates a new role synchronization service
func NewRoleSyncService(
	client AdminAPI,
	userRepo UserFinder,
	userRoleRepo UserRoleManager,
	roleRepo RoleFinder,
	config *RoleSyncConfig,
	log *logger.Logger,
) *RoleSyncService {
	if config == nil {
		config = &RoleSyncConfig{
			RoleMappings: []RoleMapping{
				{KeycloakRole: "admin", PlatformRole: DefaultRoleAdmin},
				{KeycloakRole: "operator", PlatformRole: DefaultRoleOperator},
				{KeycloakRole: "viewer", PlatformRole: DefaultRoleViewer},
			},
			SyncInterval:        1 * time.Hour,
			SyncOnStartup:       true,
			CreateMissingUsers:  true,
			DefaultRealmForSync: "master",
		}
	}

	return &RoleSyncService{
		client:       client,
		userRepo:     userRepo,
		userRoleRepo: userRoleRepo,
		roleRepo:     roleRepo,
		config:       config,
		logger:       log.WithComponent("role_sync"),
		stopCh:       make(chan struct{}),
	}
}

// Start begins automatic role synchronization if configured
func (s *RoleSyncService) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return fmt.Errorf("role sync service is already running")
	}

	s.logger.Info("Starting role synchronization service",
		logger.Bool("sync_on_startup", s.config.SyncOnStartup),
		logger.Str("sync_interval", s.config.SyncInterval.String()))

	// Sync on startup if configured
	if s.config.SyncOnStartup {
		s.logger.Info("Performing initial role synchronization")
		if err := s.SyncAllRealms(ctx, ""); err != nil {
			s.logger.Error("Failed to perform initial role sync", logger.Err(err))
			// Don't fail startup, just log the error
		}
	}

	// Start periodic sync if interval is configured
	if s.config.SyncInterval > 0 {
		s.running = true
		s.wg.Add(1)
		go s.periodicSync(ctx)
	}

	return nil
}

// Stop stops the automatic role synchronization
func (s *RoleSyncService) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return nil
	}

	s.logger.Info("Stopping role synchronization service")
	close(s.stopCh)
	s.wg.Wait()
	s.running = false

	return nil
}

// periodicSync runs the sync operation on a schedule
func (s *RoleSyncService) periodicSync(ctx context.Context) {
	defer s.wg.Done()

	ticker := time.NewTicker(s.config.SyncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			s.logger.Info("Periodic role sync stopped")
			return
		case <-ticker.C:
			s.logger.Info("Running periodic role synchronization")
			if err := s.SyncAllRealms(ctx, ""); err != nil {
				s.logger.Error("Periodic role sync failed", logger.Err(err))
			}
		}
	}
}

// SyncAllRealms syncs roles for all users in all configured realms
func (s *RoleSyncService) SyncAllRealms(ctx context.Context, tenantID string) error {
	realms := s.client.GetRealms()
	if len(realms) == 0 {
		realms = []string{s.config.DefaultRealmForSync}
	}

	s.logger.Info("Syncing roles for all realms",
		logger.Int("realm_count", len(realms)),
		logger.Str("tenant_id", tenantID))

	var errors []error
	for _, realm := range realms {
		if err := s.SyncRealm(ctx, realm, tenantID); err != nil {
			s.logger.Error("Failed to sync realm",
				logger.Str("realm", realm),
				logger.Err(err))
			errors = append(errors, fmt.Errorf("realm %s: %w", realm, err))
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("sync completed with %d errors", len(errors))
	}

	return nil
}

// SyncRealm syncs roles for all users in a specific realm
func (s *RoleSyncService) SyncRealm(ctx context.Context, realmName, tenantID string) error {
	s.logger.Info("Syncing roles for realm",
		logger.Str("realm", realmName),
		logger.Str("tenant_id", tenantID))

	// Fetch all users from Keycloak (paginated)
	const maxUsers = 1000
	users, err := s.client.GetRealmUsers(ctx, realmName, 0, maxUsers)
	if err != nil {
		return fmt.Errorf("failed to fetch users: %w", err)
	}

	s.logger.Info("Fetched users from Keycloak",
		logger.Int("user_count", len(users)),
		logger.Str("realm", realmName))

	syncCount := 0
	errorCount := 0

	for _, kcUser := range users {
		if err := s.SyncUser(ctx, realmName, &kcUser, tenantID); err != nil {
			s.logger.Warn("Failed to sync user",
				logger.Str("username", kcUser.Username),
				logger.Str("keycloak_id", kcUser.ID),
				logger.Err(err))
			errorCount++
		} else {
			syncCount++
		}
	}

	s.logger.Info("Realm sync completed",
		logger.Str("realm", realmName),
		logger.Int("synced", syncCount),
		logger.Int("errors", errorCount))

	return nil
}

// SyncUser syncs role assignments for a single user
func (s *RoleSyncService) SyncUser(ctx context.Context, realmName string, kcUser *keycloakadmin.UserRepresentation, tenantID string) error {
	// Find or create the platform user
	var platformUser *domain.User
	var err error

	// Try to find by subject (Keycloak ID)
	//
	// This is a user-identity subject, NOT an events-table source, despite
	// sharing the "keycloak:" prefix: it pairs the prefix with a user UUID
	// rather than a realm name, and it is matched against users.subject.
	// Do not route it through events.SourceForKeycloakRealm; doing so would
	// change stored subjects and break user lookup.
	subject := fmt.Sprintf("keycloak:%s", kcUser.ID)
	platformUser, err = s.userRepo.GetBySubject(ctx, subject)

	// If not found, try by email
	if err != nil && kcUser.Email != "" {
		platformUser, err = s.userRepo.GetByEmail(ctx, kcUser.Email)
	}

	// If still not found, try by username
	if err != nil && kcUser.Username != "" {
		platformUser, err = s.userRepo.GetByUsername(ctx, kcUser.Username)
	}

	// Create user if not found and creation is enabled
	if err != nil && s.config.CreateMissingUsers {
		s.logger.Info("Creating new platform user from Keycloak",
			logger.Str("username", kcUser.Username),
			logger.Str("email", kcUser.Email))

		newUser := &domain.User{
			Subject:           subject,
			Email:             kcUser.Email,
			EmailVerified:     kcUser.EmailVerified,
			Name:              fmt.Sprintf("%s %s", kcUser.FirstName, kcUser.LastName),
			GivenName:         kcUser.FirstName,
			FamilyName:        kcUser.LastName,
			PreferredUsername: kcUser.Username,
			Username:          kcUser.Username,
			IsActive:          kcUser.Enabled,
		}

		platformUser, err = s.userRepo.FindOrCreateBySubject(ctx, newUser)
		if err != nil {
			return fmt.Errorf("failed to create platform user: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("user not found and creation disabled: %w", err)
	}

	// Fetch role mappings from Keycloak
	roleMappings, err := s.client.GetUserRoleMappings(ctx, realmName, kcUser.ID)
	if err != nil {
		return fmt.Errorf("failed to fetch role mappings: %w", err)
	}

	// Map Keycloak roles to platform roles
	platformRoles := s.mapKeycloakRolesToPlatform(roleMappings)

	if len(platformRoles) == 0 {
		s.logger.Debug("No mapped platform roles for user",
			logger.Str("username", kcUser.Username))
		return nil
	}

	// Sync role assignments
	return s.syncUserRoles(ctx, platformUser.ID, platformRoles, tenantID)
}

// mapKeycloakRolesToPlatform converts Keycloak roles to platform roles using the configured mappings
func (s *RoleSyncService) mapKeycloakRolesToPlatform(roleMappings *keycloakadmin.RoleMappingsRepresentation) []string {
	platformRoles := make(map[string]bool)

	// Check realm roles
	for _, realmRole := range roleMappings.RealmMappings {
		for _, mapping := range s.config.RoleMappings {
			if mapping.KeycloakRole == realmRole.Name {
				platformRoles[mapping.PlatformRole] = true
			}
		}
	}

	// Check client roles
	for _, clientMapping := range roleMappings.ClientMappings {
		for _, clientRole := range clientMapping.Mappings {
			for _, mapping := range s.config.RoleMappings {
				if mapping.KeycloakRole == clientRole.Name {
					platformRoles[mapping.PlatformRole] = true
				}
			}
		}
	}

	// Convert map to slice
	result := make([]string, 0, len(platformRoles))
	for role := range platformRoles {
		result = append(result, role)
	}

	return result
}

// syncUserRoles synchronizes platform role assignments for a user
func (s *RoleSyncService) syncUserRoles(ctx context.Context, userID uint, platformRoleNames []string, tenantID string) error {
	// Get existing role assignments
	var existingRoles []*domain.UserRole
	var err error

	if tenantID != "" {
		existingRoles, err = s.userRoleRepo.GetUserRolesForTenant(ctx, userID, tenantID)
	} else {
		existingRoles, err = s.userRoleRepo.GetUserRoles(ctx, userID)
	}
	if err != nil {
		return fmt.Errorf("failed to get existing roles: %w", err)
	}

	// Build map of existing role names
	existingRoleMap := make(map[string]uint)
	for _, ur := range existingRoles {
		if ur.Role != nil {
			existingRoleMap[ur.Role.Name] = ur.RoleID
		}
	}

	// Determine which roles to add
	for _, roleName := range platformRoleNames {
		if _, exists := existingRoleMap[roleName]; !exists {
			// Need to add this role
			role, err := s.roleRepo.GetRoleByName(ctx, roleName)
			if err != nil {
				s.logger.Warn("Platform role not found",
					logger.Str("role", roleName),
					logger.Err(err))
				continue
			}

			var tenantIDPtr *string
			if tenantID != "" {
				tenantIDPtr = &tenantID
			}

			userRole := &domain.UserRole{
				UserID:     userID,
				RoleID:     role.ID,
				TenantID:   tenantIDPtr,
				AssignedBy: "keycloak_sync",
				AssignedAt: time.Now(),
			}

			if err := s.userRoleRepo.AssignRoleToUser(ctx, userRole); err != nil {
				s.logger.Error("Failed to assign role",
					logger.Uint("user_id", userID),
					logger.Str("role", roleName),
					logger.Err(err))
			} else {
				s.logger.Info("Assigned role to user",
					logger.Uint("user_id", userID),
					logger.Str("role", roleName),
					logger.Str("tenant_id", tenantID))
			}
		}
	}

	// Build map of desired roles
	desiredRoleMap := make(map[string]bool)
	for _, roleName := range platformRoleNames {
		desiredRoleMap[roleName] = true
	}

	// Remove roles that are no longer assigned in Keycloak
	for roleName, roleID := range existingRoleMap {
		if !desiredRoleMap[roleName] {
			var tenantIDPtr *string
			if tenantID != "" {
				tenantIDPtr = &tenantID
			}

			if err := s.userRoleRepo.RemoveRoleFromUser(ctx, userID, roleID, tenantIDPtr); err != nil {
				s.logger.Error("Failed to remove role",
					logger.Uint("user_id", userID),
					logger.Str("role", roleName),
					logger.Err(err))
			} else {
				s.logger.Info("Removed role from user",
					logger.Uint("user_id", userID),
					logger.Str("role", roleName),
					logger.Str("tenant_id", tenantID))
			}
		}
	}

	return nil
}
