package tenant

import (
	"context"
	"fmt"
	"sync"

	"repos.defensepoint.com/iam/oss-pmt/kmt/internal/domain"
)

// Service defines the interface for tenant operations.
type Service interface {
	// GetTenant retrieves a tenant by its tenant_id.
	GetTenant(ctx context.Context, tenantID string) (*domain.KeycloakTenant, error)

	// ListTenants returns all tenants.
	ListTenants(ctx context.Context) ([]*domain.KeycloakTenant, error)

	// ListEnabledTenants returns all enabled tenants.
	ListEnabledTenants(ctx context.Context) ([]*domain.KeycloakTenant, error)

	// CreateTenant creates a new tenant.
	CreateTenant(ctx context.Context, req *CreateRequest) (*domain.KeycloakTenant, error)

	// UpdateTenant updates an existing tenant.
	UpdateTenant(ctx context.Context, tenantID string, req *UpdateRequest) (*domain.KeycloakTenant, error)

	// DeleteTenant removes a tenant.
	DeleteTenant(ctx context.Context, tenantID string) error

	// UpdateHealth updates the health status of a tenant.
	UpdateHealth(ctx context.Context, tenantID, status, message string) error

	// LoadTenants loads all tenants into the cache.
	LoadTenants(ctx context.Context) error

	// UpdateError records an error for a tenant.
	UpdateError(ctx context.Context, tenantID, errorMsg string) error

	// GetHealth returns the health status of a tenant.
	GetHealth(ctx context.Context, tenantID string) (*HealthStatus, error)

	// RegisterCallback registers a callback for tenant changes.
	RegisterCallback(callback ChangeCallback)

	// GetDefault returns the default tenant.
	GetDefault(ctx context.Context) (*domain.KeycloakTenant, error)
}

// service implements the Service interface.
type service struct {
	repo      Repository
	cache     sync.Map // map[string]*domain.KeycloakTenant (tenant_id -> tenant)
	callbacks []ChangeCallback
	mu        sync.RWMutex
}

// NewService creates a new tenant service.
func NewService(repo Repository) Service {
	return &service{
		repo:      repo,
		callbacks: make([]ChangeCallback, 0),
	}
}

// LoadTenants loads all tenants into the cache.
func (s *service) LoadTenants(ctx context.Context) error {
	tenants, err := s.repo.List(ctx)
	if err != nil {
		return fmt.Errorf("failed to load tenants: %w", err)
	}

	for _, tenant := range tenants {
		s.cache.Store(tenant.TenantID, tenant)
	}

	return nil
}

