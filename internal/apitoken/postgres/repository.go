// Package postgres provides the PostgreSQL implementation of the apitoken repository.
package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/DefensePoint/keycloak-monitoring/internal/apitoken"
	"github.com/DefensePoint/keycloak-monitoring/pkg/database"
)

// Repository implements apitoken.Repository using PostgreSQL via GORM.
type Repository struct {
	db *gorm.DB
}

// NewRepository creates a new PostgreSQL apitoken repository.
func NewRepository(db *gorm.DB) apitoken.Repository {
	return &Repository{db: db}
}

// Create persists a new token with its digest and fills in ID and CreatedAt.
func (r *Repository) Create(ctx context.Context, token *apitoken.Token, digest string) error {
	tenantsJSON, _ := json.Marshal(token.TenantIDs)
	dbToken := &database.APIToken{
		UserID:      token.UserID,
		Name:        token.Name,
		TokenDigest: digest,
		TenantIDs:   tenantsJSON,
		ExpiresAt:   token.ExpiresAt,
	}
	if err := r.db.WithContext(ctx).Create(dbToken).Error; err != nil {
		return fmt.Errorf("failed to create api token: %w", err)
	}
	token.ID = dbToken.ID
	token.CreatedAt = dbToken.CreatedAt
	return nil
}

// List retrieves all tokens, newest first.
func (r *Repository) List(ctx context.Context) ([]*apitoken.Token, error) {
	var dbTokens []database.APIToken
	if err := r.db.WithContext(ctx).Order("created_at DESC").Find(&dbTokens).Error; err != nil {
		return nil, fmt.Errorf("failed to list api tokens: %w", err)
	}
	return convertTokenList(dbTokens)
}

// GetByDigest retrieves a token by its SHA-256 hex digest.
func (r *Repository) GetByDigest(ctx context.Context, digest string) (*apitoken.Token, error) {
	var dbToken database.APIToken
	if err := r.db.WithContext(ctx).Where("token_digest = ?", digest).First(&dbToken).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get api token: %w", err)
	}
	return convertToken(&dbToken)
}

// Revoke marks a token as revoked at the given time. Revoking an already
// revoked token is a no-op so the original revocation time is preserved.
func (r *Repository) Revoke(ctx context.Context, id uint, revokedAt time.Time) error {
	result := r.db.WithContext(ctx).Model(&database.APIToken{}).
		Where("id = ? AND revoked_at IS NULL", id).
		Update("revoked_at", revokedAt)
	if result.Error != nil {
		return fmt.Errorf("failed to revoke api token: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		var count int64
		if err := r.db.WithContext(ctx).Model(&database.APIToken{}).Where("id = ?", id).Count(&count).Error; err != nil {
			return fmt.Errorf("failed to revoke api token: %w", err)
		}
		if count == 0 {
			return apitoken.ErrTokenNotFound
		}
	}
	return nil
}

func convertToken(db *database.APIToken) (*apitoken.Token, error) {
	var tenantIDs []string
	if len(db.TenantIDs) > 0 {
		if err := json.Unmarshal(db.TenantIDs, &tenantIDs); err != nil {
			return nil, fmt.Errorf("failed to unmarshal tenant ids for api token %d: %w", db.ID, err)
		}
	}
	return &apitoken.Token{
		ID:        db.ID,
		UserID:    db.UserID,
		Name:      db.Name,
		TenantIDs: tenantIDs,
		ExpiresAt: db.ExpiresAt,
		RevokedAt: db.RevokedAt,
		CreatedAt: db.CreatedAt,
	}, nil
}

func convertTokenList(dbTokens []database.APIToken) ([]*apitoken.Token, error) {
	result := make([]*apitoken.Token, len(dbTokens))
	for i := range dbTokens {
		token, err := convertToken(&dbTokens[i])
		if err != nil {
			return nil, err
		}
		result[i] = token
	}
	return result, nil
}
