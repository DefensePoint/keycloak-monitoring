package chi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"

	"github.com/DefensePoint/keycloak-monitoring/internal/alerts"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
)

// newTestLogger creates a logger that discards output for tests
func newTestLogger() *logger.Logger {
	return logger.New(zerolog.New(io.Discard))
}

// Mock alerts service
type mockAlertsService struct {
	getAlertFn       func(ctx context.Context, tenantID, alertID string) (*domain.Alert, error)
	getAlertGlobalFn func(ctx context.Context, alertID string) (*domain.Alert, error)
	listAlertsFn     func(ctx context.Context, tenantID string, opts *alerts.ListOptions) ([]*domain.Alert, int64, error)
	listByRealmFn    func(ctx context.Context, tenantID, realmName string, opts *alerts.ListOptions) ([]*domain.Alert, error)
	getStatisticsFn  func(ctx context.Context, tenantID string, realmName ...string) (*alerts.Statistics, error)
	updateStatusFn   func(ctx context.Context, tenantID, alertID string, status domain.AlertStatus, acknowledgedBy string) (*domain.Alert, error)
	resolveAlertFn   func(ctx context.Context, tenantID, alertID string) (*domain.Alert, error)
	deleteAlertFn    func(ctx context.Context, tenantID, alertID string) error
	saveAlertFn      func(ctx context.Context, alert *domain.Alert) error
}

func (m *mockAlertsService) GetAlert(ctx context.Context, tenantID, alertID string) (*domain.Alert, error) {
	if m.getAlertFn != nil {
		return m.getAlertFn(ctx, tenantID, alertID)
	}
	return nil, nil
}

func (m *mockAlertsService) GetAlertGlobal(ctx context.Context, alertID string) (*domain.Alert, error) {
	if m.getAlertGlobalFn != nil {
		return m.getAlertGlobalFn(ctx, alertID)
	}
	return nil, nil
}

func (m *mockAlertsService) ListAlerts(ctx context.Context, tenantID string, opts *alerts.ListOptions) ([]*domain.Alert, int64, error) {
	if m.listAlertsFn != nil {
		return m.listAlertsFn(ctx, tenantID, opts)
	}
	return nil, 0, nil
}

func (m *mockAlertsService) ListAlertsByRealm(ctx context.Context, tenantID, realmName string, opts *alerts.ListOptions) ([]*domain.Alert, error) {
	if m.listByRealmFn != nil {
		return m.listByRealmFn(ctx, tenantID, realmName, opts)
	}
	return nil, nil
}

func (m *mockAlertsService) GetStatistics(ctx context.Context, tenantID string, realmName ...string) (*alerts.Statistics, error) {
	if m.getStatisticsFn != nil {
		return m.getStatisticsFn(ctx, tenantID, realmName...)
	}
	return nil, nil
}

func (m *mockAlertsService) UpdateStatus(ctx context.Context, tenantID, alertID string, status domain.AlertStatus, acknowledgedBy string) (*domain.Alert, error) {
	if m.updateStatusFn != nil {
		return m.updateStatusFn(ctx, tenantID, alertID, status, acknowledgedBy)
	}
	return nil, nil
}

func (m *mockAlertsService) ResolveAlert(ctx context.Context, tenantID, alertID string) (*domain.Alert, error) {
	if m.resolveAlertFn != nil {
		return m.resolveAlertFn(ctx, tenantID, alertID)
	}
	return nil, nil
}

func (m *mockAlertsService) DeleteAlert(ctx context.Context, tenantID, alertID string) error {
	if m.deleteAlertFn != nil {
		return m.deleteAlertFn(ctx, tenantID, alertID)
	}
	return nil
}

func (m *mockAlertsService) SaveAlert(ctx context.Context, alert *domain.Alert) error {
	if m.saveAlertFn != nil {
		return m.saveAlertFn(ctx, alert)
	}
	return nil
}

// Mock RBAC checker - always allows access
type mockRBACChecker struct {
	hasPermissionFn    func(ctx context.Context, userID uint, permission string, tenantID *string) (bool, error)
	hasAccessToRealmFn func(ctx context.Context, userID uint, tenantID, realmName string) (bool, error)

	isAdmin      bool
	isAdminCalls int
	policies     []*domain.TenantPolicy
	policiesErr  error
}

func (m *mockRBACChecker) HasPermission(ctx context.Context, userID uint, permission string, tenantID *string) (bool, error) {
	if m.hasPermissionFn != nil {
		return m.hasPermissionFn(ctx, userID, permission, tenantID)
	}
	return true, nil // Default: allow all
}

func (m *mockRBACChecker) GetUserPermissions(ctx context.Context, userID uint, tenantID *string) ([]string, error) {
	return []string{}, nil
}

func (m *mockRBACChecker) IsAdmin(ctx context.Context, userID uint) (bool, error) {
	m.isAdminCalls++
	return m.isAdmin, nil
}

func (m *mockRBACChecker) GetUserRoles(ctx context.Context, userID uint) ([]*UserRoleInfo, error) {
	return []*UserRoleInfo{}, nil
}

func (m *mockRBACChecker) HasAccessToTenant(ctx context.Context, userID uint, tenantID string) (bool, error) {
	return true, nil
}

