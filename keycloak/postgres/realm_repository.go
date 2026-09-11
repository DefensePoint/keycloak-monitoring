package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/keycloak"
	"github.com/DefensePoint/keycloak-monitoring/pkg/database"
)

// RealmRepository implements keycloak.RealmRepository using GORM.
type RealmRepository struct {
	db *gorm.DB
}

// NewRealmRepository creates a new RealmRepository.
func NewRealmRepository(db *gorm.DB) *RealmRepository {
	return &RealmRepository{db: db}
}

// GetRealms returns all realms for a tenant.
func (r *RealmRepository) GetRealms(ctx context.Context, tenantID string) ([]*domain.KeycloakRealmInfo, error) {
	var dbRealms []*database.KeycloakRealmInfo

	result := r.db.WithContext(ctx).
		Where("tenant_id = ?", tenantID).
		Order("realm_name ASC").
		Find(&dbRealms)

	if result.Error != nil {
		return nil, fmt.Errorf("failed to get realms: %w", result.Error)
	}

	return convertDBRealmsToDomain(dbRealms), nil
}

// GetRealmByName returns a specific realm by name.
func (r *RealmRepository) GetRealmByName(ctx context.Context, tenantID, realmName string) (*domain.KeycloakRealmInfo, error) {
	var dbRealm database.KeycloakRealmInfo

	result := r.db.WithContext(ctx).
		Where("tenant_id = ? AND realm_name = ?", tenantID, realmName).
		First(&dbRealm)

	if result.Error == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if result.Error != nil {
		return nil, fmt.Errorf("failed to get realm by name: %w", result.Error)
	}

	return convertDBRealmToDomain(&dbRealm), nil
}

// SaveRealm saves a single realm.
func (r *RealmRepository) SaveRealm(ctx context.Context, realm *domain.KeycloakRealmInfo) error {
	dbRealm := convertDomainRealmToDB(realm)

	result := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "tenant_id"}, {Name: "realm_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"realm_name", "display_name", "enabled", "ssl_required",
			"registration_allowed", "remember_me", "verify_email",
			"login_with_email_allowed", "duplicate_emails_allowed",
			"reset_password_allowed", "edit_username_allowed",
			"brute_force_protected",
			"events_enabled", "events_listeners", "enabled_event_types",
			"admin_events_enabled", "admin_events_details_enabled",
			"last_checked", "is_healthy", "health_message",
		}),
	}).Create(dbRealm)

	if result.Error != nil {
		return fmt.Errorf("failed to save realm: %w", result.Error)
	}

	return nil
}

// SaveRealms saves multiple realms.
func (r *RealmRepository) SaveRealms(ctx context.Context, realms []*domain.KeycloakRealmInfo) error {
	if len(realms) == 0 {
		return nil
	}

	for _, realm := range realms {
		if err := r.SaveRealm(ctx, realm); err != nil {
			return err
		}
	}
	return nil
}

// DeleteRealm deletes a realm.
func (r *RealmRepository) DeleteRealm(ctx context.Context, tenantID, realmName string) error {
	result := r.db.WithContext(ctx).
		Where("tenant_id = ? AND realm_name = ?", tenantID, realmName).
		Delete(&database.KeycloakRealmInfo{})

	if result.Error != nil {
		return fmt.Errorf("failed to delete realm: %w", result.Error)
	}

	return nil
}

