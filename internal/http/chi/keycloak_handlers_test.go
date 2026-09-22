package chi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/keycloak"
	"github.com/DefensePoint/keycloak-monitoring/pkg/keycloakadmin"
)

// stubKeycloakService is a keycloak.Service whose reads return canned per-realm
// data, so a test can assert exactly which realms a response discloses.
type stubKeycloakService struct {
	realmMetrics map[string]*domain.KeycloakMetrics
	dashboard    *keycloak.Dashboard
	realms       []*domain.KeycloakRealmInfo
	amfaRealms   []string
}

func (s *stubKeycloakService) ListEvents(context.Context, string, int, int) ([]*domain.KeycloakEvent, error) {
	return nil, nil
}

func (s *stubKeycloakService) ListEventsByRealm(context.Context, string, string, time.Time, time.Time, int, int) ([]*domain.KeycloakEvent, error) {
	return nil, nil
}

func (s *stubKeycloakService) ListEventsByType(context.Context, string, string, string, time.Time, time.Time, int, int) ([]*domain.KeycloakEvent, error) {
	return nil, nil
}

func (s *stubKeycloakService) ListEventsByTimeRange(context.Context, string, time.Time, time.Time, int, int) ([]*domain.KeycloakEvent, error) {
	return nil, nil
}

func (s *stubKeycloakService) CountEvents(context.Context, string) (int64, error) { return 0, nil }

func (s *stubKeycloakService) CountEventsByType(context.Context, string, string, string, time.Time, time.Time) (int64, error) {
	return 0, nil
}

func (s *stubKeycloakService) GetEventStats(context.Context, string, string, time.Time, time.Time) (*keycloak.EventStats, error) {
	return nil, nil
}

func (s *stubKeycloakService) GetLatestMetrics(context.Context, string) (*domain.KeycloakMetrics, error) {
	return nil, nil
}

func (s *stubKeycloakService) GetLatestMetricsByRealm(_ context.Context, _, realmName string) (*domain.KeycloakMetrics, error) {
	return s.realmMetrics[realmName], nil
}

func (s *stubKeycloakService) GetMetricsHistory(context.Context, string, time.Time, time.Time) ([]*domain.KeycloakMetrics, error) {
	return nil, nil
}

func (s *stubKeycloakService) GetMetricsHistoryByRealm(context.Context, string, string, time.Time, time.Time) ([]*domain.KeycloakMetrics, error) {
	return nil, nil
}

func (s *stubKeycloakService) GetAllRealmMetrics(context.Context, string) (map[string]*domain.KeycloakMetrics, error) {
	return s.realmMetrics, nil
}

func (s *stubKeycloakService) GetLatestHealth(context.Context, string) (*domain.KeycloakHealth, error) {
	return nil, nil
}

func (s *stubKeycloakService) GetHealthHistory(context.Context, string, time.Time, time.Time) ([]*domain.KeycloakHealth, error) {
	return nil, nil
}

func (s *stubKeycloakService) ListRealms(context.Context, string) ([]*domain.KeycloakRealmInfo, error) {
	return s.realms, nil
}

func (s *stubKeycloakService) GetRealm(context.Context, string, string) (*domain.KeycloakRealmInfo, error) {
	return nil, nil
}

func (s *stubKeycloakService) ListAmfaEnabledRealms(context.Context, string) ([]string, error) {
	return s.amfaRealms, nil
}

func (s *stubKeycloakService) GetDashboard(context.Context, string, string) (*keycloak.Dashboard, error) {
	return s.dashboard, nil
}

func (s *stubKeycloakService) GetVersionInfo(context.Context, string) (*keycloak.VersionCheckResult, error) {
	return nil, nil
}

func (s *stubKeycloakService) GetUsers(context.Context, string, string, int, int) ([]keycloakadmin.UserRepresentation, error) {
	return nil, nil
}