func (m *mockRBACChecker) HasAccessToRealm(ctx context.Context, userID uint, tenantID, realmName string) (bool, error) {
	if m.hasAccessToRealmFn != nil {
		return m.hasAccessToRealmFn(ctx, userID, tenantID, realmName)
	}
	return true, nil
}

func (m *mockRBACChecker) GetUserPolicies(ctx context.Context, userID uint) ([]*domain.TenantPolicy, error) {
	return m.policies, m.policiesErr
}

// Mock operator service
type mockOperatorService struct{}

func (m *mockOperatorService) RecordAction(ctx context.Context, action *domain.OperatorAction) error {
	return nil
}

func (m *mockOperatorService) GetLastActionForAlert(ctx context.Context, tenantID string, alertID uint) (*domain.OperatorAction, error) {
	return nil, nil
}

// Mock notification service
type mockNotificationService struct{}

func (m *mockNotificationService) NotifyAlertResolution(ctx context.Context, alert *domain.Alert) error {
	return nil
}

// Middleware that adds user info to context (simulates auth)
func testAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userDetails := &UserDetails{
			ID:    1,
			Email: "test@example.com",
		}
		ctx := context.WithValue(r.Context(), UserInfoKey, userDetails)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// Helper to create test handler
func newTestAlertsHandler(svc *mockAlertsService) *AlertsHandlers {
	if svc == nil {
		svc = &mockAlertsService{}
	}
	rbacMw := NewRBACMiddleware(&mockRBACChecker{}, newTestLogger())
	return NewAlertsHandlers(
		svc,
		&mockRBACChecker{},
		&mockOperatorService{},
		&mockNotificationService{},
		newTestLogger(),
		testAuthMiddleware,
		rbacMw,
	)
}

func newTestAlertsHandlerWithRBAC(svc *mockAlertsService, rbacSvc RBACChecker) *AlertsHandlers {
	if svc == nil {
		svc = &mockAlertsService{}
	}
	return NewAlertsHandlers(
		svc,
		rbacSvc,
		&mockOperatorService{},
		&mockNotificationService{},
		newTestLogger(),
		testAuthMiddleware,
		NewRBACMiddleware(rbacSvc, newTestLogger()),
	)
}

// Helper to create router with tenant routes
func setupTestRouter(h *AlertsHandlers) *chi.Mux {
	r := chi.NewRouter()
	r.Route("/api/tenants/{tenantID}", func(r chi.Router) {
		r.Use(TenantIDMiddleware)
		h.RegisterTenantRoutes(r)
	})
	return r
}

func TestHandleListAlerts(t *testing.T) {
	tests := []struct {
		name           string
		tenantID       string
		queryParams    string
		mockFn         func(ctx context.Context, tenantID string, opts *alerts.ListOptions) ([]*domain.Alert, int64, error)
		wantStatusCode int
		wantCount      int
	}{
		{
			name:        "successful list",
			tenantID:    "tenant-1",
			queryParams: "",
			mockFn: func(ctx context.Context, tenantID string, opts *alerts.ListOptions) ([]*domain.Alert, int64, error) {
				return []*domain.Alert{
					{AlertID: "alert-1", TenantID: tenantID},
					{AlertID: "alert-2", TenantID: tenantID},
				}, 2, nil
			},
			wantStatusCode: http.StatusOK,
			wantCount:      2,
		},
		{
			name:        "list with pagination",
			tenantID:    "tenant-1",
			queryParams: "?limit=10&offset=5",
			mockFn: func(ctx context.Context, tenantID string, opts *alerts.ListOptions) ([]*domain.Alert, int64, error) {
				if opts.Limit != 10 || opts.Offset != 5 {
					t.Errorf("Expected limit=10, offset=5, got limit=%d, offset=%d", opts.Limit, opts.Offset)
				}
				return []*domain.Alert{}, 0, nil
			},
			wantStatusCode: http.StatusOK,
			wantCount:      0,
		},
		{
			name:        "list with filters",
			tenantID:    "tenant-1",
			queryParams: "?severity=high&status=active",
			mockFn: func(ctx context.Context, tenantID string, opts *alerts.ListOptions) ([]*domain.Alert, int64, error) {
				if opts.Severity != domain.AlertSeverity("high") {
					t.Errorf("Expected severity=high, got %s", opts.Severity)
				}
				if opts.Status != domain.AlertStatus("active") {
					t.Errorf("Expected status=active, got %s", opts.Status)
				}
				return []*domain.Alert{}, 0, nil
			},
			wantStatusCode: http.StatusOK,
			wantCount:      0,
		},
		{
			name:        "service error",
			tenantID:    "tenant-1",
			queryParams: "",
			mockFn: func(ctx context.Context, tenantID string, opts *alerts.ListOptions) ([]*domain.Alert, int64, error) {
				return nil, 0, errors.New("database error")
			},
			wantStatusCode: http.StatusInternalServerError,
			wantCount:      0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &mockAlertsService{listAlertsFn: tt.mockFn}
			h := newTestAlertsHandler(svc)
			router := setupTestRouter(h)

			req := httptest.NewRequest(http.MethodGet, "/api/tenants/"+tt.tenantID+"/alerts"+tt.queryParams, nil)
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatusCode {
				t.Errorf("handleListAlerts() status = %d, want %d", rec.Code, tt.wantStatusCode)
			}

			if tt.wantStatusCode == http.StatusOK {
				var resp map[string]interface{}
				if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
					t.Fatalf("Failed to decode response: %v", err)
				}
				if alerts, ok := resp["alerts"].([]interface{}); ok {
					if len(alerts) != tt.wantCount {
						t.Errorf("handleListAlerts() count = %d, want %d", len(alerts), tt.wantCount)
					}
				}
			}
		})
	}
}