// GetTenant retrieves a tenant by its tenant_id.
func (s *service) GetTenant(ctx context.Context, tenantID string) (*domain.KeycloakTenant, error) {
	// Try cache first
	if value, ok := s.cache.Load(tenantID); ok {
		return value.(*domain.KeycloakTenant), nil
	}

	// Fallback to repository
	tenant, err := s.repo.GetByTenantID(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	// Update cache
	s.cache.Store(tenantID, tenant)
	return tenant, nil
}

// ListTenants returns all tenants from cache.
func (s *service) ListTenants(ctx context.Context) ([]*domain.KeycloakTenant, error) {
	tenants := make([]*domain.KeycloakTenant, 0)
	s.cache.Range(func(key, value interface{}) bool {
		tenants = append(tenants, value.(*domain.KeycloakTenant))
		return true
	})
	return tenants, nil
}

// ListEnabledTenants returns all enabled tenants from cache.
func (s *service) ListEnabledTenants(ctx context.Context) ([]*domain.KeycloakTenant, error) {
	tenants := make([]*domain.KeycloakTenant, 0)
	s.cache.Range(func(key, value interface{}) bool {
		tenant := value.(*domain.KeycloakTenant)
		if tenant.Enabled {
			tenants = append(tenants, tenant)
		}
		return true
	})
	return tenants, nil
}

// CreateTenant creates a new tenant.
func (s *service) CreateTenant(ctx context.Context, req *CreateRequest) (*domain.KeycloakTenant, error) {
	// Validate required fields
	if err := s.validateCreateRequest(req); err != nil {
		return nil, err
	}

	// Check if tenant already exists
	if _, ok := s.cache.Load(req.TenantID); ok {
		return nil, fmt.Errorf("tenant %s already exists", req.TenantID)
	}

	// If setting as default, unset other defaults first
	if req.IsDefault {
		if err := s.repo.UnsetDefault(ctx); err != nil {
			return nil, fmt.Errorf("failed to unset other default tenants: %w", err)
		}
	}

	tenant := &domain.KeycloakTenant{
		TenantID:      req.TenantID,
		Name:          req.Name,
		Description:   req.Description,
		ServerURL:     req.ServerURL,
		AdminRealm:    req.AdminRealm,
		ClientID:      req.ClientID,
		ClientSecret:  req.ClientSecret,
		Configuration: req.Configuration,
		DefaultRealm:  req.DefaultRealm,
		Enabled:       req.Enabled,
		IsDefault:     req.IsDefault,
		Tags:          req.Tags,
		Owner:         req.Owner,
		HealthStatus:  "unknown",
	}

	if err := s.repo.Create(ctx, tenant); err != nil {
		return nil, fmt.Errorf("failed to create tenant: %w", err)
	}

	// Add to cache
	s.cache.Store(tenant.TenantID, tenant)

	// Notify callbacks
	s.notifyCallbacks(EventAdded, tenant)

	return tenant, nil
}

// UpdateTenant updates an existing tenant.
func (s *service) UpdateTenant(ctx context.Context, tenantID string, req *UpdateRequest) (*domain.KeycloakTenant, error) {
	tenant, err := s.GetTenant(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("tenant %s not found", tenantID)
	}

	wasEnabled := tenant.Enabled

	// Apply updates
	if req.Name != nil {
		tenant.Name = *req.Name
	}
	if req.Description != nil {
		tenant.Description = *req.Description
	}
	if req.ServerURL != nil {
		tenant.ServerURL = *req.ServerURL
	}
	if req.AdminRealm != nil {
		tenant.AdminRealm = *req.AdminRealm
	}
	if req.ClientID != nil {
		tenant.ClientID = *req.ClientID
	}
	// An empty string is never a valid secret rotation — the edit form never
	// displays the stored secret (write-only, matching Stripe/GCP/Auth0
	// practice), so it submits "" when the user leaves the field untouched.
	// Treat that as "no change" rather than wiping the stored secret.
	if req.ClientSecret != nil && *req.ClientSecret != "" {
		tenant.ClientSecret = *req.ClientSecret
	}
	if req.Configuration != nil {
		tenant.Configuration = *req.Configuration
	}
	if req.DefaultRealm != nil {
		tenant.DefaultRealm = *req.DefaultRealm
	}
	if req.Enabled != nil {
		tenant.Enabled = *req.Enabled
	}
	if req.IsDefault != nil {
		if *req.IsDefault {
			if err := s.repo.UnsetDefault(ctx); err != nil {
				return nil, fmt.Errorf("failed to unset other default tenants: %w", err)
			}
		}
		tenant.IsDefault = *req.IsDefault
	}
	if req.Tags != nil {
		tenant.Tags = req.Tags
	}
	if req.Owner != nil {
		tenant.Owner = *req.Owner
	}

	// a partial update must not leave the tenant without the client_credentials auth fields, or it would fail at
	// monitor start rather than here. Unset fields retain their existing value.
	if tenant.ClientID == "" {
		return nil, fmt.Errorf("client_id is required")
	}
	if tenant.ClientSecret == "" {
		return nil, fmt.Errorf("client_secret is required")
	}

	if err := s.repo.Update(ctx, tenant); err != nil {
		return nil, fmt.Errorf("failed to update tenant: %w", err)
	}

	// Update cache
	s.cache.Store(tenantID, tenant)

	// Determine event type
	event := EventUpdated
	if !wasEnabled && tenant.Enabled {
		event = EventEnabled
	} else if wasEnabled && !tenant.Enabled {
		event = EventDisabled
	}

	// Notify callbacks
	s.notifyCallbacks(event, tenant)

	return tenant, nil
}

// DeleteTenant removes a tenant.
func (s *service) DeleteTenant(ctx context.Context, tenantID string) error {
	tenant, err := s.GetTenant(ctx, tenantID)
	if err != nil {
		return fmt.Errorf("tenant %s not found", tenantID)
	}

	if err := s.repo.Delete(ctx, tenantID); err != nil {
		return fmt.Errorf("failed to delete tenant: %w", err)
	}

	// Remove from cache
	s.cache.Delete(tenantID)

	// Notify callbacks
	s.notifyCallbacks(EventRemoved, tenant)

	return nil
}

// UpdateHealth updates the health status of a tenant.
func (s *service) UpdateHealth(ctx context.Context, tenantID, status, message string) error {
	if err := s.repo.UpdateHealth(ctx, tenantID, status, message); err != nil {
		return err
	}

	// Update cache if tenant is cached
	if value, ok := s.cache.Load(tenantID); ok {
		tenant := value.(*domain.KeycloakTenant)
		tenant.HealthStatus = status
		tenant.HealthMessage = message
		s.cache.Store(tenantID, tenant)
	}

	return nil
}

// UpdateError records an error for a tenant.
func (s *service) UpdateError(ctx context.Context, tenantID, errorMsg string) error {
	if err := s.repo.UpdateError(ctx, tenantID, errorMsg); err != nil {
		return err
	}

	// Update cache if tenant is cached
	if value, ok := s.cache.Load(tenantID); ok {
		tenant := value.(*domain.KeycloakTenant)
		tenant.LastError = errorMsg
		s.cache.Store(tenantID, tenant)
	}

	return nil
}

// GetHealth returns the health status of a tenant.
func (s *service) GetHealth(ctx context.Context, tenantID string) (*HealthStatus, error) {
	tenant, err := s.GetTenant(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	var lastHealthCheck string
	if !tenant.LastHealthCheck.IsZero() {
		lastHealthCheck = tenant.LastHealthCheck.Format("2006-01-02T15:04:05Z07:00")
	}

	var lastError *string
	if tenant.LastError != "" {
		lastError = &tenant.LastError
	}

	var lastErrorAt *string
	if tenant.LastErrorAt != nil && !tenant.LastErrorAt.IsZero() {
		formatted := tenant.LastErrorAt.Format("2006-01-02T15:04:05Z07:00")
		lastErrorAt = &formatted
	}

	return &HealthStatus{
		TenantID:        tenant.TenantID,
		Status:          tenant.HealthStatus,
		Message:         tenant.HealthMessage,
		LastHealthCheck: lastHealthCheck,
		LastError:       lastError,
		LastErrorAt:     lastErrorAt,
	}, nil
}

// RegisterCallback registers a callback for tenant changes.
func (s *service) RegisterCallback(callback ChangeCallback) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.callbacks = append(s.callbacks, callback)
}

// GetDefault returns the default tenant.
func (s *service) GetDefault(ctx context.Context) (*domain.KeycloakTenant, error) {
	return s.repo.GetDefault(ctx)
}

// validateCreateRequest validates the create request.
func (s *service) validateCreateRequest(req *CreateRequest) error {
	if req.TenantID == "" {
		return fmt.Errorf("tenant_id is required")
	}
	if req.Name == "" {
		return fmt.Errorf("name is required")
	}
	if req.ServerURL == "" {
		return fmt.Errorf("server_url is required")
	}
	if req.AdminRealm == "" {
		return fmt.Errorf("admin_realm is required")
	}
	// Authentication uses the client_credentials grant; client_id + client_secret are required.
	if req.ClientID == "" {
		return fmt.Errorf("client_id is required")
	}
	if req.ClientSecret == "" {
		return fmt.Errorf("client_secret is required")
	}
	return nil
}

// notifyCallbacks notifies all registered callbacks.
func (s *service) notifyCallbacks(event Event, tenant *domain.KeycloakTenant) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, callback := range s.callbacks {
		go callback(event, tenant)
	}
}