func (s *stubKeycloakService) GetClients(context.Context, string, string) ([]*keycloakadmin.ClientRepresentation, error) {
	return nil, nil
}

func (s *stubKeycloakService) GetUserDetails(context.Context, string, string, string) (*keycloakadmin.UserDetails, error) {
	return nil, nil
}

func (s *stubKeycloakService) GetInfinispanMetrics(context.Context, string) (*keycloakadmin.InfinispanMetrics, error) {
	return nil, nil
}

func (s *stubKeycloakService) SetClientProvider(keycloak.ClientProvider) {}

func (s *stubKeycloakService) SetVersionChecker(*keycloak.VersionChecker) {}

var _ keycloak.Service = (*stubKeycloakService)(nil)

func newTestKeycloakHandlers(svc keycloak.Service, rbacSvc RBACChecker) *KeycloakHandlers {
	return NewKeycloakHandlers(
		svc,
		rbacSvc,
		nil,
		newTestLogger(),
		testAuthMiddleware,
		NewRBACMiddleware(rbacSvc, newTestLogger()),
	)
}

// setupTestKeycloakRouter mounts the keycloak routes exactly as the tenant
// router does, so a test drives the handler through its real middleware chain.
func setupTestKeycloakRouter(h *KeycloakHandlers) *chi.Mux {
	r := chi.NewRouter()
	r.Route("/api/tenants/{tenantID}", func(r chi.Router) {
		r.Use(TenantIDMiddleware)
		h.RegisterTenantRoutes(r)
	})
	return r
}

// stubKeycloakEventRepo records the arguments each read ran with so a test can
// assert what the query was actually narrowed to.
type stubKeycloakEventRepo struct {
	byRealm      []*domain.KeycloakEvent
	byType       []*domain.KeycloakEvent
	realmCalls   []string
	typeCalls    [][2]string
	otherRealmEv []*domain.KeycloakEvent
}

func (s *stubKeycloakEventRepo) GetEvents(context.Context, string, int, int) ([]*domain.KeycloakEvent, error) {
	return nil, nil
}

func (s *stubKeycloakEventRepo) GetEventsByRealm(_ context.Context, _, realmName string, _, _ time.Time, _, _ int) ([]*domain.KeycloakEvent, error) {
	s.realmCalls = append(s.realmCalls, realmName)
	return s.byRealm, nil
}

func (s *stubKeycloakEventRepo) GetEventsByType(_ context.Context, _, realmName, eventType string, _, _ time.Time, _, _ int) ([]*domain.KeycloakEvent, error) {
	s.typeCalls = append(s.typeCalls, [2]string{realmName, eventType})
	if realmName == "" {
		return append(append([]*domain.KeycloakEvent{}, s.byType...), s.otherRealmEv...), nil
	}
	return s.byType, nil
}

func (s *stubKeycloakEventRepo) GetEventsByTimeRange(context.Context, string, time.Time, time.Time, int, int) ([]*domain.KeycloakEvent, error) {
	return nil, nil
}

func (s *stubKeycloakEventRepo) CountEvents(context.Context, string) (int64, error) { return 0, nil }

func (s *stubKeycloakEventRepo) CountEventsByType(context.Context, string, string, string, time.Time, time.Time) (int64, error) {
	return 0, nil
}

func (s *stubKeycloakEventRepo) SaveEvent(context.Context, *domain.KeycloakEvent) error { return nil }

func (s *stubKeycloakEventRepo) SaveEvents(context.Context, []*domain.KeycloakEvent) error {
	return nil
}

func (s *stubKeycloakEventRepo) DeleteEventsBefore(context.Context, string, time.Time) (int64, error) {
	return 0, nil
}

var _ keycloak.EventRepository = (*stubKeycloakEventRepo)(nil)