func TestHandleListAlerts_RealmPolicy(t *testing.T) {
	tests := []struct {
		name       string
		rbac       *mockRBACChecker
		query      string
		wantStatus int
		wantRealm  string
		wantRealms []string
		wantCalled bool
	}{
		{
			name:       "caller with no policy row queries every realm",
			rbac:       &mockRBACChecker{},
			wantStatus: http.StatusOK,
			wantCalled: true,
		},
		{
			name:       "admin queries every realm despite a restricting policy",
			rbac:       &mockRBACChecker{isAdmin: true, policies: policyFor("tenant-1", "realmB")},
			wantStatus: http.StatusOK,
			wantCalled: true,
		},
		{
			name:       "policy restricted to one realm narrows an unfiltered query",
			rbac:       &mockRBACChecker{policies: policyFor("tenant-1", "realmB")},
			wantStatus: http.StatusOK,
			wantRealms: []string{"realmB"},
			wantCalled: true,
		},
		{
			name:       "policy restricted to several realms narrows to all of them",
			rbac:       &mockRBACChecker{policies: policyFor("tenant-1", "realmB", "realmC")},
			wantStatus: http.StatusOK,
			wantRealms: []string{"realmB", "realmC"},
			wantCalled: true,
		},
		{
			name:       "realm outside the policy is refused",
			rbac:       &mockRBACChecker{policies: policyFor("tenant-1", "realmB")},
			query:      "?realm=realmA",
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "realm inside the policy still filters on it",
			rbac:       &mockRBACChecker{policies: policyFor("tenant-1", "realmB")},
			query:      "?realm=realmB",
			wantStatus: http.StatusOK,
			wantRealm:  "realmB",
			wantCalled: true,
		},
		{
			name:       "policy with an empty realm list returns nothing",
			rbac:       &mockRBACChecker{policies: policyFor("tenant-1")},
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotOpts *alerts.ListOptions
			calls := 0
			svc := &mockAlertsService{
				listAlertsFn: func(_ context.Context, tenantID string, opts *alerts.ListOptions) ([]*domain.Alert, int64, error) {
					calls++
					gotOpts = opts
					return []*domain.Alert{{AlertID: "alert-1", TenantID: tenantID}}, 1, nil
				},
			}
			router := setupTestRouter(newTestAlertsHandlerWithRBAC(svc, tt.rbac))

			req := httptest.NewRequest(http.MethodGet, "/api/tenants/tenant-1/alerts"+tt.query, nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body = %s", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if !tt.wantCalled {
				if calls != 0 {
					t.Fatalf("alerts service queried %d times for a refused caller, want 0 (opts = %+v)", calls, gotOpts)
				}
				return
			}
			if gotOpts == nil {
				t.Fatalf("alerts service was never queried, want realms %v", tt.wantRealms)
			}
			if gotOpts.RealmName != tt.wantRealm {
				t.Errorf("RealmName = %q, want %q", gotOpts.RealmName, tt.wantRealm)
			}
			if !reflect.DeepEqual(gotOpts.RealmNames, tt.wantRealms) {
				t.Errorf("RealmNames = %v, want %v", gotOpts.RealmNames, tt.wantRealms)
			}
		})
	}
}

func TestHandleGetAlert(t *testing.T) {
	tests := []struct {
		name           string
		tenantID       string
		alertID        string
		usePathParam   bool // true = /{alertID}, false = ?id={alertID}
		mockFn         func(ctx context.Context, tenantID, alertID string) (*domain.Alert, error)
		wantStatusCode int
	}{
		{
			name:         "get alert by path param",
			tenantID:     "tenant-1",
			alertID:      "alert-123",
			usePathParam: true,
			mockFn: func(ctx context.Context, tenantID, alertID string) (*domain.Alert, error) {
				return &domain.Alert{AlertID: alertID, TenantID: tenantID}, nil
			},
			wantStatusCode: http.StatusOK,
		},
		{
			name:         "get alert by query param",
			tenantID:     "tenant-1",
			alertID:      "alert-123",
			usePathParam: false,
			mockFn: func(ctx context.Context, tenantID, alertID string) (*domain.Alert, error) {
				return &domain.Alert{AlertID: alertID, TenantID: tenantID}, nil
			},
			wantStatusCode: http.StatusOK,
		},
		{
			name:         "alert not found",
			tenantID:     "tenant-1",
			alertID:      "non-existent",
			usePathParam: true,
			mockFn: func(ctx context.Context, tenantID, alertID string) (*domain.Alert, error) {
				return nil, errors.New("not found")
			},
			wantStatusCode: http.StatusNotFound,
		},
		{
			name:           "missing alert_id query param",
			tenantID:       "tenant-1",
			alertID:        "",
			usePathParam:   false,
			mockFn:         nil,
			wantStatusCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &mockAlertsService{getAlertFn: tt.mockFn}
			h := newTestAlertsHandler(svc)
			router := setupTestRouter(h)

			var url string
			if tt.usePathParam {
				url = "/api/tenants/" + tt.tenantID + "/alerts/" + tt.alertID
			} else {
				if tt.alertID != "" {
					url = "/api/tenants/" + tt.tenantID + "/alerts/get?id=" + tt.alertID
				} else {
					url = "/api/tenants/" + tt.tenantID + "/alerts/get"
				}
			}

			req := httptest.NewRequest(http.MethodGet, url, nil)
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatusCode {
				t.Errorf("handleGetAlert() status = %d, want %d, body = %s", rec.Code, tt.wantStatusCode, rec.Body.String())
			}
		})
	}
}

