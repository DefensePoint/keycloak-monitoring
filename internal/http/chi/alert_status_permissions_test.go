package chi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/rbac"
)

// Every status the endpoint accepts used to need only alerts:acknowledge, so
// that permission could also ignore an alert or reopen a closed one.
func TestUpdateAlertStatus_PermissionPerStatus(t *testing.T) {
	ack := rbac.PermissionAlertsAcknowledge
	res := rbac.PermissionAlertsResolve
	for _, tc := range []struct {
		status  string
		granted []string
		allowed bool
	}{
		{"acknowledged", []string{ack}, true},
		{"acknowledged", []string{res}, false},
		{"acknowledged", nil, false},
		{"ignored", []string{ack}, false},
		{"ignored", []string{res}, true},
		{"active", []string{ack}, false},
		{"active", []string{res}, true},
		{"resolved", []string{ack}, false},
		{"resolved", []string{res}, true},
		{"ignored", []string{ack, res}, true},
	} {
		for _, route := range []string{"put-by-id", "post-update-status"} {
			t.Run(tc.status+"/"+route, func(t *testing.T) {
				granted := map[string]bool{rbac.PermissionTenantsRead: true, rbac.PermissionAlertsRead: true}
				for _, p := range tc.granted {
					granted[p] = true
				}
				checker := &mockRBACChecker{
					isAdmin: true, // realm scope only; permissions come from hasPermissionFn
					hasPermissionFn: func(ctx context.Context, userID uint, permission string, tenantID *string) (bool, error) {
						return granted[permission], nil
					},
				}
				changed := false
				svc := &mockAlertsService{
					getAlertFn: func(_ context.Context, tenantID, alertID string) (*domain.Alert, error) {
						return &domain.Alert{AlertID: alertID, TenantID: tenantID, Status: domain.AlertStatusActive}, nil
					},
					updateStatusFn: func(_ context.Context, tenantID, alertID string, status domain.AlertStatus, by string) (*domain.Alert, error) {
						changed = true
						return &domain.Alert{AlertID: alertID, TenantID: tenantID, Status: status}, nil
					},
					resolveAlertFn: func(_ context.Context, tenantID, alertID string) (*domain.Alert, error) {
						changed = true
						return &domain.Alert{AlertID: alertID, TenantID: tenantID, Status: domain.AlertStatusResolved}, nil
					},
				}
				router := setupTestRouter(newTestAlertsHandlerWithRBAC(svc, checker))

				body, _ := json.Marshal(map[string]string{"status": tc.status})
				method, url := http.MethodPut, "/api/tenants/t1/alerts/a1/status"
				if route == "post-update-status" {
					method, url = http.MethodPost, "/api/tenants/t1/alerts/update-status?id=a1"
				}
				req := httptest.NewRequest(method, url, bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, req)

				if tc.allowed && (rec.Code != http.StatusOK || !changed) {
					t.Fatalf("granted %v: status %d changed=%v, want 200 and a change (%s)", tc.granted, rec.Code, changed, rec.Body.String())
				}
				if !tc.allowed && (rec.Code != http.StatusForbidden || changed) {
					t.Fatalf("granted %v: status %d changed=%v, want 403 and no change", tc.granted, rec.Code, changed)
				}
			})
		}
	}
}
