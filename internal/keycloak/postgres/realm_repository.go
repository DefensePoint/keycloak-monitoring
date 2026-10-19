package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/keycloak"
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
//
// enabled, login_with_email_allowed and is_healthy are written a second time,
// in the same transaction, because GORM substitutes a column's default for any
// zero-valued field carrying one. All three default to true, so a false value
// never reached the insert, and the conflict update then copied that true from
// EXCLUDED: a realm disabled in Keycloak was stored enabled and healthy, and
// every later sync reset it to enabled even if it had been corrected.
//
// A realm deleted and recreated in Keycloak under the same name comes back with
// a new id. The old row is retired (soft-deleted) first, in the same
// transaction: it describes a realm that no longer exists, and while it stayed
// live the (tenant_id, realm_name) index refused the new one, so every sync of
// that realm failed and KMT kept showing the old realm's state. The saved row
// is also made live again, for a realm that was retired and has come back.
func (r *RealmRepository) SaveRealm(ctx context.Context, realm *domain.KeycloakRealmInfo) error {
	dbRealm := convertDomainRealmToDB(realm)

	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("tenant_id = ? AND realm_name = ? AND realm_id <> ?",
			dbRealm.TenantID, dbRealm.RealmName, dbRealm.RealmID).
			Delete(&database.KeycloakRealmInfo{}).Error; err != nil {
			return fmt.Errorf("failed to retire the previous realm %q: %w", dbRealm.RealmName, err)
		}
		if err := upsertRealm(tx, dbRealm); err != nil {
			return err
		}
		// The values come from realm, not dbRealm: GORM's insert callback has
		// already overwritten dbRealm's zero-valued fields with the defaults.
		// Unscoped, because the conflict target also matches a soft-deleted
		// row, and this must reach the same row the upsert just wrote.
		res := tx.Unscoped().Model(&database.KeycloakRealmInfo{}).
			Where("tenant_id = ? AND realm_id = ?", dbRealm.TenantID, dbRealm.RealmID).
			UpdateColumns(map[string]any{
				"enabled":                  realm.Enabled,
				"login_with_email_allowed": realm.LoginWithEmailAllowed,
				"is_healthy":               realm.IsHealthy,
				"deleted_at":               nil,
				// Read in full: whatever made it unreadable has passed.
				"unreadable_since": nil,
			})
		if res.Error != nil {
			return fmt.Errorf("failed to set realm flags: %w", res.Error)
		}
		if res.RowsAffected != 1 {
			return fmt.Errorf("failed to set realm flags: %d rows affected, want 1", res.RowsAffected)
		}
		return nil
	})
}

// upsertRealm inserts the realm or refreshes an existing row. It cannot store
// false in a column whose default is true; SaveRealm writes those afterwards.
func upsertRealm(tx *gorm.DB, dbRealm *database.KeycloakRealmInfo) error {
	result := tx.Clauses(clause.OnConflict{
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

// MarkRealmsUnreadable implements keycloak.RealmWriter. It touches only live
// rows not marked yet, so the first time a realm was found unreadable stays.
func (r *RealmRepository) MarkRealmsUnreadable(ctx context.Context, tenantID string, realmNames []string, since time.Time) error {
	if len(realmNames) == 0 {
		return nil
	}
	if err := r.db.WithContext(ctx).Model(&database.KeycloakRealmInfo{}).
		Where("tenant_id = ? AND realm_name IN ? AND unreadable_since IS NULL", tenantID, realmNames).
		UpdateColumn("unreadable_since", since).Error; err != nil {
		return fmt.Errorf("failed to mark realms unreadable: %w", err)
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

// RetireRealmsNotIn soft-deletes the tenant's live realms whose Keycloak id is
// not in realmIDs, and returns how many it retired. The caller passes the ids
// of Keycloak's complete, current realm list; a realm missing from it has been
// deleted in Keycloak. An empty list retires nothing, so a caller that got no
// realms back cannot wipe the tenant's.
func (r *RealmRepository) RetireRealmsNotIn(ctx context.Context, tenantID string, realmIDs []string) (int64, error) {
	if len(realmIDs) == 0 {
		return 0, nil
	}
	result := r.db.WithContext(ctx).
		Where("tenant_id = ? AND realm_id NOT IN ?", tenantID, realmIDs).
		Delete(&database.KeycloakRealmInfo{})
	if result.Error != nil {
		return 0, fmt.Errorf("failed to retire deleted realms: %w", result.Error)
	}
	return result.RowsAffected, nil
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
		UnreadableSince:           r.UnreadableSince,
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
		UnreadableSince:           r.UnreadableSince,
		CreatedAt:                 r.CreatedAt,
		UpdatedAt:                 r.UpdatedAt,
	}
}

// Ensure RealmRepository implements keycloak.RealmRepository
var _ keycloak.RealmRepository = (*RealmRepository)(nil)