// Drives the real keycloak service so the assertion lands on the repository
// query rather than on a stub of the layer under test.
func TestKeycloakHandlers_Events_TypeFilterStaysInRealm(t *testing.T) {
	tests := []struct {
		name      string
		query     string
		wantType  [][2]string
		wantRealm []string
	}{
		{
			name:      "type filter runs inside the authorized realm",
			query:     "?realm=realmB&type=LOGIN",
			wantType:  [][2]string{{"realmB", "LOGIN"}},
			wantRealm: nil,
		},
		{
			name:      "no type filter still runs inside the authorized realm",
			query:     "?realm=realmB",
			wantType:  nil,
			wantRealm: []string{"realmB"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &stubKeycloakEventRepo{
				byRealm: []*domain.KeycloakEvent{{RealmName: "realmB", EventType: "LOGIN"}},
				byType:  []*domain.KeycloakEvent{{RealmName: "realmB", EventType: "LOGIN"}},
				otherRealmEv: []*domain.KeycloakEvent{
					{RealmName: "realmA", EventType: "LOGIN", Username: "victim@example.com"},
				},
			}
			svc := keycloak.NewService(repo, nil, nil, nil, nil, nil)
			rbacSvc := &mockRBACChecker{
				policies: policyFor("tenant-a", "realmB"),
				hasAccessToRealmFn: func(_ context.Context, _ uint, _, realmName string) (bool, error) {
					return realmName == "realmB", nil
				},
			}
			router := setupTestKeycloakRouter(newTestKeycloakHandlers(svc, rbacSvc))

			req := httptest.NewRequest(http.MethodGet, "/api/tenants/tenant-a/keycloak/events"+tt.query, nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
			}
			if !reflect.DeepEqual(repo.typeCalls, tt.wantType) {
				t.Errorf("GetEventsByType calls = %v, want %v", repo.typeCalls, tt.wantType)
			}
			if !reflect.DeepEqual(repo.realmCalls, tt.wantRealm) {
				t.Errorf("GetEventsByRealm calls = %v, want %v", repo.realmCalls, tt.wantRealm)
			}

			var resp struct {
				Events []*domain.KeycloakEvent `json:"events"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatalf("decode: %v; body = %s", err, rec.Body.String())
			}
			for _, ev := range resp.Events {
				if ev.RealmName != "realmB" {
					t.Errorf("response discloses realm %q; body = %s", ev.RealmName, rec.Body.String())
				}
			}
		})
	}
}

func TestKeycloakHandlers_Events_DeniedRealmNeverQueries(t *testing.T) {
	repo := &stubKeycloakEventRepo{}
	svc := keycloak.NewService(repo, nil, nil, nil, nil, nil)
	rbacSvc := &mockRBACChecker{
		policies: policyFor("tenant-a", "realmB"),
		hasAccessToRealmFn: func(_ context.Context, _ uint, _, realmName string) (bool, error) {
			return realmName == "realmB", nil
		},
	}
	router := setupTestKeycloakRouter(newTestKeycloakHandlers(svc, rbacSvc))

	req := httptest.NewRequest(http.MethodGet, "/api/tenants/tenant-a/keycloak/events?realm=realmA&type=LOGIN", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusForbidden, rec.Body.String())
	}
	if len(repo.typeCalls) != 0 || len(repo.realmCalls) != 0 {
		t.Errorf("repository queried on a denied realm: byType=%v byRealm=%v", repo.typeCalls, repo.realmCalls)
	}
}

func realmMetrics(names ...string) map[string]*domain.KeycloakMetrics {
	out := make(map[string]*domain.KeycloakMetrics, len(names))
	for _, n := range names {
		out[n] = &domain.KeycloakMetrics{RealmName: n, TotalUsers: 10}
	}
	return out
}

func TestKeycloakHandlers_Metrics_AllRealmsNarrowedToPolicy(t *testing.T) {
	tests := []struct {
		name       string
		rbac       *mockRBACChecker
		wantRealms []string
	}{
		{
			name:       "caller with no policy row keeps every realm",
			rbac:       &mockRBACChecker{},
			wantRealms: []string{"realmA", "realmB"},
		},
		{
			name:       "admin keeps every realm despite a restricting policy",
			rbac:       &mockRBACChecker{isAdmin: true, policies: policyFor("tenant-a", "realmB")},
			wantRealms: []string{"realmA", "realmB"},
		},
		{
			name:       "policy restricted to one realm narrows the response",
			rbac:       &mockRBACChecker{policies: policyFor("tenant-a", "realmB")},
			wantRealms: []string{"realmB"},
		},
		{
			name: "policy with an empty realm list discloses no realm",
			rbac: &mockRBACChecker{policies: policyFor("tenant-a")},
		},
		{
			name:       "policy for another tenant does not restrict this one",
			rbac:       &mockRBACChecker{policies: policyFor("tenant-other", "realmB")},
			wantRealms: []string{"realmA", "realmB"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &stubKeycloakService{realmMetrics: realmMetrics("realmA", "realmB")}
			router := setupTestKeycloakRouter(newTestKeycloakHandlers(svc, tt.rbac))

			req := httptest.NewRequest(http.MethodGet, "/api/tenants/tenant-a/keycloak/metrics", nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
			}

			var resp struct {
				Metrics map[string]*domain.KeycloakMetrics `json:"metrics"`
				Count   int                                `json:"count"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatalf("decode: %v; body = %s", err, rec.Body.String())
			}

			if resp.Count != len(tt.wantRealms) {
				t.Errorf("count = %d, want %d", resp.Count, len(tt.wantRealms))
			}
			if len(resp.Metrics) != len(tt.wantRealms) {
				t.Fatalf("metrics = %v, want realms %v", keysOf(resp.Metrics), tt.wantRealms)
			}
			for _, realm := range tt.wantRealms {
				if _, ok := resp.Metrics[realm]; !ok {
					t.Errorf("metrics missing realm %q; got %v", realm, keysOf(resp.Metrics))
				}
			}
		})
	}
}

