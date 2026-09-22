package tenant

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

// mockRepository implements Repository interface for testing
type mockRepository struct {
	getByIDFn       func(ctx context.Context, id uint) (*domain.KeycloakTenant, error)
	getByTenantIDFn func(ctx context.Context, tenantID string) (*domain.KeycloakTenant, error)
	listFn          func(ctx context.Context) ([]*domain.KeycloakTenant, error)
	listEnabledFn   func(ctx context.Context) ([]*domain.KeycloakTenant, error)
	getDefaultFn    func(ctx context.Context) (*domain.KeycloakTenant, error)
	createFn        func(ctx context.Context, tenant *domain.KeycloakTenant) error
	updateFn        func(ctx context.Context, tenant *domain.KeycloakTenant) error
	deleteFn        func(ctx context.Context, tenantID string) error
	updateHealthFn  func(ctx context.Context, tenantID, status, message string) error
	updateErrorFn   func(ctx context.Context, tenantID, errorMsg string) error
	unsetDefaultFn  func(ctx context.Context) error
}

func (m *mockRepository) GetByID(ctx context.Context, id uint) (*domain.KeycloakTenant, error) {
	if m.getByIDFn != nil {
		return m.getByIDFn(ctx, id)
	}
	return nil, nil
}

func (m *mockRepository) GetByTenantID(ctx context.Context, tenantID string) (*domain.KeycloakTenant, error) {
	if m.getByTenantIDFn != nil {
		return m.getByTenantIDFn(ctx, tenantID)
	}
	return nil, nil
}

func (m *mockRepository) List(ctx context.Context) ([]*domain.KeycloakTenant, error) {
	if m.listFn != nil {
		return m.listFn(ctx)
	}
	return nil, nil
}

func (m *mockRepository) ListEnabled(ctx context.Context) ([]*domain.KeycloakTenant, error) {
	if m.listEnabledFn != nil {
		return m.listEnabledFn(ctx)
	}
	return nil, nil
}

func (m *mockRepository) GetDefault(ctx context.Context) (*domain.KeycloakTenant, error) {
	if m.getDefaultFn != nil {
		return m.getDefaultFn(ctx)
	}
	return nil, nil
}

func (m *mockRepository) Create(ctx context.Context, tenant *domain.KeycloakTenant) error {
	if m.createFn != nil {
		return m.createFn(ctx, tenant)
	}
	return nil
}

func (m *mockRepository) Update(ctx context.Context, tenant *domain.KeycloakTenant) error {
	if m.updateFn != nil {
		return m.updateFn(ctx, tenant)
	}
	return nil
}

func (m *mockRepository) Delete(ctx context.Context, tenantID string) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, tenantID)
	}
	return nil
}

func (m *mockRepository) UpdateHealth(ctx context.Context, tenantID, status, message string) error {
	if m.updateHealthFn != nil {
		return m.updateHealthFn(ctx, tenantID, status, message)
	}
	return nil
}

func (m *mockRepository) UpdateError(ctx context.Context, tenantID, errorMsg string) error {
	if m.updateErrorFn != nil {
		return m.updateErrorFn(ctx, tenantID, errorMsg)
	}
	return nil
}

func (m *mockRepository) UnsetDefault(ctx context.Context) error {
	if m.unsetDefaultFn != nil {
		return m.unsetDefaultFn(ctx)
	}
	return nil
}