func TestHandleAlertStats(t *testing.T) {
	tests := []struct {
		name           string
		tenantID       string
		queryParams    string
		mockFn         func(ctx context.Context, tenantID string, realmName ...string) (*alerts.Statistics, error)
		wantStatusCode int
	}{
		{
			name:        "successful stats",
			tenantID:    "tenant-1",
			queryParams: "",
			mockFn: func(ctx context.Context, tenantID string, realmName ...string) (*alerts.Statistics, error) {
				return &alerts.Statistics{
					TotalActive: 10,
					BySeverity:  map[string]int{"high": 5, "medium": 5},
				}, nil
			},
			wantStatusCode: http.StatusOK,
		},
		{
			name:        "stats with realm filter",
			tenantID:    "tenant-1",
			queryParams: "?realm=master",
			mockFn: func(ctx context.Context, tenantID string, realmName ...string) (*alerts.Statistics, error) {
				// Note: realm filtering is handled differently in the actual handler
				return &alerts.Statistics{TotalActive: 5}, nil
			},
			wantStatusCode: http.StatusOK,
		},
		{
			name:        "stats error",
			tenantID:    "tenant-1",
			queryParams: "",
			mockFn: func(ctx context.Context, tenantID string, realmName ...string) (*alerts.Statistics, error) {
				return nil, errors.New("database error")
			},
			wantStatusCode: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &mockAlertsService{getStatisticsFn: tt.mockFn}
			h := newTestAlertsHandler(svc)
			router := setupTestRouter(h)

			req := httptest.NewRequest(http.MethodGet, "/api/tenants/"+tt.tenantID+"/alerts/stats"+tt.queryParams, nil)
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatusCode {
				t.Errorf("handleAlertStats() status = %d, want %d", rec.Code, tt.wantStatusCode)
			}
		})
	}
}

func TestHandleAlertStats_RealmPolicy(t *testing.T) {
	tests := []struct {
		name       string
		rbac       *mockRBACChecker
		query      string
		wantStatus int
		wantRealms []string
		wantCalled bool
	}{
		{
			name:       "caller with no policy row counts every realm",
			rbac:       &mockRBACChecker{},
			wantStatus: http.StatusOK,
			wantCalled: true,
		},
		{
			name:       "admin counts every realm despite a restricting policy",
			rbac:       &mockRBACChecker{isAdmin: true, policies: policyFor("tenant-1", "realmB")},
			wantStatus: http.StatusOK,
			wantCalled: true,
		},
		{
			name:       "policy restricted to one realm narrows an unfiltered count",
			rbac:       &mockRBACChecker{policies: policyFor("tenant-1", "realmB")},
			wantStatus: http.StatusOK,
			wantRealms: []string{"realmB"},
			wantCalled: true,
		},
		{
			name:       "policy restricted to several realms counts all of them",
			rbac:       &mockRBACChecker{policies: policyFor("tenant-1", "realmB", "realmC")},
			wantStatus: http.StatusOK,
			wantRealms: []string{"realmB", "realmC"},
			wantCalled: true,
		},
		{
			name:       "realm outside the policy is refused",
			rbac:       &mockRBACChecker{policies: policyFor("tenant-1", "realmB")},
			query:      "?realm_name=realmA",
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "realm inside the policy still counts only it",
			rbac:       &mockRBACChecker{policies: policyFor("tenant-1", "realmB")},
			query:      "?realm_name=realmB",
			wantStatus: http.StatusOK,
			wantRealms: []string{"realmB"},
			wantCalled: true,
		},
		{
			name:       "policy with an empty realm list counts nothing",
			rbac:       &mockRBACChecker{policies: policyFor("tenant-1")},
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotRealms []string
			calls := 0
			svc := &mockAlertsService{
				getStatisticsFn: func(_ context.Context, _ string, realmName ...string) (*alerts.Statistics, error) {
					calls++
					gotRealms = realmName
					return &alerts.Statistics{TotalActive: 3}, nil
				},
			}
			router := setupTestRouter(newTestAlertsHandlerWithRBAC(svc, tt.rbac))

			req := httptest.NewRequest(http.MethodGet, "/api/tenants/tenant-1/alerts/stats"+tt.query, nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body = %s", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if !tt.wantCalled {
				if calls != 0 {
					t.Fatalf("statistics queried %d times for a refused caller, want 0 (realms = %v)", calls, gotRealms)
				}
				if tt.wantStatus == http.StatusOK {
					var stats alerts.Statistics
					if err := json.Unmarshal(rec.Body.Bytes(), &stats); err != nil {
						t.Fatalf("decode: %v; body = %s", err, rec.Body.String())
					}
					if stats.TotalActive != 0 {
						t.Errorf("total_active = %d, want 0", stats.TotalActive)
					}
				}
				return
			}
			if calls == 0 {
				t.Fatalf("statistics were never queried, want realms %v", tt.wantRealms)
			}
			if !reflect.DeepEqual(gotRealms, tt.wantRealms) {
				t.Errorf("realms = %v, want %v", gotRealms, tt.wantRealms)
			}
		})
	}
}