func dashboardFor(names ...string) *keycloak.Dashboard {
	summaries := make([]*keycloak.RealmSummary, 0, len(names))
	for _, n := range names {
		summaries = append(summaries, &keycloak.RealmSummary{RealmName: n, Enabled: true, IsHealthy: true})
	}
	return &keycloak.Dashboard{Realms: summaries, TotalRealms: len(summaries)}
}

func TestKeycloakHandlers_Dashboard_AllRealmsNarrowedToPolicy(t *testing.T) {
	tests := []struct {
		name       string
		rbac       *mockRBACChecker
		wantRealms []string
	}{
		{
			name:       "caller with no policy row keeps every realm",
			rbac:       &mockRBACChecker{},
			wantRealms: []string{"realmA", "realmB"},
		},
		{
			name:       "admin keeps every realm despite a restricting policy",
			rbac:       &mockRBACChecker{isAdmin: true, policies: policyFor("tenant-a", "realmB")},
			wantRealms: []string{"realmA", "realmB"},
		},
		{
			name:       "policy restricted to one realm narrows the response",
			rbac:       &mockRBACChecker{policies: policyFor("tenant-a", "realmB")},
			wantRealms: []string{"realmB"},
		},
		{
			name: "policy with an empty realm list discloses no realm",
			rbac: &mockRBACChecker{policies: policyFor("tenant-a")},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &stubKeycloakService{dashboard: dashboardFor("realmA", "realmB")}
			router := setupTestKeycloakRouter(newTestKeycloakHandlers(svc, tt.rbac))

			req := httptest.NewRequest(http.MethodGet, "/api/tenants/tenant-a/keycloak/dashboard", nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
			}

			var resp keycloak.Dashboard
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatalf("decode: %v; body = %s", err, rec.Body.String())
			}

			if resp.TotalRealms != len(tt.wantRealms) {
				t.Errorf("total_realms = %d, want %d", resp.TotalRealms, len(tt.wantRealms))
			}
			var got []string
			for _, summary := range resp.Realms {
				got = append(got, summary.RealmName)
			}
			if len(got) != len(tt.wantRealms) {
				t.Fatalf("realms = %v, want %v", got, tt.wantRealms)
			}
			for i, realm := range tt.wantRealms {
				if got[i] != realm {
					t.Errorf("realms = %v, want %v", got, tt.wantRealms)
					break
				}
			}
		})
	}
}

