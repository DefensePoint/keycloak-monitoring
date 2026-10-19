package chi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/rbac"
)

// Resolving through the status endpoints used to be a bare status write: no
// resolved_at, no "resolved" operator action, and no resolution notice, so the
// GitLab issue stayed open and Slack never heard the alert was over. It must
// now behave exactly like POST .../resolve.

type recordingNotifier struct {
	mu   sync.Mutex
	sent []*domain.Alert
	done chan struct{}
}

func newRecordingNotifier() *recordingNotifier {
	return &recordingNotifier{done: make(chan struct{}, 8)}
}

func (n *recordingNotifier) NotifyAlertResolution(_ context.Context, alert *domain.Alert) error {
	n.mu.Lock()
	n.sent = append(n.sent, alert)
	n.mu.Unlock()
	n.done <- struct{}{}
	return nil
}

// waitSent returns how many notices arrived within the window. The handler
// sends asynchronously, so a missing notice can only be detected by waiting.
func (n *recordingNotifier) waitSent(window time.Duration) int {
	deadline := time.After(window)
	for {
		select {
		case <-n.done:
		case <-deadline:
			n.mu.Lock()
			defer n.mu.Unlock()
			return len(n.sent)
		}
	}
}

type statusCalls struct {
	resolve, update int
}

func statusEndpointService(calls *statusCalls) *mockAlertsService {
	open := &domain.Alert{TenantID: "tenant-a", AlertID: "alert-1", Status: domain.AlertStatusActive, Title: "Tenant down"}
	return &mockAlertsService{
		getAlertFn: func(context.Context, string, string) (*domain.Alert, error) { return open, nil },
		resolveAlertFn: func(_ context.Context, tenantID, alertID string) (*domain.Alert, error) {
			calls.resolve++
			now := time.Now()
			return &domain.Alert{TenantID: tenantID, AlertID: alertID, Status: domain.AlertStatusResolved, Title: open.Title, ResolvedAt: &now}, nil
		},
		updateStatusFn: func(_ context.Context, tenantID, alertID string, status domain.AlertStatus, _ string) (*domain.Alert, error) {
			calls.update++
			return &domain.Alert{TenantID: tenantID, AlertID: alertID, Status: status}, nil
		},
	}
}

func statusRequest(route, status string) *http.Request {
	body := strings.NewReader(`{"status":"` + status + `"}`)
	var req *http.Request
	if route == "update-status" {
		req = httptest.NewRequest(http.MethodPost, "/api/tenants/tenant-a/alerts/update-status?id=alert-1", body)
	} else {
		req = httptest.NewRequest(http.MethodPut, "/api/tenants/tenant-a/alerts/alert-1/status", body)
	}
	req.Header.Set("Content-Type", "application/json")
	return req
}

func statusHandler(svc *mockAlertsService, checker *mockRBACChecker, notifier *recordingNotifier) http.Handler {
	h := NewAlertsHandlers(svc, checker, &mockOperatorService{}, notifier, newTestLogger(), testAuthMiddleware, NewRBACMiddleware(checker, newTestLogger()))
	return setupTestRouter(h)
}

func TestStatusEndpoints_ResolvedIsTheResolveAction(t *testing.T) {
	for _, route := range []string{"update-status", "{id}/status"} {
		t.Run(route, func(t *testing.T) {
			calls := &statusCalls{}
			notifier := newRecordingNotifier()
			rec := httptest.NewRecorder()
			statusHandler(statusEndpointService(calls), &mockRBACChecker{}, notifier).ServeHTTP(rec, statusRequest(route, "resolved"))

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
			}
			if calls.resolve != 1 || calls.update != 0 {
				t.Errorf("ResolveAlert called %d time(s), UpdateStatus %d; want the resolve path only", calls.resolve, calls.update)
			}
			if sent := notifier.waitSent(500 * time.Millisecond); sent != 1 {
				t.Errorf("resolution notices sent = %d, want 1", sent)
			}
		})
	}
}

func TestStatusEndpoints_ResolvedNeedsTheResolvePermission(t *testing.T) {
	for _, route := range []string{"update-status", "{id}/status"} {
		t.Run(route, func(t *testing.T) {
			calls := &statusCalls{}
			notifier := newRecordingNotifier()
			ackOnly := &mockRBACChecker{hasPermissionFn: func(_ context.Context, _ uint, permission string, _ *string) (bool, error) {
				return permission != rbac.PermissionAlertsResolve, nil
			}}
			rec := httptest.NewRecorder()
			statusHandler(statusEndpointService(calls), ackOnly, notifier).ServeHTTP(rec, statusRequest(route, "resolved"))

			if rec.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want 403 for a caller without alerts:resolve; body = %s", rec.Code, rec.Body.String())
			}
			if calls.resolve != 0 || calls.update != 0 {
				t.Errorf("the alert was changed (resolve %d, update %d) for a caller without alerts:resolve", calls.resolve, calls.update)
			}
			if sent := notifier.waitSent(200 * time.Millisecond); sent != 0 {
				t.Errorf("resolution notices sent = %d, want 0", sent)
			}
		})
	}
}

func TestStatusEndpoints_OtherStatusesStayPlainWrites(t *testing.T) {
	for _, status := range []string{"acknowledged", "ignored", "active"} {
		t.Run(status, func(t *testing.T) {
			calls := &statusCalls{}
			notifier := newRecordingNotifier()
			rec := httptest.NewRecorder()
			statusHandler(statusEndpointService(calls), &mockRBACChecker{}, notifier).ServeHTTP(rec, statusRequest("{id}/status", status))

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
			}
			if calls.update != 1 || calls.resolve != 0 {
				t.Errorf("UpdateStatus called %d time(s), ResolveAlert %d; want the plain status write", calls.update, calls.resolve)
			}
			if sent := notifier.waitSent(200 * time.Millisecond); sent != 0 {
				t.Errorf("resolution notices sent = %d for status %s, want 0", sent, status)
			}
		})
	}
}

// The resolution notice is sent from a goroutine. A resolve that yields no
// alert must not reach it: a nil there panics outside any request recovery and
// takes the whole process down.
func TestResolve_NoAlertBackDoesNotCrashOrNotify(t *testing.T) {
	notifier := newRecordingNotifier()
	open := &domain.Alert{TenantID: "tenant-a", AlertID: "alert-1", Status: domain.AlertStatusActive}
	svc := &mockAlertsService{
		getAlertFn:     func(context.Context, string, string) (*domain.Alert, error) { return open, nil },
		resolveAlertFn: func(context.Context, string, string) (*domain.Alert, error) { return nil, nil },
	}
	rec := httptest.NewRecorder()
	statusHandler(svc, &mockRBACChecker{}, notifier).ServeHTTP(rec, statusRequest("{id}/status", "resolved"))

	if sent := notifier.waitSent(300 * time.Millisecond); sent != 0 {
		t.Errorf("resolution notices sent = %d for a resolve that returned no alert, want 0", sent)
	}
}