func TestHandleUpdateAlertStatus(t *testing.T) {
	tests := []struct {
		name           string
		tenantID       string
		alertID        string
		usePathParam   bool
		requestBody    map[string]interface{}
		mockGetFn      func(ctx context.Context, tenantID, alertID string) (*domain.Alert, error)
		mockUpdateFn   func(ctx context.Context, tenantID, alertID string, status domain.AlertStatus, acknowledgedBy string) (*domain.Alert, error)
		wantStatusCode int
	}{
		{
			name:         "successful status update via path",
			tenantID:     "tenant-1",
			alertID:      "alert-123",
			usePathParam: true,
			requestBody:  map[string]interface{}{"status": "acknowledged"},
			mockGetFn: func(ctx context.Context, tenantID, alertID string) (*domain.Alert, error) {
				return &domain.Alert{AlertID: alertID, TenantID: tenantID, Status: domain.AlertStatusActive}, nil
			},
			mockUpdateFn: func(ctx context.Context, tenantID, alertID string, status domain.AlertStatus, acknowledgedBy string) (*domain.Alert, error) {
				return &domain.Alert{AlertID: alertID, TenantID: tenantID, Status: status}, nil
			},
			wantStatusCode: http.StatusOK,
		},
		{
			name:         "successful status update via query",
			tenantID:     "tenant-1",
			alertID:      "alert-123",
			usePathParam: false,
			requestBody:  map[string]interface{}{"status": "resolved"},
			mockGetFn: func(ctx context.Context, tenantID, alertID string) (*domain.Alert, error) {
				return &domain.Alert{AlertID: alertID, TenantID: tenantID, Status: domain.AlertStatusActive}, nil
			},
			mockUpdateFn: func(ctx context.Context, tenantID, alertID string, status domain.AlertStatus, acknowledgedBy string) (*domain.Alert, error) {
				return &domain.Alert{AlertID: alertID, TenantID: tenantID, Status: status}, nil
			},
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "missing alert_id query param",
			tenantID:       "tenant-1",
			alertID:        "",
			usePathParam:   false,
			requestBody:    map[string]interface{}{"status": "acknowledged"},
			wantStatusCode: http.StatusBadRequest,
		},
		{
			name:         "invalid status",
			tenantID:     "tenant-1",
			alertID:      "alert-123",
			usePathParam: true,
			requestBody:  map[string]interface{}{"status": "invalid-status"},
			mockGetFn: func(ctx context.Context, tenantID, alertID string) (*domain.Alert, error) {
				return &domain.Alert{AlertID: alertID, TenantID: tenantID}, nil
			},
			mockUpdateFn: func(ctx context.Context, tenantID, alertID string, status domain.AlertStatus, acknowledgedBy string) (*domain.Alert, error) {
				return nil, errors.New("invalid status")
			},
			wantStatusCode: http.StatusBadRequest,
		},
		{
			name:         "update error",
			tenantID:     "tenant-1",
			alertID:      "alert-123",
			usePathParam: true,
			requestBody:  map[string]interface{}{"status": "acknowledged"},
			mockGetFn: func(ctx context.Context, tenantID, alertID string) (*domain.Alert, error) {
				return &domain.Alert{AlertID: alertID, TenantID: tenantID}, nil
			},
			mockUpdateFn: func(ctx context.Context, tenantID, alertID string, status domain.AlertStatus, acknowledgedBy string) (*domain.Alert, error) {
				return nil, errors.New("database error")
			},
			wantStatusCode: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &mockAlertsService{
				getAlertFn:     tt.mockGetFn,
				updateStatusFn: tt.mockUpdateFn,
			}
			h := newTestAlertsHandler(svc)
			router := setupTestRouter(h)

			body, _ := json.Marshal(tt.requestBody)

			var url string
			var method string
			if tt.usePathParam {
				url = "/api/tenants/" + tt.tenantID + "/alerts/" + tt.alertID + "/status"
				method = http.MethodPut
			} else {
				if tt.alertID != "" {
					url = "/api/tenants/" + tt.tenantID + "/alerts/update-status?id=" + tt.alertID
				} else {
					url = "/api/tenants/" + tt.tenantID + "/alerts/update-status"
				}
				method = http.MethodPost
			}

			req := httptest.NewRequest(method, url, bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatusCode {
				t.Errorf("handleUpdateAlertStatus() status = %d, want %d, body = %s", rec.Code, tt.wantStatusCode, rec.Body.String())
			}
		})
	}
}