func TestKeycloakHandlers_Dashboard_NamedRealmStillDenied(t *testing.T) {
	svc := &stubKeycloakService{dashboard: dashboardFor("realmA", "realmB")}
	rbacSvc := &mockRBACChecker{
		policies: policyFor("tenant-a", "realmB"),
		hasAccessToRealmFn: func(_ context.Context, _ uint, _, realmName string) (bool, error) {
			return realmName == "realmB", nil
		},
	}
	router := setupTestKeycloakRouter(newTestKeycloakHandlers(svc, rbacSvc))

	req := httptest.NewRequest(http.MethodGet, "/api/tenants/tenant-a/keycloak/dashboard?realm=realmA", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusForbidden, rec.Body.String())
	}
}

func TestKeycloakHandlers_Realms_NarrowedToPolicy(t *testing.T) {
	tests := []struct {
		name       string
		rbac       *mockRBACChecker
		wantRealms []string
	}{
		{
			name:       "caller with no policy row keeps every realm",
			rbac:       &mockRBACChecker{},
			wantRealms: []string{"realmA", "realmB"},
		},
		{
			name:       "admin keeps every realm despite a restricting policy",
			rbac:       &mockRBACChecker{isAdmin: true, policies: policyFor("tenant-a", "realmB")},
			wantRealms: []string{"realmA", "realmB"},
		},
		{
			name:       "policy restricted to one realm narrows the list",
			rbac:       &mockRBACChecker{policies: policyFor("tenant-a", "realmB")},
			wantRealms: []string{"realmB"},
		},
		{
			name: "policy with an empty realm list lists no realm",
			rbac: &mockRBACChecker{policies: policyFor("tenant-a")},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &stubKeycloakService{realms: realms("realmA", "realmB")}
			router := setupTestKeycloakRouter(newTestKeycloakHandlers(svc, tt.rbac))

			req := httptest.NewRequest(http.MethodGet, "/api/tenants/tenant-a/keycloak/realms", nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
			}

			var resp struct {
				Realms []*domain.KeycloakRealmInfo `json:"realms"`
				Count  int                         `json:"count"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatalf("decode: %v; body = %s", err, rec.Body.String())
			}

			if resp.Count != len(tt.wantRealms) {
				t.Errorf("count = %d, want %d", resp.Count, len(tt.wantRealms))
			}
			var got []string
			for _, realm := range resp.Realms {
				got = append(got, realm.RealmName)
			}
			if len(got) != len(tt.wantRealms) {
				t.Fatalf("realms = %v, want %v", got, tt.wantRealms)
			}
			for i, realm := range tt.wantRealms {
				if got[i] != realm {
					t.Errorf("realms = %v, want %v", got, tt.wantRealms)
					break
				}
			}
		})
	}
}

func TestKeycloakHandlers_AmfaRealms_NarrowedToPolicy(t *testing.T) {
	tests := []struct {
		name       string
		rbac       *mockRBACChecker
		wantRealms []string
	}{
		{
			name:       "caller with no policy row keeps every AMFA realm",
			rbac:       &mockRBACChecker{},
			wantRealms: []string{"realmA", "realmB"},
		},
		{
			name:       "admin keeps every AMFA realm despite a restricting policy",
			rbac:       &mockRBACChecker{isAdmin: true, policies: policyFor("tenant-a", "realmB")},
			wantRealms: []string{"realmA", "realmB"},
		},
		{
			name:       "policy restricted to one realm narrows the list",
			rbac:       &mockRBACChecker{policies: policyFor("tenant-a", "realmB")},
			wantRealms: []string{"realmB"},
		},
		{
			name: "policy naming only a realm without AMFA lists nothing",
			rbac: &mockRBACChecker{policies: policyFor("tenant-a", "realmC")},
		},
		{
			name: "policy with an empty realm list lists no realm",
			rbac: &mockRBACChecker{policies: policyFor("tenant-a")},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &stubKeycloakService{amfaRealms: []string{"realmA", "realmB"}}
			router := setupTestKeycloakRouter(newTestKeycloakHandlers(svc, tt.rbac))

			req := httptest.NewRequest(http.MethodGet, "/api/tenants/tenant-a/keycloak/amfa-realms", nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
			}

			var resp struct {
				Realms []string `json:"realms"`
				Count  int      `json:"count"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatalf("decode: %v; body = %s", err, rec.Body.String())
			}

			if resp.Count != len(tt.wantRealms) {
				t.Errorf("count = %d, want %d", resp.Count, len(tt.wantRealms))
			}
			if len(resp.Realms) != len(tt.wantRealms) {
				t.Fatalf("realms = %v, want %v", resp.Realms, tt.wantRealms)
			}
			for i, realm := range tt.wantRealms {
				if resp.Realms[i] != realm {
					t.Errorf("realms = %v, want %v", resp.Realms, tt.wantRealms)
					break
				}
			}
		})
	}
}