// convertDBRealmToDomain converts a database realm to domain realm.
func convertDBRealmToDomain(r *database.KeycloakRealmInfo) *domain.KeycloakRealmInfo {
	// Unmarshal event listeners and types from JSON strings
	var listeners, types []string
	if r.EventsListeners != "" {
		_ = json.Unmarshal([]byte(r.EventsListeners), &listeners)
	}
	if r.EnabledEventTypes != "" {
		_ = json.Unmarshal([]byte(r.EnabledEventTypes), &types)
	}

	return &domain.KeycloakRealmInfo{
		ID:                        r.ID,
		TenantID:                  r.TenantID,
		RealmID:                   r.RealmID,
		RealmName:                 r.RealmName,
		DisplayName:               r.DisplayName,
		Enabled:                   r.Enabled,
		SslRequired:               r.SslRequired,
		RegistrationAllowed:       r.RegistrationAllowed,
		RememberMe:                r.RememberMe,
		VerifyEmail:               r.VerifyEmail,
		LoginWithEmailAllowed:     r.LoginWithEmailAllowed,
		DuplicateEmailsAllowed:    r.DuplicateEmailsAllowed,
		ResetPasswordAllowed:      r.ResetPasswordAllowed,
		EditUsernameAllowed:       r.EditUsernameAllowed,
		BruteForceProtected:       r.BruteForceProtected,
		EventsEnabled:             r.EventsEnabled,
		EventsListeners:           listeners,
		EnabledEventTypes:         types,
		AdminEventsEnabled:        r.AdminEventsEnabled,
		AdminEventsDetailsEnabled: r.AdminEventsDetailsEnabled,
		LastChecked:               r.LastChecked,
		IsHealthy:                 r.IsHealthy,
		HealthMessage:             r.HealthMessage,
		CreatedAt:                 r.CreatedAt,
		UpdatedAt:                 r.UpdatedAt,
	}
}

// convertDBRealmsToDomain converts a list of database realms.
func convertDBRealmsToDomain(dbRealms []*database.KeycloakRealmInfo) []*domain.KeycloakRealmInfo {
	result := make([]*domain.KeycloakRealmInfo, len(dbRealms))
	for i, r := range dbRealms {
		result[i] = convertDBRealmToDomain(r)
	}
	return result
}

// convertDomainRealmToDB converts domain realm to database realm.
func convertDomainRealmToDB(r *domain.KeycloakRealmInfo) *database.KeycloakRealmInfo {
	// Marshal event listeners and types to JSON strings
	var listenersJSON, typesJSON string
	if len(r.EventsListeners) > 0 {
		if b, err := json.Marshal(r.EventsListeners); err == nil {
			listenersJSON = string(b)
		}
	}
	if len(r.EnabledEventTypes) > 0 {
		if b, err := json.Marshal(r.EnabledEventTypes); err == nil {
			typesJSON = string(b)
		}
	}

	return &database.KeycloakRealmInfo{
		ID:                        r.ID,
		TenantID:                  r.TenantID,
		RealmID:                   r.RealmID,
		RealmName:                 r.RealmName,
		DisplayName:               r.DisplayName,
		Enabled:                   r.Enabled,
		SslRequired:               r.SslRequired,
		RegistrationAllowed:       r.RegistrationAllowed,
		RememberMe:                r.RememberMe,
		VerifyEmail:               r.VerifyEmail,
		LoginWithEmailAllowed:     r.LoginWithEmailAllowed,
		DuplicateEmailsAllowed:    r.DuplicateEmailsAllowed,
		ResetPasswordAllowed:      r.ResetPasswordAllowed,
		EditUsernameAllowed:       r.EditUsernameAllowed,
		BruteForceProtected:       r.BruteForceProtected,
		EventsEnabled:             r.EventsEnabled,
		EventsListeners:           listenersJSON,
		EnabledEventTypes:         typesJSON,
		AdminEventsEnabled:        r.AdminEventsEnabled,
		AdminEventsDetailsEnabled: r.AdminEventsDetailsEnabled,
		LastChecked:               r.LastChecked,
		IsHealthy:                 r.IsHealthy,
		HealthMessage:             r.HealthMessage,
		CreatedAt:                 r.CreatedAt,
		UpdatedAt:                 r.UpdatedAt,
	}
}

// Ensure RealmRepository implements keycloak.RealmRepository
var _ keycloak.RealmRepository = (*RealmRepository)(nil)