func TestLoadTenants(t *testing.T) {
	tests := []struct {
		name    string
		listFn  func(ctx context.Context) ([]*domain.KeycloakTenant, error)
		wantErr bool
	}{
		{
			name: "successful load",
			listFn: func(ctx context.Context) ([]*domain.KeycloakTenant, error) {
				return []*domain.KeycloakTenant{
					{TenantID: "tenant-1", Name: "Tenant 1"},
					{TenantID: "tenant-2", Name: "Tenant 2"},
				}, nil
			},
			wantErr: false,
		},
		{
			name: "load error",
			listFn: func(ctx context.Context) ([]*domain.KeycloakTenant, error) {
				return nil, errors.New("database error")
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockRepository{listFn: tt.listFn}
			svc := NewService(repo)

			err := svc.LoadTenants(context.Background())
			if (err != nil) != tt.wantErr {
				t.Errorf("LoadTenants() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestGetTenant(t *testing.T) {
	tests := []struct {
		name       string
		tenantID   string
		setupCache func(svc Service)
		repoFn     func(ctx context.Context, tenantID string) (*domain.KeycloakTenant, error)
		wantErr    bool
		wantTenant *domain.KeycloakTenant
	}{
		{
			name:     "get from cache",
			tenantID: "tenant-1",
			setupCache: func(svc Service) {
				// Pre-populate cache via LoadTenants
				s := svc.(*service)
				s.cache.Store("tenant-1", &domain.KeycloakTenant{TenantID: "tenant-1", Name: "Cached Tenant"})
			},
			repoFn:  nil, // Should not be called
			wantErr: false,
			wantTenant: &domain.KeycloakTenant{
				TenantID: "tenant-1",
				Name:     "Cached Tenant",
			},
		},
		{
			name:       "get from repository",
			tenantID:   "tenant-2",
			setupCache: nil,
			repoFn: func(ctx context.Context, tenantID string) (*domain.KeycloakTenant, error) {
				return &domain.KeycloakTenant{TenantID: tenantID, Name: "From Repo"}, nil
			},
			wantErr: false,
			wantTenant: &domain.KeycloakTenant{
				TenantID: "tenant-2",
				Name:     "From Repo",
			},
		},
		{
			name:       "not found",
			tenantID:   "non-existent",
			setupCache: nil,
			repoFn: func(ctx context.Context, tenantID string) (*domain.KeycloakTenant, error) {
				return nil, errors.New("tenant not found")
			},
			wantErr:    true,
			wantTenant: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockRepository{getByTenantIDFn: tt.repoFn}
			svc := NewService(repo)

			if tt.setupCache != nil {
				tt.setupCache(svc)
			}

			got, err := svc.GetTenant(context.Background(), tt.tenantID)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetTenant() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantTenant != nil && got != nil {
				if got.TenantID != tt.wantTenant.TenantID {
					t.Errorf("GetTenant() TenantID = %v, want %v", got.TenantID, tt.wantTenant.TenantID)
				}
				if got.Name != tt.wantTenant.Name {
					t.Errorf("GetTenant() Name = %v, want %v", got.Name, tt.wantTenant.Name)
				}
			}
		})
	}
}

func TestListTenants(t *testing.T) {
	repo := &mockRepository{}
	svc := NewService(repo)

	// Pre-populate cache
	s := svc.(*service)
	s.cache.Store("tenant-1", &domain.KeycloakTenant{TenantID: "tenant-1", Name: "Tenant 1"})
	s.cache.Store("tenant-2", &domain.KeycloakTenant{TenantID: "tenant-2", Name: "Tenant 2"})

	tenants, err := svc.ListTenants(context.Background())
	if err != nil {
		t.Errorf("ListTenants() error = %v", err)
	}
	if len(tenants) != 2 {
		t.Errorf("ListTenants() count = %v, want 2", len(tenants))
	}
}

func TestListEnabledTenants(t *testing.T) {
	repo := &mockRepository{}
	svc := NewService(repo)

	// Pre-populate cache
	s := svc.(*service)
	s.cache.Store("tenant-1", &domain.KeycloakTenant{TenantID: "tenant-1", Enabled: true})
	s.cache.Store("tenant-2", &domain.KeycloakTenant{TenantID: "tenant-2", Enabled: false})
	s.cache.Store("tenant-3", &domain.KeycloakTenant{TenantID: "tenant-3", Enabled: true})

	tenants, err := svc.ListEnabledTenants(context.Background())
	if err != nil {
		t.Errorf("ListEnabledTenants() error = %v", err)
	}
	if len(tenants) != 2 {
		t.Errorf("ListEnabledTenants() count = %v, want 2", len(tenants))
	}
}

func TestCreateTenant(t *testing.T) {
	tests := []struct {
		name     string
		req      *CreateRequest
		createFn func(ctx context.Context, tenant *domain.KeycloakTenant) error
		wantErr  bool
	}{
		{
			name: "successful create",
			req: &CreateRequest{
				TenantID:     "new-tenant",
				Name:         "New Tenant",
				ServerURL:    "https://keycloak.example.com",
				AdminRealm:   "master",
				ClientID:     "monitoring-service",
				ClientSecret: "client-secret",
			},
			createFn: func(ctx context.Context, tenant *domain.KeycloakTenant) error {
				return nil
			},
			wantErr: false,
		},
		{
			name: "missing tenant_id",
			req: &CreateRequest{
				Name:       "New Tenant",
				ServerURL:  "https://keycloak.example.com",
				AdminRealm: "master",
			},
			createFn: nil,
			wantErr:  true,
		},
		{
			name: "missing name",
			req: &CreateRequest{
				TenantID:   "new-tenant",
				ServerURL:  "https://keycloak.example.com",
				AdminRealm: "master",
			},
			createFn: nil,
			wantErr:  true,
		},
		{
			name: "missing server_url",
			req: &CreateRequest{
				TenantID:   "new-tenant",
				Name:       "New Tenant",
				AdminRealm: "master",
			},
			createFn: nil,
			wantErr:  true,
		},
		{
			name: "repository error",
			req: &CreateRequest{
				TenantID:     "new-tenant",
				Name:         "New Tenant",
				ServerURL:    "https://keycloak.example.com",
				AdminRealm:   "master",
				ClientID:     "monitoring-service",
				ClientSecret: "client-secret",
			},
			createFn: func(ctx context.Context, tenant *domain.KeycloakTenant) error {
				return errors.New("database error")
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockRepository{createFn: tt.createFn}
			svc := NewService(repo)

			tenant, err := svc.CreateTenant(context.Background(), tt.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("CreateTenant() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && tenant == nil {
				t.Error("CreateTenant() returned nil tenant on success")
			}
		})
	}
}

func TestCreateTenant_AlreadyExists(t *testing.T) {
	repo := &mockRepository{}
	svc := NewService(repo)

	// Pre-populate cache with existing tenant
	s := svc.(*service)
	s.cache.Store("existing-tenant", &domain.KeycloakTenant{TenantID: "existing-tenant"})

	req := &CreateRequest{
		TenantID:     "existing-tenant",
		Name:         "Existing Tenant",
		ServerURL:    "https://keycloak.example.com",
		AdminRealm:   "master",
		ClientID:     "monitoring-service",
		ClientSecret: "client-secret",
	}

	_, err := svc.CreateTenant(context.Background(), req)
	if err == nil {
		t.Error("CreateTenant() should fail for existing tenant")
	}
}

func TestSyncFromConfig_CreatesAndMarksConfigDefined(t *testing.T) {
	repo := &mockRepository{
		getByTenantIDFn: func(ctx context.Context, tenantID string) (*domain.KeycloakTenant, error) {
			return nil, errors.New("not found")
		},
	}
	svc := NewService(repo)

	createReq := &CreateRequest{
		TenantID: "cfg-tenant", Name: "Cfg Tenant", ServerURL: "https://keycloak.example.com",
		AdminRealm: "master", ClientID: "monitoring-service", ClientSecret: "secret",
	}

	got, err := svc.SyncFromConfig(context.Background(), "cfg-tenant", createReq, &UpdateRequest{})
	if err != nil {
		t.Fatalf("SyncFromConfig() unexpected error = %v", err)
	}
	if !got.IsConfigDefined {
		t.Error("SyncFromConfig() must mark a newly created tenant as IsConfigDefined")
	}
}

func TestSyncFromConfig_UpdatesAndBypassesReadOnlyGuard(t *testing.T) {
	repo := &mockRepository{
		updateFn: func(ctx context.Context, tenant *domain.KeycloakTenant) error { return nil },
		getByTenantIDFn: func(ctx context.Context, tenantID string) (*domain.KeycloakTenant, error) {
			return nil, errors.New("not found")
		},
	}
	svc := NewService(repo)
	s := svc.(*service)
	s.cache.Store("cfg-tenant", &domain.KeycloakTenant{
		TenantID: "cfg-tenant", Name: "Old Name", ClientID: "monitoring-service",
		ClientSecret: "secret", IsConfigDefined: true,
	})

	updateReq := &UpdateRequest{Name: strPtr("New Name From Config")}
	got, err := svc.SyncFromConfig(context.Background(), "cfg-tenant", &CreateRequest{}, updateReq)
	if err != nil {
		t.Fatalf("SyncFromConfig() unexpected error = %v, want config-sync to bypass the read-only guard", err)
	}
	if got.Name != "New Name From Config" {
		t.Errorf("SyncFromConfig() Name = %q, want %q", got.Name, "New Name From Config")
	}
	if !got.IsConfigDefined {
		t.Error("SyncFromConfig() must keep the tenant marked IsConfigDefined")
	}
}

// TestSyncFromConfig_RefusesToTakeOverUIManagedTenant covers a tenant_id
// collision: a UI-created tenant already exists under the ID a config.yaml
// entry now wants to use. SyncFromConfig must not silently overwrite and
// permanently lock it — that would surprise whoever created it in the UI.
func TestSyncFromConfig_RefusesToTakeOverUIManagedTenant(t *testing.T) {
	updateCalled := false
	repo := &mockRepository{
		updateFn: func(ctx context.Context, tenant *domain.KeycloakTenant) error {
			updateCalled = true
			return nil
		},
		getByTenantIDFn: func(ctx context.Context, tenantID string) (*domain.KeycloakTenant, error) {
			return nil, errors.New("not found")
		},
	}
	svc := NewService(repo)
	s := svc.(*service)
	s.cache.Store("shared-id", &domain.KeycloakTenant{
		TenantID: "shared-id", Name: "UI Tenant", ClientID: "monitoring-service",
		ClientSecret: "secret", IsConfigDefined: false,
	})

	createReq := &CreateRequest{TenantID: "shared-id", Name: "Cfg Tenant"}
	updateReq := &UpdateRequest{Name: strPtr("Cfg Tenant")}
	_, err := svc.SyncFromConfig(context.Background(), "shared-id", createReq, updateReq)
	if err == nil {
		t.Fatal("SyncFromConfig() expected an error for a UI-managed tenant_id collision, got nil")
	}
	if updateCalled {
		t.Error("SyncFromConfig() must not modify a UI-managed tenant on ID collision")
	}

	unchanged, _ := s.cache.Load("shared-id")
	ut := unchanged.(*domain.KeycloakTenant)
	if ut.Name != "UI Tenant" || ut.IsConfigDefined {
		t.Errorf("SyncFromConfig() must leave the UI-managed tenant untouched, got Name=%q IsConfigDefined=%v", ut.Name, ut.IsConfigDefined)
	}
}

// TestSyncFromConfig_DoesNotReadoptPreviouslyUnlockedTenant documents an
// intentional consequence of the collision guard: a tenant that WAS
// config-defined, got unlocked by unlockOrphanedConfigDefinedTenants because
// it left config.yaml, is indistinguishable from a genuine UI-created tenant
// once IsConfigDefined is false. Re-adding the same tenant_id to config.yaml
// does not silently re-lock it — recovering config-management requires
// deleting the row first. This is the same guard as the fresh-collision
// case; the test exists to make the behavior explicit and regression-proof.
func TestSyncFromConfig_DoesNotReadoptPreviouslyUnlockedTenant(t *testing.T) {
	repo := &mockRepository{
		getByTenantIDFn: func(ctx context.Context, tenantID string) (*domain.KeycloakTenant, error) {
			return nil, errors.New("not found")
		},
	}
	svc := NewService(repo)
	s := svc.(*service)
	// Simulates a tenant that was config-defined, then unlocked (e.g. by
	// unlockOrphanedConfigDefinedTenants) and possibly edited afterward.
	s.cache.Store("was-config", &domain.KeycloakTenant{
		TenantID: "was-config", Name: "Edited After Unlock", ClientID: "monitoring-service",
		ClientSecret: "secret", IsConfigDefined: false,
	})

	createReq := &CreateRequest{TenantID: "was-config", Name: "Cfg Tenant"}
	updateReq := &UpdateRequest{Name: strPtr("Cfg Tenant")}
	_, err := svc.SyncFromConfig(context.Background(), "was-config", createReq, updateReq)
	if err == nil {
		t.Fatal("SyncFromConfig() expected an error when re-adding a previously-unlocked tenant_id to config.yaml, got nil")
	}
}

func TestListConfigDefinedTenants(t *testing.T) {
	repo := &mockRepository{
		listFn: func(ctx context.Context) ([]*domain.KeycloakTenant, error) {
			return []*domain.KeycloakTenant{
				{TenantID: "cfg-1", IsConfigDefined: true},
				{TenantID: "ui-1", IsConfigDefined: false},
				{TenantID: "cfg-2", IsConfigDefined: true},
			}, nil
		},
	}
	svc := NewService(repo)

	got, err := svc.ListConfigDefinedTenants(context.Background())
	if err != nil {
		t.Fatalf("ListConfigDefinedTenants() unexpected error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ListConfigDefinedTenants() returned %d tenants, want 2", len(got))
	}
	for _, ct := range got {
		if !ct.IsConfigDefined {
			t.Errorf("ListConfigDefinedTenants() returned non-config-defined tenant %q", ct.TenantID)
		}
	}
}

func TestUnmarkConfigDefined(t *testing.T) {
	repo := &mockRepository{
		updateFn: func(ctx context.Context, tenant *domain.KeycloakTenant) error { return nil },
		getByTenantIDFn: func(ctx context.Context, tenantID string) (*domain.KeycloakTenant, error) {
			return nil, errors.New("not found")
		},
	}
	svc := NewService(repo)
	s := svc.(*service)
	s.cache.Store("cfg-tenant", &domain.KeycloakTenant{TenantID: "cfg-tenant", IsConfigDefined: true})

	if err := svc.UnmarkConfigDefined(context.Background(), "cfg-tenant"); err != nil {
		t.Fatalf("UnmarkConfigDefined() unexpected error = %v", err)
	}

	updated, _ := s.cache.Load("cfg-tenant")
	if updated.(*domain.KeycloakTenant).IsConfigDefined {
		t.Error("UnmarkConfigDefined() must clear IsConfigDefined")
	}
}

func TestUnmarkConfigDefined_NoopWhenAlreadyUnmarked(t *testing.T) {
	updateCalled := false
	repo := &mockRepository{
		updateFn: func(ctx context.Context, tenant *domain.KeycloakTenant) error {
			updateCalled = true
			return nil
		},
		getByTenantIDFn: func(ctx context.Context, tenantID string) (*domain.KeycloakTenant, error) {
			return nil, errors.New("not found")
		},
	}
	svc := NewService(repo)
	s := svc.(*service)
	s.cache.Store("ui-tenant", &domain.KeycloakTenant{TenantID: "ui-tenant", IsConfigDefined: false})

	if err := svc.UnmarkConfigDefined(context.Background(), "ui-tenant"); err != nil {
		t.Fatalf("UnmarkConfigDefined() unexpected error = %v", err)
	}
	if updateCalled {
		t.Error("UnmarkConfigDefined() must not write when the tenant is already not config-defined")
	}
}

func TestUpdateTenant(t *testing.T) {
	tests := []struct {
		name       string
		tenantID   string
		req        *UpdateRequest
		setupCache func(svc Service)
		updateFn   func(ctx context.Context, tenant *domain.KeycloakTenant) error
		wantErr    bool
	}{
		{
			name:     "successful update",
			tenantID: "tenant-1",
			req: &UpdateRequest{
				Name: strPtr("Updated Name"),
			},
			setupCache: func(svc Service) {
				s := svc.(*service)
				s.cache.Store("tenant-1", &domain.KeycloakTenant{TenantID: "tenant-1", Name: "Original Name", Enabled: true, ClientID: "monitoring-service", ClientSecret: "secret"})
			},
			updateFn: func(ctx context.Context, tenant *domain.KeycloakTenant) error {
				return nil
			},
			wantErr: false,
		},
		{
			name:     "tenant not found",
			tenantID: "non-existent",
			req: &UpdateRequest{
				Name: strPtr("Updated Name"),
			},
			setupCache: nil,
			updateFn:   nil,
			wantErr:    true,
		},
		{
			name:     "update enabled status",
			tenantID: "tenant-1",
			req: &UpdateRequest{
				Enabled: boolPtr(false),
			},
			setupCache: func(svc Service) {
				s := svc.(*service)
				s.cache.Store("tenant-1", &domain.KeycloakTenant{TenantID: "tenant-1", Enabled: true, ClientID: "monitoring-service", ClientSecret: "secret"})
			},
			updateFn: func(ctx context.Context, tenant *domain.KeycloakTenant) error {
				return nil
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockRepository{
				updateFn: tt.updateFn,
				getByTenantIDFn: func(ctx context.Context, tenantID string) (*domain.KeycloakTenant, error) {
					return nil, errors.New("not found")
				},
			}
			svc := NewService(repo)

			if tt.setupCache != nil {
				tt.setupCache(svc)
			}

			tenant, err := svc.UpdateTenant(context.Background(), tt.tenantID, tt.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("UpdateTenant() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && tenant == nil {
				t.Error("UpdateTenant() returned nil tenant on success")
			}
		})
	}
}

func TestUpdateTenant_RejectsConfigDefined(t *testing.T) {
	repo := &mockRepository{
		getByTenantIDFn: func(ctx context.Context, tenantID string) (*domain.KeycloakTenant, error) {
			return nil, errors.New("not found")
		},
	}
	svc := NewService(repo)
	s := svc.(*service)
	s.cache.Store("tenant-1", &domain.KeycloakTenant{
		TenantID: "tenant-1", Name: "Config Tenant", Enabled: true,
		ClientID: "monitoring-service", ClientSecret: "secret", IsConfigDefined: true,
	})

	_, err := svc.UpdateTenant(context.Background(), "tenant-1", &UpdateRequest{Name: strPtr("New Name")})
	if !errors.Is(err, ErrConfigDefinedTenantReadOnly) {
		t.Fatalf("UpdateTenant() error = %v, want %v", err, ErrConfigDefinedTenantReadOnly)
	}

	unchanged, _ := s.cache.Load("tenant-1")
	if unchanged.(*domain.KeycloakTenant).Name != "Config Tenant" {
		t.Error("UpdateTenant() must not modify a config-defined tenant, but Name changed")
	}
}

// TestUpdateTenant_ClientSecret covers the write-only secret field: the edit
// form never renders the stored secret, so it submits an empty value when the
// user leaves it untouched. That must be treated as "no change" rather than
// clearing the credential, while a real rotation must still be applied.
func TestUpdateTenant_ClientSecret(t *testing.T) {
	const storedSecret = "stored-secret"

	tests := []struct {
		name       string
		req        *UpdateRequest
		wantSecret string
	}{
		{
			name:       "empty client_secret retains stored secret",
			req:        &UpdateRequest{Name: strPtr("Updated Name"), ClientSecret: strPtr("")},
			wantSecret: storedSecret,
		},
		{
			name:       "omitted client_secret retains stored secret",
			req:        &UpdateRequest{Name: strPtr("Updated Name")},
			wantSecret: storedSecret,
		},
		{
			name:       "non-empty client_secret rotates the secret",
			req:        &UpdateRequest{ClientSecret: strPtr("rotated-secret")},
			wantSecret: "rotated-secret",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var persisted string
			repo := &mockRepository{
				updateFn: func(ctx context.Context, tenant *domain.KeycloakTenant) error {
					persisted = tenant.ClientSecret
					return nil
				},
				getByTenantIDFn: func(ctx context.Context, tenantID string) (*domain.KeycloakTenant, error) {
					return nil, errors.New("not found")
				},
			}
			svc := NewService(repo)
			svc.(*service).cache.Store("tenant-1", &domain.KeycloakTenant{
				TenantID:     "tenant-1",
				Name:         "Original Name",
				Enabled:      true,
				ClientID:     "monitoring-service",
				ClientSecret: storedSecret,
			})

			updated, err := svc.UpdateTenant(context.Background(), "tenant-1", tt.req)
			if err != nil {
				t.Fatalf("UpdateTenant() unexpected error = %v", err)
			}

			if persisted != tt.wantSecret {
				t.Errorf("persisted client_secret = %q, want %q", persisted, tt.wantSecret)
			}
			if updated.ClientSecret != tt.wantSecret {
				t.Errorf("returned client_secret = %q, want %q", updated.ClientSecret, tt.wantSecret)
			}
		})
	}
}

func TestUpdateTenant_AmfaLookbackAndTimeoutSurviveAnUnrelatedEdit(t *testing.T) {
	// The edit form only ever renders amfa.enabled/api_base_url, so a plain
	// rename-and-save submits events_lookback_days/api_timeout_seconds as the
	// zero value. Since both are validated as >= 1 elsewhere, 0 can never be a
	// legitimate explicit value, so it doubles as "not provided" the same way
	// an empty ClientSecret does.
	tests := []struct {
		name             string
		req              *UpdateRequest
		wantLookbackDays int
		wantTimeout      int
	}{
		{
			name: "zero lookback/timeout in the request keeps the stored values",
			req: &UpdateRequest{
				Name: strPtr("Updated Name"),
				Amfa: &AmfaRequest{Enabled: true, APIBaseURL: "https://amfa.internal"},
			},
			wantLookbackDays: 14,
			wantTimeout:      45,
		},
		{
			name: "a non-zero value in the request still rotates it",
			req: &UpdateRequest{
				Amfa: &AmfaRequest{
					Enabled: true, APIBaseURL: "https://amfa.internal",
					EventsLookbackDays: 7, APITimeoutSeconds: 20,
				},
			},
			wantLookbackDays: 7,
			wantTimeout:      20,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var persisted *domain.TenantAmfa
			repo := &mockRepository{
				updateFn: func(ctx context.Context, tenant *domain.KeycloakTenant) error {
					persisted = tenant.Amfa
					return nil
				},
				getByTenantIDFn: func(ctx context.Context, tenantID string) (*domain.KeycloakTenant, error) {
					return nil, errors.New("not found")
				},
			}
			svc := NewService(repo)
			svc.(*service).cache.Store("tenant-1", &domain.KeycloakTenant{
				TenantID:     "tenant-1",
				Name:         "Original Name",
				Enabled:      true,
				ClientID:     "monitoring-service",
				ClientSecret: "stored-secret",
				Amfa: &domain.TenantAmfa{
					Enabled: true, APIBaseURL: "https://amfa.internal",
					EventsLookbackDays: 14, APITimeoutSeconds: 45,
				},
			})

			if _, err := svc.UpdateTenant(context.Background(), "tenant-1", tt.req); err != nil {
				t.Fatalf("UpdateTenant() unexpected error = %v", err)
			}

			if persisted == nil {
				t.Fatal("expected Amfa to be persisted")
			}
			if persisted.EventsLookbackDays != tt.wantLookbackDays {
				t.Errorf("EventsLookbackDays = %d, want %d", persisted.EventsLookbackDays, tt.wantLookbackDays)
			}
			if persisted.APITimeoutSeconds != tt.wantTimeout {
				t.Errorf("APITimeoutSeconds = %d, want %d", persisted.APITimeoutSeconds, tt.wantTimeout)
			}
		})
	}
}

func TestNotifyCallbacksDeliversEventsInOrderPerCallback(t *testing.T) {
	// A callback that's slow to process one event must not let a later event,
	// dispatched while the first is still in flight, be applied first. Firing
	// `go callback(...)` per event independently (the old implementation) gave
	// no such guarantee between two goroutines.
	svc := NewService(&mockRepository{})

	var mu sync.Mutex
	var received []Event
	done := make(chan struct{})

	svc.RegisterCallback(func(event Event, _ *domain.KeycloakTenant) {
		if event == EventEnabled {
			time.Sleep(50 * time.Millisecond)
		}
		mu.Lock()
		received = append(received, event)
		n := len(received)
		mu.Unlock()
		if n == 2 {
			close(done)
		}
	})

	s := svc.(*service)
	s.notifyCallbacks(EventEnabled, &domain.KeycloakTenant{TenantID: "t1"})
	s.notifyCallbacks(EventDisabled, &domain.KeycloakTenant{TenantID: "t1"})

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for both events to be delivered")
	}

	mu.Lock()
	defer mu.Unlock()
	if len(received) != 2 || received[0] != EventEnabled || received[1] != EventDisabled {
		t.Errorf("received = %v, want [%v %v]", received, EventEnabled, EventDisabled)
	}
}

func TestDeleteTenant(t *testing.T) {
	tests := []struct {
		name       string
		tenantID   string
		setupCache func(svc Service)
		deleteFn   func(ctx context.Context, tenantID string) error
		wantErr    bool
	}{
		{
			name:     "successful delete",
			tenantID: "tenant-1",
			setupCache: func(svc Service) {
				s := svc.(*service)
				s.cache.Store("tenant-1", &domain.KeycloakTenant{TenantID: "tenant-1"})
			},
			deleteFn: func(ctx context.Context, tenantID string) error {
				return nil
			},
			wantErr: false,
		},
		{
			name:       "tenant not found",
			tenantID:   "non-existent",
			setupCache: nil,
			deleteFn:   nil,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockRepository{
				deleteFn: tt.deleteFn,
				getByTenantIDFn: func(ctx context.Context, tenantID string) (*domain.KeycloakTenant, error) {
					return nil, errors.New("not found")
				},
			}
			svc := NewService(repo)

			if tt.setupCache != nil {
				tt.setupCache(svc)
			}

			err := svc.DeleteTenant(context.Background(), tt.tenantID)
			if (err != nil) != tt.wantErr {
				t.Errorf("DeleteTenant() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDeleteTenant_RejectsConfigDefined(t *testing.T) {
	deleteCalled := false
	repo := &mockRepository{
		deleteFn: func(ctx context.Context, tenantID string) error {
			deleteCalled = true
			return nil
		},
		getByTenantIDFn: func(ctx context.Context, tenantID string) (*domain.KeycloakTenant, error) {
			return nil, errors.New("not found")
		},
	}
	svc := NewService(repo)
	s := svc.(*service)
	s.cache.Store("tenant-1", &domain.KeycloakTenant{TenantID: "tenant-1", IsConfigDefined: true})

	err := svc.DeleteTenant(context.Background(), "tenant-1")
	if !errors.Is(err, ErrConfigDefinedTenantReadOnly) {
		t.Fatalf("DeleteTenant() error = %v, want %v", err, ErrConfigDefinedTenantReadOnly)
	}
	if deleteCalled {
		t.Error("DeleteTenant() must not call repo.Delete for a config-defined tenant")
	}
	if _, ok := s.cache.Load("tenant-1"); !ok {
		t.Error("DeleteTenant() must not remove a config-defined tenant from cache")
	}
}

func TestUpdateHealth(t *testing.T) {
	tests := []struct {
		name       string
		tenantID   string
		status     string
		message    string
		setupCache func(svc Service)
		updateFn   func(ctx context.Context, tenantID, status, message string) error
		wantErr    bool
		checkCache func(t *testing.T, svc Service)
	}{
		{
			name:     "successful health update",
			tenantID: "tenant-1",
			status:   "healthy",
			message:  "All systems operational",
			setupCache: func(svc Service) {
				s := svc.(*service)
				s.cache.Store("tenant-1", &domain.KeycloakTenant{TenantID: "tenant-1", HealthStatus: "unknown"})
			},
			updateFn: func(ctx context.Context, tenantID, status, message string) error {
				return nil
			},
			wantErr: false,
			checkCache: func(t *testing.T, svc Service) {
				s := svc.(*service)
				if v, ok := s.cache.Load("tenant-1"); ok {
					tenant := v.(*domain.KeycloakTenant)
					if tenant.HealthStatus != "healthy" {
						t.Errorf("Cache HealthStatus = %v, want healthy", tenant.HealthStatus)
					}
				}
			},
		},
		{
			name:       "update error",
			tenantID:   "tenant-1",
			status:     "unhealthy",
			message:    "Connection failed",
			setupCache: nil,
			updateFn: func(ctx context.Context, tenantID, status, message string) error {
				return errors.New("database error")
			},
			wantErr:    true,
			checkCache: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockRepository{updateHealthFn: tt.updateFn}
			svc := NewService(repo)

			if tt.setupCache != nil {
				tt.setupCache(svc)
			}

			err := svc.UpdateHealth(context.Background(), tt.tenantID, tt.status, tt.message)
			if (err != nil) != tt.wantErr {
				t.Errorf("UpdateHealth() error = %v, wantErr %v", err, tt.wantErr)
			}

			if tt.checkCache != nil {
				tt.checkCache(t, svc)
			}
		})
	}
}

func TestUpdateError(t *testing.T) {
	tests := []struct {
		name       string
		tenantID   string
		errorMsg   string
		setupCache func(svc Service)
		updateFn   func(ctx context.Context, tenantID, errorMsg string) error
		wantErr    bool
	}{
		{
			name:     "successful error update",
			tenantID: "tenant-1",
			errorMsg: "Connection timeout",
			setupCache: func(svc Service) {
				s := svc.(*service)
				s.cache.Store("tenant-1", &domain.KeycloakTenant{TenantID: "tenant-1"})
			},
			updateFn: func(ctx context.Context, tenantID, errorMsg string) error {
				return nil
			},
			wantErr: false,
		},
		{
			name:       "update error fails",
			tenantID:   "tenant-1",
			errorMsg:   "Some error",
			setupCache: nil,
			updateFn: func(ctx context.Context, tenantID, errorMsg string) error {
				return errors.New("database error")
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockRepository{updateErrorFn: tt.updateFn}
			svc := NewService(repo)

			if tt.setupCache != nil {
				tt.setupCache(svc)
			}

			err := svc.UpdateError(context.Background(), tt.tenantID, tt.errorMsg)
			if (err != nil) != tt.wantErr {
				t.Errorf("UpdateError() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestGetHealth(t *testing.T) {
	tests := []struct {
		name       string
		tenantID   string
		setupCache func(svc Service)
		wantErr    bool
		wantStatus string
	}{
		{
			name:     "successful get health",
			tenantID: "tenant-1",
			setupCache: func(svc Service) {
				s := svc.(*service)
				s.cache.Store("tenant-1", &domain.KeycloakTenant{
					TenantID:      "tenant-1",
					HealthStatus:  "healthy",
					HealthMessage: "All good",
				})
			},
			wantErr:    false,
			wantStatus: "healthy",
		},
		{
			name:       "tenant not found",
			tenantID:   "non-existent",
			setupCache: nil,
			wantErr:    true,
			wantStatus: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockRepository{
				getByTenantIDFn: func(ctx context.Context, tenantID string) (*domain.KeycloakTenant, error) {
					return nil, errors.New("not found")
				},
			}
			svc := NewService(repo)

			if tt.setupCache != nil {
				tt.setupCache(svc)
			}

			health, err := svc.GetHealth(context.Background(), tt.tenantID)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetHealth() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && health.Status != tt.wantStatus {
				t.Errorf("GetHealth() Status = %v, want %v", health.Status, tt.wantStatus)
			}
		})
	}
}

func TestRegisterCallback(t *testing.T) {
	repo := &mockRepository{}
	svc := NewService(repo)

	callCount := 0
	callback := func(event Event, tenant *domain.KeycloakTenant) {
		callCount++
	}

	svc.RegisterCallback(callback)

	// Access the internal service to check callback was registered
	s := svc.(*service)
	if len(s.callbacks) != 1 {
		t.Errorf("RegisterCallback() callbacks count = %v, want 1", len(s.callbacks))
	}
}

func TestGetDefault(t *testing.T) {
	tests := []struct {
		name    string
		getFn   func(ctx context.Context) (*domain.KeycloakTenant, error)
		wantErr bool
	}{
		{
			name: "successful get default",
			getFn: func(ctx context.Context) (*domain.KeycloakTenant, error) {
				return &domain.KeycloakTenant{TenantID: "default-tenant", IsDefault: true}, nil
			},
			wantErr: false,
		},
		{
			name: "no default tenant",
			getFn: func(ctx context.Context) (*domain.KeycloakTenant, error) {
				return nil, errors.New("no default tenant")
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockRepository{getDefaultFn: tt.getFn}
			svc := NewService(repo)

			tenant, err := svc.GetDefault(context.Background())
			if (err != nil) != tt.wantErr {
				t.Errorf("GetDefault() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && tenant == nil {
				t.Error("GetDefault() returned nil tenant on success")
			}
		})
	}
}

func TestValidateCreateRequest(t *testing.T) {
	repo := &mockRepository{}
	svc := NewService(repo)
	s := svc.(*service)

	tests := []struct {
		name    string
		req     *CreateRequest
		wantErr bool
	}{
		{
			name: "valid request",
			req: &CreateRequest{
				TenantID:     "tenant-1",
				Name:         "Tenant",
				ServerURL:    "https://example.com",
				AdminRealm:   "master",
				ClientID:     "monitoring-service",
				ClientSecret: "client-secret",
			},
			wantErr: false,
		},
		{
			name: "missing client_secret",
			req: &CreateRequest{
				TenantID:   "tenant-1",
				Name:       "Tenant",
				ServerURL:  "https://example.com",
				AdminRealm: "master",
				ClientID:   "monitoring-service",
			},
			wantErr: true,
		},
		{
			name: "missing tenant_id",
			req: &CreateRequest{
				Name:       "Tenant",
				ServerURL:  "https://example.com",
				AdminRealm: "master",
			},
			wantErr: true,
		},
		{
			name: "missing admin_realm",
			req: &CreateRequest{
				TenantID:  "tenant-1",
				Name:      "Tenant",
				ServerURL: "https://example.com",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := s.validateCreateRequest(tt.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateCreateRequest() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// Helper functions
func strPtr(s string) *string {
	return &s
}

func boolPtr(b bool) *bool {
	return &b
}