func keysOf(m map[string]*domain.KeycloakMetrics) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestKeycloakHandlers_CheckRealmAccess(t *testing.T) {
	tests := []struct {
		name           string
		realmName      string
		authenticated  bool
		hasAccessFn    func(ctx context.Context, userID uint, tenantID, realmName string) (bool, error)
		wantAllowed    bool
		wantStatusCode int
	}{
		{
			name:          "realm the caller may read",
			realmName:     "master",
			authenticated: true,
			wantAllowed:   true,
		},
		{
			name:          "realm outside the caller's policy",
			realmName:     "master",
			authenticated: true,
			hasAccessFn: func(_ context.Context, _ uint, _, _ string) (bool, error) {
				return false, nil
			},
			wantStatusCode: http.StatusForbidden,
		},
		{
			// An empty realm names no realm to authorize, so it is denied.
			name:           "empty realm name",
			realmName:      "",
			authenticated:  true,
			wantStatusCode: http.StatusForbidden,
		},
		{
			name:           "no authenticated caller",
			realmName:      "master",
			wantStatusCode: http.StatusUnauthorized,
		},
		{
			name:           "no authenticated caller and an empty realm",
			realmName:      "",
			wantStatusCode: http.StatusUnauthorized,
		},
		{
			name:          "realm lookup fails",
			realmName:     "master",
			authenticated: true,
			hasAccessFn: func(_ context.Context, _ uint, _, _ string) (bool, error) {
				return false, errors.New("policy lookup failed")
			},
			wantStatusCode: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &KeycloakHandlers{
				rbacService: &mockRBACChecker{hasAccessToRealmFn: tt.hasAccessFn},
				log:         newTestLogger(),
			}

			ctx := context.Background()
			if tt.authenticated {
				ctx = context.WithValue(ctx, UserInfoKey, &UserDetails{ID: 1, Email: "test@example.com"})
			}

			rec := httptest.NewRecorder()
			got := h.checkRealmAccess(ctx, rec, "tenant-a", tt.realmName)

			if got != tt.wantAllowed {
				t.Fatalf("checkRealmAccess() = %v, want %v; body = %s", got, tt.wantAllowed, rec.Body.String())
			}
			if tt.wantAllowed {
				return
			}
			if rec.Code != tt.wantStatusCode {
				t.Errorf("status = %d, want %d; body = %s", rec.Code, tt.wantStatusCode, rec.Body.String())
			}
		})
	}
}