func TestHandleResolveAlert(t *testing.T) {
	tests := []struct {
		name           string
		tenantID       string
		alertID        string
		usePathParam   bool
		mockGetFn      func(ctx context.Context, tenantID, alertID string) (*domain.Alert, error)
		mockResolveFn  func(ctx context.Context, tenantID, alertID string) (*domain.Alert, error)
		wantStatusCode int
	}{
		{
			name:         "successful resolve via path",
			tenantID:     "tenant-1",
			alertID:      "alert-123",
			usePathParam: true,
			mockGetFn: func(ctx context.Context, tenantID, alertID string) (*domain.Alert, error) {
				return &domain.Alert{AlertID: alertID, TenantID: tenantID, Status: domain.AlertStatusActive}, nil
			},
			mockResolveFn: func(ctx context.Context, tenantID, alertID string) (*domain.Alert, error) {
				return &domain.Alert{AlertID: alertID, TenantID: tenantID, Status: domain.AlertStatusResolved}, nil
			},
			wantStatusCode: http.StatusOK,
		},
		{
			name:         "successful resolve via query",
			tenantID:     "tenant-1",
			alertID:      "alert-123",
			usePathParam: false,
			mockGetFn: func(ctx context.Context, tenantID, alertID string) (*domain.Alert, error) {
				return &domain.Alert{AlertID: alertID, TenantID: tenantID, Status: domain.AlertStatusActive}, nil
			},
			mockResolveFn: func(ctx context.Context, tenantID, alertID string) (*domain.Alert, error) {
				return &domain.Alert{AlertID: alertID, TenantID: tenantID, Status: domain.AlertStatusResolved}, nil
			},
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "missing alert_id query param",
			tenantID:       "tenant-1",
			alertID:        "",
			usePathParam:   false,
			wantStatusCode: http.StatusBadRequest,
		},
		{
			name:         "resolve error",
			tenantID:     "tenant-1",
			alertID:      "alert-123",
			usePathParam: true,
			mockGetFn: func(ctx context.Context, tenantID, alertID string) (*domain.Alert, error) {
				return &domain.Alert{AlertID: alertID, TenantID: tenantID}, nil
			},
			mockResolveFn: func(ctx context.Context, tenantID, alertID string) (*domain.Alert, error) {
				return nil, errors.New("resolve failed")
			},
			wantStatusCode: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &mockAlertsService{
				getAlertFn:     tt.mockGetFn,
				resolveAlertFn: tt.mockResolveFn,
			}
			h := newTestAlertsHandler(svc)
			router := setupTestRouter(h)

			var url string
			if tt.usePathParam {
				url = "/api/tenants/" + tt.tenantID + "/alerts/" + tt.alertID + "/resolve"
			} else {
				if tt.alertID != "" {
					url = "/api/tenants/" + tt.tenantID + "/alerts/resolve?id=" + tt.alertID
				} else {
					url = "/api/tenants/" + tt.tenantID + "/alerts/resolve"
				}
			}

			req := httptest.NewRequest(http.MethodPost, url, nil)
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatusCode {
				t.Errorf("handleResolveAlert() status = %d, want %d, body = %s", rec.Code, tt.wantStatusCode, rec.Body.String())
			}
		})
	}
}

func TestHandleDeleteAlert(t *testing.T) {
	tests := []struct {
		name           string
		tenantID       string
		alertID        string
		mockFn         func(ctx context.Context, tenantID, alertID string) error
		wantStatusCode int
	}{
		{
			name:     "successful delete",
			tenantID: "tenant-1",
			alertID:  "alert-123",
			mockFn: func(ctx context.Context, tenantID, alertID string) error {
				return nil
			},
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "missing alert_id",
			tenantID:       "tenant-1",
			alertID:        "",
			mockFn:         nil,
			wantStatusCode: http.StatusBadRequest,
		},
		{
			name:     "delete error",
			tenantID: "tenant-1",
			alertID:  "alert-123",
			mockFn: func(ctx context.Context, tenantID, alertID string) error {
				return errors.New("delete failed")
			},
			wantStatusCode: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Delete resolves the alert first so its realm can be checked.
			svc := &mockAlertsService{
				deleteAlertFn: tt.mockFn,
				getAlertFn: func(_ context.Context, tenantID, alertID string) (*domain.Alert, error) {
					return &domain.Alert{AlertID: alertID, TenantID: tenantID, RealmName: "master"}, nil
				},
			}
			h := newTestAlertsHandler(svc)
			router := setupTestRouter(h)

			var url string
			if tt.alertID != "" {
				url = "/api/tenants/" + tt.tenantID + "/alerts/delete?id=" + tt.alertID
			} else {
				url = "/api/tenants/" + tt.tenantID + "/alerts/delete"
			}

			req := httptest.NewRequest(http.MethodDelete, url, nil)
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatusCode {
				t.Errorf("handleDeleteAlert() status = %d, want %d, body = %s", rec.Code, tt.wantStatusCode, rec.Body.String())
			}
		})
	}
}

func TestHandleListAlertsByRealm(t *testing.T) {
	tests := []struct {
		name           string
		tenantID       string
		queryParams    string
		mockFn         func(ctx context.Context, tenantID, realmName string, opts *alerts.ListOptions) ([]*domain.Alert, error)
		wantStatusCode int
	}{
		{
			name:        "successful list by realm",
			tenantID:    "tenant-1",
			queryParams: "?realm=master",
			mockFn: func(ctx context.Context, tenantID, realmName string, opts *alerts.ListOptions) ([]*domain.Alert, error) {
				if realmName != "master" {
					t.Errorf("Expected realm=master, got %s", realmName)
				}
				return []*domain.Alert{
					{AlertID: "alert-1", RealmName: realmName},
				}, nil
			},
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "missing realm param",
			tenantID:       "tenant-1",
			queryParams:    "",
			mockFn:         nil,
			wantStatusCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &mockAlertsService{listByRealmFn: tt.mockFn}
			h := newTestAlertsHandler(svc)
			router := setupTestRouter(h)

			req := httptest.NewRequest(http.MethodGet, "/api/tenants/"+tt.tenantID+"/alerts/by-realm"+tt.queryParams, nil)
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatusCode {
				t.Errorf("handleListAlertsByRealm() status = %d, want %d", rec.Code, tt.wantStatusCode)
			}
		})
	}
}

