package chi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

// eventFilterEndpoints runs every filter case against both endpoints that
// share the event filter parameters.
var eventFilterEndpoints = []struct {
	name   string
	path   string
	handle func(h *SystemHandlers, w http.ResponseWriter, r *http.Request)
}{
	{"events", "/events", (*SystemHandlers).handleEvents},
	{"stats", "/stats", (*SystemHandlers).handleStats},
}

func newFilterTestHandlers(repo *stubEventRepo) *SystemHandlers {
	return &SystemHandlers{
		eventRepo:   repo,
		realmLister: &mockRealmLister{realms: realms("realmA")},
		rbacService: &mockRBACChecker{},
		log:         newTestLogger(),
	}
}

func TestSystemHandlers_EventFilters_PassedToRepository(t *testing.T) {
	for _, ep := range eventFilterEndpoints {
		t.Run(ep.name, func(t *testing.T) {
			repo := &stubEventRepo{events: []*domain.Event{{EventID: "e1"}}, total: 1}
			h := newFilterTestHandlers(repo)

			rec := httptest.NewRecorder()
			ep.handle(h, rec, newSystemRequest(ep.path+"?type=%20LOGIN_ERROR%20&severity=error&min_risk=3&user_id=%20kc-user-1%20", "tenant-a"))

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
			}
			if repo.lastOpts == nil {
				t.Fatal("repository was never queried")
			}
			if repo.lastOpts.Type != "LOGIN_ERROR" {
				t.Errorf("Type = %q, want %q (trimmed)", repo.lastOpts.Type, "LOGIN_ERROR")
			}
			if repo.lastOpts.Severity != "error" {
				t.Errorf("Severity = %q, want %q", repo.lastOpts.Severity, "error")
			}
			if repo.lastOpts.MinRisk == nil || *repo.lastOpts.MinRisk != 3 {
				t.Errorf("MinRisk = %v, want 3", repo.lastOpts.MinRisk)
			}
			if repo.lastOpts.UserID != "kc-user-1" {
				t.Errorf("UserID = %q, want %q (trimmed)", repo.lastOpts.UserID, "kc-user-1")
			}
		})
	}
}

func TestSystemHandlers_EventFilters_OmittedLeavesOptionsUnset(t *testing.T) {
	for _, ep := range eventFilterEndpoints {
		t.Run(ep.name, func(t *testing.T) {
			repo := &stubEventRepo{total: 1}
			h := newFilterTestHandlers(repo)

			rec := httptest.NewRecorder()
			ep.handle(h, rec, newSystemRequest(ep.path, "tenant-a"))

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
			}
			if repo.lastOpts == nil {
				t.Fatal("repository was never queried")
			}
			if repo.lastOpts.Type != "" || repo.lastOpts.Severity != "" || repo.lastOpts.MinRisk != nil || repo.lastOpts.UserID != "" {
				t.Errorf("filters = (%q, %q, %v, %q), want all unset",
					repo.lastOpts.Type, repo.lastOpts.Severity, repo.lastOpts.MinRisk, repo.lastOpts.UserID)
			}
		})
	}
}

func TestSystemHandlers_EventFilters_InvalidRejected(t *testing.T) {
	cases := []struct {
		name  string
		query string
	}{
		{"unknown severity", "severity=fatal"},
		{"severity is case-sensitive", "severity=ERROR"},
		{"non-numeric min_risk", "min_risk=high"},
		{"negative min_risk", "min_risk=-1"},
		{"fractional min_risk", "min_risk=2.5"},
		{"overlong type", "type=" + strings.Repeat("A", maxEventTypeFilterLen+1)},
		{"overlong user_id", "user_id=" + strings.Repeat("u", maxEventUserIDFilterLen+1)},
	}
	for _, ep := range eventFilterEndpoints {
		for _, tc := range cases {
			t.Run(ep.name+"/"+tc.name, func(t *testing.T) {
				repo := &stubEventRepo{total: 1}
				h := newFilterTestHandlers(repo)

				rec := httptest.NewRecorder()
				ep.handle(h, rec, newSystemRequest(ep.path+"?"+tc.query, "tenant-a"))

				if rec.Code != http.StatusBadRequest {
					t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
				}
				if repo.calls != 0 {
					t.Errorf("repository queried %d times for an invalid filter, want 0", repo.calls)
				}
			})
		}
	}
}
