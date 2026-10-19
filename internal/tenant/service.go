package tenant

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

// ErrConfigDefinedTenantReadOnly is returned when the public API is used to
// update or delete a tenant that is managed via config.yaml. Such tenants
// can only be changed by editing config.yaml and restarting the app.
var ErrConfigDefinedTenantReadOnly = errors.New("tenant is config-defined and read-only; edit config.yaml and restart to change it")

// ErrClientSecretRequired is returned when an update points the tenant at a
// different server without supplying the client secret. The stored secret is
// sent to server_url to get tokens, and those tokens are sent to the AMFA API
// URL, so keeping it would hand the secret, or a token made with it, to
// whoever runs the new server.
var ErrClientSecretRequired = errors.New("changing the Keycloak server URL or the AMFA API URL to a different server requires entering the client secret again")

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

	// SyncFromConfig creates or updates a tenant from config.yaml and marks it
	// config-defined. It skips the read-only guard on purpose: config.yaml
	// always wins on restart. IsConfigDefined has no field in CreateRequest
	// or UpdateRequest, so the public API can never set it.
	SyncFromConfig(ctx context.Context, tenantID string, createReq *CreateRequest, updateReq *UpdateRequest) (*domain.KeycloakTenant, error)

	// ListConfigDefinedTenants returns every tenant currently marked
	// IsConfigDefined, straight from the repository (not the cache, which may
	// not be populated yet during bootstrap).
	ListConfigDefinedTenants(ctx context.Context) ([]*domain.KeycloakTenant, error)

	// UnmarkConfigDefined clears IsConfigDefined on a tenant no longer present
	// in config.yaml, restoring normal edit/delete access.
	UnmarkConfigDefined(ctx context.Context, tenantID string) error

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
	repo Repository
	// cache entries are replaced, never changed in place: the API handlers,
	// the monitors and the change callbacks all keep the pointer they were
	// handed, so writing a field would show them a change the database has
	// not accepted yet, or may reject outright.
	cache sync.Map // map[string]*domain.KeycloakTenant (tenant_id -> tenant)
	// cacheMu orders the writers that load an entry, copy it and store the
	// copy; without it one of them stores a copy older than another's.
	cacheMu sync.Mutex
	// updateMu orders the writes of a tenant's configuration (create, update,
	// sync from config, unmark, delete) from the read they start from to
	// the store they end with, so each reads what the last one stored and the
	// cache takes them in the order the database did. It is held across the
	// database call. Health and error writes do not take it: they own other
	// columns, and an edit must not hold up a health check.
	updateMu  sync.Mutex
	callbacks []*registeredCallback
	mu        sync.RWMutex

	deletionHook DeletionHook
}

// DeletionHook takes part in deleting a tenant. Deleting purges every row the
// tenant owns, its alerts included, so whatever must be read from those rows
// has to be read first.
type DeletionHook interface {
	// PrepareDeletion runs before the tenant's rows are purged. It returns
	// what to run once the deletion has committed, or nil; that is not run
	// when the deletion fails.
	PrepareDeletion(ctx context.Context, tenant *domain.KeycloakTenant) func(context.Context)
}

// ServiceOption configures optional Service behavior.
type ServiceOption func(*service)

// WithDeletionHook wires a hook that takes part in deleting a tenant.
func WithDeletionHook(h DeletionHook) ServiceOption {
	return func(s *service) { s.deletionHook = h }
}

// callbackQueueSize bounds how far a callback can fall behind the tenant
// changes that triggered it before a caller (Create/UpdateTenant) starts
// blocking on notifyCallbacks. Generous for what is normally fast,
// synchronous work (registering/unregistering an in-memory repository).
const callbackQueueSize = 64

// callbackJob is one tenant-change event queued for a single callback.
type callbackJob struct {
	event  Event
	tenant *domain.KeycloakTenant
}

// registeredCallback runs one callback's events through a single worker
// goroutine, so consecutive changes to the same tenant (e.g. enable then
// disable in quick succession) are always delivered in the order they
// happened. Firing `go callback(...)` per event independently, as this used
// to do, gave the Go scheduler no ordering guarantee between two such
// goroutines, so a later event could be applied before an earlier one.
type registeredCallback struct {
	fn    ChangeCallback
	queue chan callbackJob
}

func newRegisteredCallback(fn ChangeCallback) *registeredCallback {
	rc := &registeredCallback{fn: fn, queue: make(chan callbackJob, callbackQueueSize)}
	go rc.run()
	return rc
}

func (rc *registeredCallback) run() {
	for job := range rc.queue {
		rc.fn(job.event, job.tenant)
	}
}