func TestHandleListAlertsByRealm_RealmPolicy(t *testing.T) {
	tests := []struct {
		name       string
		rbac       *mockRBACChecker
		wantStatus int
	}{
		{
			name:       "caller with no policy row may read the realm",
			rbac:       &mockRBACChecker{},
			wantStatus: http.StatusOK,
		},
		{
			name:       "admin may read the realm despite a restricting policy",
			rbac:       &mockRBACChecker{isAdmin: true, policies: policyFor("tenant-1", "realmB")},
			wantStatus: http.StatusOK,
		},
		{
			name:       "caller restricted to the realm may read it",
			rbac:       &mockRBACChecker{policies: policyFor("tenant-1", "realmA")},
			wantStatus: http.StatusOK,
		},
		{
			name:       "realm outside the policy is refused",
			rbac:       &mockRBACChecker{policies: policyFor("tenant-1", "realmB")},
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "policy with an empty realm list is refused",
			rbac:       &mockRBACChecker{policies: policyFor("tenant-1")},
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "a failed policy lookup is refused rather than waved through",
			rbac:       &mockRBACChecker{policiesErr: errors.New("policy lookup failed")},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			svc := &mockAlertsService{
				listByRealmFn: func(_ context.Context, _, realmName string, _ *alerts.ListOptions) ([]*domain.Alert, error) {
					calls++
					return []*domain.Alert{{AlertID: "alert-1", RealmName: realmName}}, nil
				},
			}
			router := setupTestRouter(newTestAlertsHandlerWithRBAC(svc, tt.rbac))

			req := httptest.NewRequest(http.MethodGet, "/api/tenants/tenant-1/alerts/by-realm?realm=realmA", nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body = %s", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if tt.wantStatus != http.StatusOK && calls != 0 {
				t.Errorf("alerts service queried %d times for a refused caller, want 0", calls)
			}
		})
	}
}

func TestHandleListAlertsByRealm_RequiresAuthenticatedCaller(t *testing.T) {
	// The route's own auth middleware always seeds a caller, so the handler is
	// driven directly with a context that carries none.
	calls := 0
	svc := &mockAlertsService{
		listByRealmFn: func(_ context.Context, _, _ string, _ *alerts.ListOptions) ([]*domain.Alert, error) {
			calls++
			return nil, nil
		},
	}
	h := newTestAlertsHandlerWithRBAC(svc, &mockRBACChecker{})

	req := httptest.NewRequest(http.MethodGet, "/alerts/by-realm?realm=realmA", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("tenantID", "tenant-1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rec := httptest.NewRecorder()
	h.handleListAlertsByRealm(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}
	if calls != 0 {
		t.Errorf("alerts service queried %d times without an authenticated caller, want 0", calls)
	}
}

func TestRBACPermissionDenied(t *testing.T) {
	// Test that RBAC permission denial returns 403
	svc := &mockAlertsService{
		listAlertsFn: func(ctx context.Context, tenantID string, opts *alerts.ListOptions) ([]*domain.Alert, int64, error) {
			return []*domain.Alert{}, 0, nil
		},
	}

	rbacChecker := &mockRBACChecker{
		hasPermissionFn: func(ctx context.Context, userID uint, permission string, tenantID *string) (bool, error) {
			return false, nil // Deny all
		},
	}

	rbacMw := NewRBACMiddleware(rbacChecker, newTestLogger())
	h := &AlertsHandlers{
		service:        svc,
		rbacService:    rbacChecker,
		log:            newTestLogger(),
		authMiddleware: testAuthMiddleware,
		rbacMiddleware: rbacMw,
	}

	router := setupTestRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/api/tenants/tenant-1/alerts", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("Expected 403 Forbidden when RBAC denies permission, got %d", rec.Code)
	}
}

// alertMutationRecorder counts the service calls that change an alert.
type alertMutationRecorder struct {
	updates  int
	resolves int
	deletes  int
}

func (m *alertMutationRecorder) total() int { return m.updates + m.resolves + m.deletes }

// byIDAlertsService answers GetAlert with alert when the requested ID matches
// it and with nothing otherwise.
func byIDAlertsService(alert *domain.Alert, rec *alertMutationRecorder) *mockAlertsService {
	return &mockAlertsService{
		getAlertFn: func(_ context.Context, _, alertID string) (*domain.Alert, error) {
			if alert == nil || alertID != alert.AlertID {
				return nil, nil
			}
			return alert, nil
		},
		updateStatusFn: func(context.Context, string, string, domain.AlertStatus, string) (*domain.Alert, error) {
			rec.updates++
			return alert, nil
		},
		resolveAlertFn: func(context.Context, string, string) (*domain.Alert, error) {
			rec.resolves++
			return alert, nil
		},
		deleteAlertFn: func(context.Context, string, string) error {
			rec.deletes++
			return nil
		},
	}
}

func doByIDAlertRequest(svc *mockAlertsService, rbacSvc RBACChecker, route byIDRoute, alertID string) *httptest.ResponseRecorder {
	router := setupTestRouter(newTestAlertsHandlerWithRBAC(svc, rbacSvc))
	req := httptest.NewRequest(route.method, "/api/tenants/tenant-a"+route.path(alertID),
		bytes.NewBufferString(`{"status":"acknowledged"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// Drives all seven by-ID routes through the real router against stubs, so the
// realm gate has coverage that runs under -short.
func TestAlertsHandlers_ByIDRoutesRefuseAnAlertOutsideTheCallersRealms(t *testing.T) {
	const (
		deniedID  = "alert-in-realmA"
		missingID = "alert-that-does-not-exist"
	)
	crossRealmAlert := &domain.Alert{
		ID:        7,
		AlertID:   deniedID,
		TenantID:  "tenant-a",
		RealmName: "realmA",
		Title:     "another realm's alert",
	}

	for _, route := range byIDRoutes() {
		t.Run(route.name, func(t *testing.T) {
			restricted := &mockRBACChecker{policies: policyFor("tenant-a", "realmB")}

			deniedRec := &alertMutationRecorder{}
			denied := doByIDAlertRequest(byIDAlertsService(crossRealmAlert, deniedRec), restricted, route, deniedID)
			if denied.Code != http.StatusNotFound {
				t.Errorf("status = %d, want %d; body = %s", denied.Code, http.StatusNotFound, denied.Body.String())
			}
			if deniedRec.total() != 0 {
				t.Errorf("alert in realmA was mutated by a caller restricted to realmB: %+v", *deniedRec)
			}

			missingRec := &alertMutationRecorder{}
			missing := doByIDAlertRequest(byIDAlertsService(nil, missingRec), restricted, route, missingID)
			if missing.Code != denied.Code || missing.Body.String() != denied.Body.String() {
				t.Errorf("an alert in another realm answers %d %s but a missing one answers %d %s; the difference is an oracle for which alert IDs exist",
					denied.Code, denied.Body.String(), missing.Code, missing.Body.String())
			}

			controlRec := &alertMutationRecorder{}
			allowed := doByIDAlertRequest(byIDAlertsService(crossRealmAlert, controlRec), &mockRBACChecker{}, route, deniedID)
			if allowed.Code != http.StatusOK {
				t.Fatalf("unrestricted caller: status = %d, want %d; body = %s", allowed.Code, http.StatusOK, allowed.Body.String())
			}
			if route.mutates && controlRec.total() == 0 {
				t.Error("unrestricted caller's mutation never reached the service; the 404 above proves nothing")
			}
		})
	}
}

func TestAlertsHandlers_ByIDRead_ResolvesAdminOnce(t *testing.T) {
	tests := []struct {
		name         string
		rbac         *mockRBACChecker
		wantMetadata string
	}{
		{
			name:         "an administrator keeps the internal fields",
			rbac:         &mockRBACChecker{isAdmin: true},
			wantMetadata: `{"rule":"internal"}`,
		},
		{
			name: "anyone else does not",
			rbac: &mockRBACChecker{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			alert := &domain.Alert{
				ID:        3,
				AlertID:   "alert-1",
				TenantID:  "tenant-a",
				RealmName: "realmB",
				Metadata:  `{"rule":"internal"}`,
				CheckType: "keycloak-config",
			}
			svc := byIDAlertsService(alert, &alertMutationRecorder{})
			router := setupTestRouter(newTestAlertsHandlerWithRBAC(svc, tt.rbac))

			req := httptest.NewRequest(http.MethodGet, "/api/tenants/tenant-a/alerts/alert-1", nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
			}

			var got domain.Alert
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatalf("decode: %v; body = %s", err, rec.Body.String())
			}
			if got.Metadata != tt.wantMetadata {
				t.Errorf("metadata = %q, want %q", got.Metadata, tt.wantMetadata)
			}
			if tt.rbac.isAdminCalls != 1 {
				t.Errorf("IsAdmin called %d times, want 1", tt.rbac.isAdminCalls)
			}
		})
	}
}

func TestSanitizeAlertStripsInternalFieldsForNonAdmin(t *testing.T) {
	ruleID := "rule-42"
	eventID := "event-99"
	alert := &domain.Alert{
		AlertID:        "alert-1",
		Title:          "Brute force detected",
		Description:    "Repeated login failures",
		Recommendation: "Lock the account",
		RealmName:      "prod",
		Metadata:       `{"internal":"detail"}`,
		CheckType:      "internal-check",
		RuleID:         &ruleID,
		EventID:        &eventID,
	}
	h := &AlertsHandlers{log: newTestLogger()}

	got := h.sanitizeAlert(alert, false)
	if got.Metadata != "" || got.CheckType != "" || got.RuleID != nil || got.EventID != nil {
		t.Fatalf("sanitized alert retains internal fields: %+v", got)
	}

	if h.sanitizeAlert(alert, true) != alert {
		t.Fatal("admin path must return the alert unchanged")
	}
}