// NewService creates a new tenant service.
func NewService(repo Repository, opts ...ServiceOption) Service {
	s := &service{
		repo:      repo,
		callbacks: make([]*registeredCallback, 0),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// LoadTenants loads all tenants into the cache.
func (s *service) LoadTenants(ctx context.Context) error {
	tenants, err := s.repo.List(ctx)
	if err != nil {
		return fmt.Errorf("failed to load tenants: %w", err)
	}

	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
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

	// Update cache, unless a writer stored the tenant while it was read: that
	// entry is the newer one.
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	if value, ok := s.cache.Load(tenantID); ok {
		return value.(*domain.KeycloakTenant), nil
	}
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
	s.updateMu.Lock()
	defer s.updateMu.Unlock()
	return s.applyCreate(ctx, req, false)
}

// applyCreate creates a tenant from req. Shared by CreateTenant (public,
// always isConfigDefined=false) and SyncFromConfig (isConfigDefined=true).
func (s *service) applyCreate(ctx context.Context, req *CreateRequest, isConfigDefined bool) (*domain.KeycloakTenant, error) {
	// Validate required fields
	if err := s.validateCreateRequest(req); err != nil {
		return nil, err
	}

	// Check if tenant already exists
	if _, ok := s.cache.Load(req.TenantID); ok {
		return nil, fmt.Errorf("tenant %s already exists", req.TenantID)
	}

	tenant := &domain.KeycloakTenant{
		TenantID:        req.TenantID,
		Name:            req.Name,
		Description:     req.Description,
		ServerURL:       req.ServerURL,
		AdminRealm:      req.AdminRealm,
		ClientID:        req.ClientID,
		ClientSecret:    req.ClientSecret,
		Configuration:   req.Configuration,
		DefaultRealm:    req.DefaultRealm,
		Enabled:         req.Enabled,
		IsDefault:       req.IsDefault,
		IsConfigDefined: isConfigDefined,
		Tags:            req.Tags,
		Owner:           req.Owner,
		Amfa:            req.Amfa.toDomain(),
		HealthStatus:    "unknown",
	}

	if err := s.repo.Create(ctx, tenant); err != nil {
		return nil, fmt.Errorf("failed to create tenant: %w", err)
	}

	// Add to cache
	s.storeTenant(tenant)

	// Notify callbacks
	s.notifyCallbacks(EventAdded, tenant)

	return tenant, nil
}

// SyncFromConfig creates the tenant if tenantID is new, else updates it.
// Refuses to take over a tenant_id that currently belongs to a UI-managed
// tenant (IsConfigDefined false), since silently overwriting and permanently
// locking someone's tenant on an incidental ID collision would be a bigger
// surprise than skipping this one config entry.
//
// This also applies to a tenant that WAS config-defined before: once
// unlockOrphanedConfigDefinedTenants (bootstrap.go) clears the flag because
// the tenant left config.yaml, re-adding the same tenant_id to config.yaml
// later does not automatically re-lock it — it looks identical to a fresh ID
// collision from here, and there's no signal distinguishing "born UI" from
// "previously config, now orphaned". Recovering config-management for such a
// tenant requires deleting the row and letting config.yaml recreate it.
func (s *service) SyncFromConfig(ctx context.Context, tenantID string, createReq *CreateRequest, updateReq *UpdateRequest) (*domain.KeycloakTenant, error) {
	s.updateMu.Lock()
	defer s.updateMu.Unlock()
	existing, err := s.GetTenant(ctx, tenantID)
	if err == nil && existing != nil {
		if !existing.IsConfigDefined {
			return nil, fmt.Errorf("tenant_id %q already exists as a UI-managed tenant; rename it in config.yaml or delete the existing tenant first", tenantID)
		}
		return s.applyUpdate(ctx, existing, updateReq, true)
	}

	return s.applyCreate(ctx, createReq, true)
}

// ListConfigDefinedTenants returns every tenant currently marked
// IsConfigDefined, read straight from the repository.
func (s *service) ListConfigDefinedTenants(ctx context.Context) ([]*domain.KeycloakTenant, error) {
	all, err := s.repo.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list tenants: %w", err)
	}

	configDefined := make([]*domain.KeycloakTenant, 0, len(all))
	for _, t := range all {
		if t.IsConfigDefined {
			configDefined = append(configDefined, t)
		}
	}
	return configDefined, nil
}

// UnmarkConfigDefined clears IsConfigDefined on a tenant that is no longer
// present in config.yaml, restoring normal edit/delete access through the
// public API. It bypasses the read-only guard for the same reason
// SyncFromConfig does: this is the config-sync path unlocking its own prior
// lock, not a public mutation.
func (s *service) UnmarkConfigDefined(ctx context.Context, tenantID string) error {
	s.updateMu.Lock()
	defer s.updateMu.Unlock()
	existing, err := s.GetTenant(ctx, tenantID)
	if err != nil {
		return fmt.Errorf("tenant %s not found", tenantID)
	}
	if !existing.IsConfigDefined {
		return nil
	}

	unmarked := cloneTenant(existing)
	unmarked.IsConfigDefined = false
	if err := s.repo.Update(ctx, unmarked); err != nil {
		return fmt.Errorf("failed to unmark tenant %s as config-defined: %w", tenantID, err)
	}
	s.storeTenant(unmarked)
	return nil
}

// UpdateTenant updates an existing tenant.
func (s *service) UpdateTenant(ctx context.Context, tenantID string, req *UpdateRequest) (*domain.KeycloakTenant, error) {
	s.updateMu.Lock()
	defer s.updateMu.Unlock()
	tenant, err := s.GetTenant(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("tenant %s not found", tenantID)
	}

	if tenant.IsConfigDefined {
		return nil, ErrConfigDefinedTenantReadOnly
	}

	secretGiven := req.ClientSecret != nil && *req.ClientSecret != ""
	if !secretGiven && credentialDestinationChanges(tenant, req) {
		return nil, ErrClientSecretRequired
	}

	return s.applyUpdate(ctx, tenant, req, false)
}

// credentialDestinationChanges reports whether req sends the tenant's
// credentials to a server they have not gone to before: server_url, which
// receives the client secret, or the AMFA API URL, which receives tokens
// issued with it. A path change on the same scheme, host and port is not a
// new destination. Setting an AMFA API URL where there was none is.
func credentialDestinationChanges(t *domain.KeycloakTenant, req *UpdateRequest) bool {
	if req.ServerURL != nil && !sameOrigin(t.ServerURL, *req.ServerURL) {
		return true
	}
	if req.Amfa != nil && req.Amfa.APIBaseURL != "" {
		previous := ""
		if t.Amfa != nil {
			previous = t.Amfa.APIBaseURL
		}
		if !sameOrigin(previous, req.Amfa.APIBaseURL) {
			return true
		}
	}
	return false
}

// sameOrigin reports whether two URLs share scheme, host and port. A URL that
// does not parse or has no host is never the same origin as anything, so an
// unexpected value asks for the secret rather than keeping it.
func sameOrigin(a, b string) bool {
	ua, errA := url.Parse(strings.TrimSpace(a))
	ub, errB := url.Parse(strings.TrimSpace(b))
	if errA != nil || errB != nil || ua.Hostname() == "" || ub.Hostname() == "" {
		return false
	}
	return strings.EqualFold(ua.Scheme, ub.Scheme) &&
		strings.EqualFold(ua.Hostname(), ub.Hostname()) &&
		effectivePort(ua) == effectivePort(ub)
}

func effectivePort(u *url.URL) string {
	if p := u.Port(); p != "" {
		return p
	}
	switch strings.ToLower(u.Scheme) {
	case "https":
		return "443"
	case "http":
		return "80"
	}
	return ""
}

// applyUpdate applies req onto a copy of current and persists it, publishing
// the copy only once the database has taken it. Shared by UpdateTenant
// (guarded, public, isConfigDefined=false) and SyncFromConfig (unguarded,
// isConfigDefined=true).
func (s *service) applyUpdate(ctx context.Context, current *domain.KeycloakTenant, req *UpdateRequest, isConfigDefined bool) (*domain.KeycloakTenant, error) {
	tenant := cloneTenant(current)
	tenant.IsConfigDefined = isConfigDefined

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
	if req.Amfa != nil {
		// 0 is never a valid EventsLookbackDays/APITimeoutSeconds (both are
		// validated as >= 1), so it can double as "not provided" the same way
		// an empty ClientSecret does above. The edit form only ever renders
		// enabled/api_base_url, so a plain rename-and-save round-trips these
		// as zero; without this, that save would silently wipe a previously
		// configured lookback/timeout back to the default.
		merged := *req.Amfa
		if merged.EventsLookbackDays == 0 && tenant.Amfa != nil {
			merged.EventsLookbackDays = tenant.Amfa.EventsLookbackDays
		}
		if merged.APITimeoutSeconds == 0 && tenant.Amfa != nil {
			merged.APITimeoutSeconds = tenant.Amfa.APITimeoutSeconds
		}
		tenant.Amfa = merged.toDomain()
	}
	if req.DefaultRealm != nil {
		tenant.DefaultRealm = *req.DefaultRealm
	}
	if req.Enabled != nil {
		tenant.Enabled = *req.Enabled
	}
	if req.IsDefault != nil {
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
	s.storeTenant(tenant)

	// Determine event type
	event := EventUpdated
	if !current.Enabled && tenant.Enabled {
		event = EventEnabled
	} else if current.Enabled && !tenant.Enabled {
		event = EventDisabled
	}

	// Notify callbacks
	s.notifyCallbacks(event, tenant)

	return tenant, nil
}

// DeleteTenant removes a tenant.
func (s *service) DeleteTenant(ctx context.Context, tenantID string) error {
	s.updateMu.Lock()
	defer s.updateMu.Unlock()
	tenant, err := s.GetTenant(ctx, tenantID)
	if err != nil {
		return fmt.Errorf("tenant %s not found", tenantID)
	}

	if tenant.IsConfigDefined {
		return ErrConfigDefinedTenantReadOnly
	}

	var afterDelete func(context.Context)
	if s.deletionHook != nil {
		afterDelete = s.deletionHook.PrepareDeletion(ctx, tenant)
	}

	if err := s.repo.Delete(ctx, tenantID); err != nil {
		return fmt.Errorf("failed to delete tenant: %w", err)
	}

	if afterDelete != nil {
		afterDelete(ctx)
	}

	// Remove from cache
	s.cacheMu.Lock()
	s.cache.Delete(tenantID)
	s.cacheMu.Unlock()

	// Notify callbacks
	s.notifyCallbacks(EventRemoved, tenant)

	return nil
}

// UpdateHealth updates the health status of a tenant.
func (s *service) UpdateHealth(ctx context.Context, tenantID, status, message string) error {
	if err := s.repo.UpdateHealth(ctx, tenantID, status, message); err != nil {
		return err
	}

	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	if value, ok := s.cache.Load(tenantID); ok {
		tenant := cloneTenant(value.(*domain.KeycloakTenant))
		tenant.HealthStatus = status
		tenant.HealthMessage = message
		tenant.LastHealthCheck = time.Now()
		s.cache.Store(tenantID, tenant)
	}

	return nil
}

// UpdateError records an error for a tenant.
func (s *service) UpdateError(ctx context.Context, tenantID, errorMsg string) error {
	if err := s.repo.UpdateError(ctx, tenantID, errorMsg); err != nil {
		return err
	}

	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	if value, ok := s.cache.Load(tenantID); ok {
		tenant := cloneTenant(value.(*domain.KeycloakTenant))
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
	s.callbacks = append(s.callbacks, newRegisteredCallback(callback))
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

// cloneTenant copies t far enough to be changed without touching the original:
// every field an update can replace is copied rather than shared.
func cloneTenant(t *domain.KeycloakTenant) *domain.KeycloakTenant {
	clone := *t
	if t.Tags != nil {
		clone.Tags = append([]string(nil), t.Tags...)
	}
	if t.Amfa != nil {
		amfa := *t.Amfa
		clone.Amfa = &amfa
	}
	if t.LastErrorAt != nil {
		at := *t.LastErrorAt
		clone.LastErrorAt = &at
	}
	return &clone
}

// storeTenant publishes a saved tenant to the cache. Its health and error
// fields are taken from the entry it replaces: the health and error writers
// own them, and one may have written since the tenant was copied, which the
// repository's Update leaves alone in the database too. A tenant that is now
// the default also clears the flag on every other cached tenant, to follow the
// repository having just cleared it in the database; nothing else reloads the
// cache before a restart.
func (s *service) storeTenant(tenant *domain.KeycloakTenant) {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()

	if value, ok := s.cache.Load(tenant.TenantID); ok {
		current := value.(*domain.KeycloakTenant)
		tenant.HealthStatus = current.HealthStatus
		tenant.HealthMessage = current.HealthMessage
		tenant.LastHealthCheck = current.LastHealthCheck
		tenant.LastError = current.LastError
		tenant.LastErrorAt = current.LastErrorAt
	}
	s.cache.Store(tenant.TenantID, tenant)
	if !tenant.IsDefault {
		return
	}

	s.cache.Range(func(key, value interface{}) bool {
		other := value.(*domain.KeycloakTenant)
		if other.TenantID == tenant.TenantID || !other.IsDefault {
			return true
		}
		cleared := cloneTenant(other)
		cleared.IsDefault = false
		s.cache.Store(key, cleared)
		return true
	})
}

// notifyCallbacks notifies all registered callbacks.
func (s *service) notifyCallbacks(event Event, tenant *domain.KeycloakTenant) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, rc := range s.callbacks {
		rc.queue <- callbackJob{event: event, tenant: tenant}
	}
}
