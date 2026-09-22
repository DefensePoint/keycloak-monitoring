package chi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/DefensePoint/keycloak-monitoring/internal/amfa"
	"github.com/DefensePoint/keycloak-monitoring/internal/amfacheck"
)

// stubAmfaService is a minimal in-memory amfa.Service for handler tests.
// Each field controls one method's response so a test can exercise either a
// happy or sad path independently.
type stubAmfaService struct {
	events  []amfa.Event
	total   int64
	listErr error

	stats    amfa.Stats
	statsErr error

	geo    []amfa.GeoBucket
	geoErr error

	// Last-call captures for assertions about opts plumbing.
	calls         int
	lastListOpts  amfa.ListEventsOptions
	lastStatsOpts amfa.StatsOptions
	lastGeoOpts   amfa.GeoOptions
}

func (s *stubAmfaService) ListEvents(_ context.Context, _ string, opts amfa.ListEventsOptions) ([]amfa.Event, int64, error) {
	s.calls++
	s.lastListOpts = opts
	return s.events, s.total, s.listErr
}

func (s *stubAmfaService) GetStats(_ context.Context, _ string, opts amfa.StatsOptions) (amfa.Stats, error) {
	s.calls++
	s.lastStatsOpts = opts
	return s.stats, s.statsErr
}

func (s *stubAmfaService) GetGeoBuckets(_ context.Context, _ string, opts amfa.GeoOptions) ([]amfa.GeoBucket, error) {
	s.calls++
	s.lastGeoOpts = opts
	return s.geo, s.geoErr
}

// newTestAmfaHandlers builds an AmfaHandlers wired with the given stub service,
// an unrestricted caller's RBAC checker, and no auth/RBAC middleware (handlers
// are called directly so middleware is not exercised here — that's covered in
// middleware_test.go).
func newTestAmfaHandlers(svc amfa.Service) *AmfaHandlers {
	return newTestAmfaHandlersWithRBAC(svc, &mockRBACChecker{})
}

func newTestAmfaHandlersWithRBAC(svc amfa.Service, rbacSvc RBACChecker) *AmfaHandlers {
	return NewAmfaHandlers(svc, nil, rbacSvc, newTestLogger(), nil, nil, nil)
}

// newAmfaRequest constructs an *http.Request with the chi route context
// pre-populated with a tenantID URL param and an authenticated caller in the
// context, mimicking what a real Chi-routed request looks like inside the
// handler once the auth middleware has run.
func newAmfaRequest(path, tenantID string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("tenantID", tenantID)
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
	ctx = context.WithValue(ctx, UserInfoKey, &UserDetails{ID: 1, Email: "test@example.com"})
	return req.WithContext(ctx)
}

func TestAmfaHandlers_GetEvents_RequiresRealmID(t *testing.T) {
	h := newTestAmfaHandlers(&stubAmfaService{})

	req := newAmfaRequest("/amfa/events", "tA")
	rec := httptest.NewRecorder()
	h.handleAmfaEvents(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "realm_id") {
		t.Errorf("expected error to mention realm_id, got %s", rec.Body.String())
	}
}

func TestAmfaHandlers_GetEvents_Returns404WhenTenantNotConfigured(t *testing.T) {
	h := newTestAmfaHandlers(&stubAmfaService{listErr: amfa.ErrAmfaNotConfigured})

	req := newAmfaRequest("/amfa/events?realm_id=master", "tA")
	rec := httptest.NewRecorder()
	h.handleAmfaEvents(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "amfa_not_configured") {
		t.Errorf("expected body to contain amfa_not_configured, got %s", rec.Body.String())
	}
}

func TestAmfaHandlers_GetEvents_Returns503OnUnavailable(t *testing.T) {
	h := newTestAmfaHandlers(&stubAmfaService{listErr: amfa.ErrAmfaUnavailable})

	req := newAmfaRequest("/amfa/events?realm_id=master", "tA")
	rec := httptest.NewRecorder()
	h.handleAmfaEvents(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusServiceUnavailable, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "amfa_unavailable") {
		t.Errorf("expected body to contain amfa_unavailable, got %s", rec.Body.String())
	}
}

func TestAmfaHandlers_GetEvents_Returns200WithItems(t *testing.T) {
	uid := "u1"
	svc := &stubAmfaService{
		events: []amfa.Event{{EventID: "e1", EventType: "LOGIN", UserID: &uid}},
		total:  1,
	}
	h := newTestAmfaHandlers(svc)

	req := newAmfaRequest("/amfa/events?realm_id=master&limit=25", "tA")
	rec := httptest.NewRecorder()
	h.handleAmfaEvents(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp struct {
		Items  []amfa.Event `json:"items"`
		Total  int64        `json:"total"`
		Limit  int          `json:"limit"`
		Offset int          `json:"offset"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v; body = %s", err, rec.Body.String())
	}
	if len(resp.Items) != 1 || resp.Items[0].EventID != "e1" {
		t.Errorf("items mismatch: %+v", resp.Items)
	}
	if resp.Total != 1 {
		t.Errorf("total = %d, want 1", resp.Total)
	}
	if resp.Limit != 25 {
		t.Errorf("limit = %d, want 25", resp.Limit)
	}
	if svc.lastListOpts.RealmID != "master" {
		t.Errorf("RealmID = %q, want %q", svc.lastListOpts.RealmID, "master")
	}
}

func TestAmfaHandlers_GetStats_Returns200(t *testing.T) {
	svc := &stubAmfaService{
		stats: amfa.Stats{Total: 100, Risky: 5, UniqueUsers: 42, FlaggedIPs: 3},
	}
	h := newTestAmfaHandlers(svc)

	req := newAmfaRequest("/amfa/stats?realm_id=master", "tA")
	rec := httptest.NewRecorder()
	h.handleAmfaStats(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got amfa.Stats
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v; body = %s", err, rec.Body.String())
	}
	if got != svc.stats {
		t.Errorf("stats = %+v, want %+v", got, svc.stats)
	}
	if svc.lastStatsOpts.RealmID != "master" {
		t.Errorf("RealmID = %q, want %q", svc.lastStatsOpts.RealmID, "master")
	}
}

func TestAmfaHandlers_GetStats_AllRealmsWhenRealmIDOmitted(t *testing.T) {
	// Unlike /amfa/events and /amfa/geo, /amfa/stats treats a missing realm_id as
	// "all realms": for a caller whose policy does not restrict realms it returns
	// 200 and passes an empty RealmID to the service so the repo drops the realm
	// filter.
	svc := &stubAmfaService{
		stats: amfa.Stats{Total: 100, Risky: 5, UniqueUsers: 42, FlaggedIPs: 3},
	}
	h := newTestAmfaHandlers(svc)

	req := newAmfaRequest("/amfa/stats", "tA")
	rec := httptest.NewRecorder()
	h.handleAmfaStats(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if svc.lastStatsOpts.RealmID != "" {
		t.Errorf("RealmID = %q, want empty (all realms)", svc.lastStatsOpts.RealmID)
	}
}

func TestAmfaHandlers_GetGeo_Returns200WithEmptyArray(t *testing.T) {
	// Service returns a nil slice; the handler must encode JSON `[]`, not `null`.
	h := newTestAmfaHandlers(&stubAmfaService{geo: nil})

	req := newAmfaRequest("/amfa/geo?realm_id=master", "tA")
	rec := httptest.NewRecorder()
	h.handleAmfaGeo(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	// json.Encoder appends a trailing newline; trim it before comparing.
	body := strings.TrimSpace(rec.Body.String())
	if body != "[]" {
		t.Errorf("body = %q, want %q", body, "[]")
	}
}

func TestAmfaHandlers_GetGeo_Returns200WithBuckets(t *testing.T) {
	country := "US"
	svc := &stubAmfaService{
		geo: []amfa.GeoBucket{{Country: &country, Lat: 40.0, Long: -74.0, Count: 10, RiskyCount: 2}},
	}
	h := newTestAmfaHandlers(svc)

	req := newAmfaRequest("/amfa/geo?realm_id=master", "tA")
	rec := httptest.NewRecorder()
	h.handleAmfaGeo(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got []amfa.GeoBucket
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v; body = %s", err, rec.Body.String())
	}
	if len(got) != 1 || got[0].Count != 10 {
		t.Errorf("buckets mismatch: %+v", got)
	}
}

func TestAmfaHandlers_GetGeo_RequiresRealmID(t *testing.T) {
	h := newTestAmfaHandlers(&stubAmfaService{})

	req := newAmfaRequest("/amfa/geo", "tA")
	rec := httptest.NewRecorder()
	h.handleAmfaGeo(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestAmfaHandlers_RealmPolicy(t *testing.T) {
	tests := []struct {
		name       string
		rbac       *mockRBACChecker
		handler    func(*AmfaHandlers) http.HandlerFunc
		path       string
		statsErr   error
		wantStatus int
		wantRealm  string
		wantCode   string
	}{
		{
			name:       "events for a caller with no policy row",
			rbac:       &mockRBACChecker{},
			handler:    func(h *AmfaHandlers) http.HandlerFunc { return h.handleAmfaEvents },
			path:       "/amfa/events?realm_id=master",
			wantStatus: http.StatusOK,
		},
		{
			name:       "events for an admin with a restricting policy",
			rbac:       &mockRBACChecker{isAdmin: true, policies: policyFor("tA", "other")},
			handler:    func(h *AmfaHandlers) http.HandlerFunc { return h.handleAmfaEvents },
			path:       "/amfa/events?realm_id=master",
			wantStatus: http.StatusOK,
		},
		{
			name:       "events for the caller's own realm",
			rbac:       &mockRBACChecker{policies: policyFor("tA", "master")},
			handler:    func(h *AmfaHandlers) http.HandlerFunc { return h.handleAmfaEvents },
			path:       "/amfa/events?realm_id=master",
			wantStatus: http.StatusOK,
		},
		{
			name:       "events for a realm outside the policy",
			rbac:       &mockRBACChecker{policies: policyFor("tA", "other")},
			handler:    func(h *AmfaHandlers) http.HandlerFunc { return h.handleAmfaEvents },
			path:       "/amfa/events?realm_id=master",
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "events for a policy with an empty realm list",
			rbac:       &mockRBACChecker{policies: policyFor("tA")},
			handler:    func(h *AmfaHandlers) http.HandlerFunc { return h.handleAmfaEvents },
			path:       "/amfa/events?realm_id=master",
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "stats for the caller's own realm",
			rbac:       &mockRBACChecker{policies: policyFor("tA", "master")},
			handler:    func(h *AmfaHandlers) http.HandlerFunc { return h.handleAmfaStats },
			path:       "/amfa/stats?realm_id=master",
			wantStatus: http.StatusOK,
		},
		{
			name:       "stats for a realm outside the policy",
			rbac:       &mockRBACChecker{policies: policyFor("tA", "other")},
			handler:    func(h *AmfaHandlers) http.HandlerFunc { return h.handleAmfaStats },
			path:       "/amfa/stats?realm_id=master",
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "all-realm stats for a caller with no policy row",
			rbac:       &mockRBACChecker{},
			handler:    func(h *AmfaHandlers) http.HandlerFunc { return h.handleAmfaStats },
			path:       "/amfa/stats",
			wantStatus: http.StatusOK,
		},
		{
			name:       "all-realm stats for an admin",
			rbac:       &mockRBACChecker{isAdmin: true},
			handler:    func(h *AmfaHandlers) http.HandlerFunc { return h.handleAmfaStats },
			path:       "/amfa/stats",
			wantStatus: http.StatusOK,
		},
		{
			name:       "all-realm stats narrow to a caller's single realm",
			rbac:       &mockRBACChecker{policies: policyFor("tA", "master")},
			handler:    func(h *AmfaHandlers) http.HandlerFunc { return h.handleAmfaStats },
			path:       "/amfa/stats",
			wantStatus: http.StatusOK,
			wantRealm:  "master",
		},
		{
			// A permission refusal, so it must not borrow the transport's code.
			name:       "all-realm stats for a caller left with several realms",
			rbac:       &mockRBACChecker{policies: policyFor("tA", "master", "other")},
			handler:    func(h *AmfaHandlers) http.HandlerFunc { return h.handleAmfaStats },
			path:       "/amfa/stats",
			wantStatus: http.StatusForbidden,
			wantCode:   "amfa_realm_scope_requires_realm",
		},
		{
			// The other half: a transport that genuinely cannot aggregate keeps
			// its own code.
			name:       "all-realm stats a transport cannot aggregate",
			rbac:       &mockRBACChecker{},
			handler:    func(h *AmfaHandlers) http.HandlerFunc { return h.handleAmfaStats },
			path:       "/amfa/stats",
			statsErr:   amfa.ErrAllRealmsUnsupported,
			wantStatus: http.StatusNotImplemented,
			wantCode:   "amfa_all_realms_unsupported",
		},
		{
			// Also a permission refusal, so it uses the same code rather than
			// the generic realm denial.
			name:       "all-realm stats for a policy with an empty realm list",
			rbac:       &mockRBACChecker{policies: policyFor("tA")},
			handler:    func(h *AmfaHandlers) http.HandlerFunc { return h.handleAmfaStats },
			path:       "/amfa/stats",
			wantStatus: http.StatusForbidden,
			wantCode:   "amfa_realm_scope_requires_realm",
		},
		{
			name:       "geo for the caller's own realm",
			rbac:       &mockRBACChecker{policies: policyFor("tA", "master")},
			handler:    func(h *AmfaHandlers) http.HandlerFunc { return h.handleAmfaGeo },
			path:       "/amfa/geo?realm_id=master",
			wantStatus: http.StatusOK,
		},
		{
			name:       "geo for a realm outside the policy",
			rbac:       &mockRBACChecker{policies: policyFor("tA", "other")},
			handler:    func(h *AmfaHandlers) http.HandlerFunc { return h.handleAmfaGeo },
			path:       "/amfa/geo?realm_id=master",
			wantStatus: http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &stubAmfaService{statsErr: tt.statsErr}
			h := newTestAmfaHandlersWithRBAC(svc, tt.rbac)

			rec := httptest.NewRecorder()
			tt.handler(h)(rec, newAmfaRequest(tt.path, "tA"))

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body = %s", rec.Code, tt.wantStatus, rec.Body.String())
			}
			// A refusal the handler makes must never reach the service.
			if tt.wantStatus >= http.StatusBadRequest && tt.statsErr == nil && svc.calls != 0 {
				t.Errorf("AMFA service called %d times for a refused request, want 0", svc.calls)
			}
			if tt.wantRealm != "" && svc.lastStatsOpts.RealmID != tt.wantRealm {
				t.Errorf("stats RealmID = %q, want %q", svc.lastStatsOpts.RealmID, tt.wantRealm)
			}
			if tt.wantCode != "" {
				var body struct {
					Error string `json:"error"`
				}
				if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
					t.Fatalf("decode: %v; body = %s", err, rec.Body.String())
				}
				if body.Error != tt.wantCode {
					t.Errorf("error = %q, want %q", body.Error, tt.wantCode)
				}
			}
		})
	}
}

// setupTestAmfaRouter mounts the AMFA routes exactly as the tenant router does,
// so a test drives the handler through its real middleware chain.
func setupTestAmfaRouter(h *AmfaHandlers) *chi.Mux {
	r := chi.NewRouter()
	r.Route("/api/tenants/{tenantID}", func(r chi.Router) {
		r.Use(TenantIDMiddleware)
		h.RegisterTenantRoutes(r)
	})
	return r
}

func TestAmfaHandlers_GetStats_AppliedRealmID(t *testing.T) {
	tests := []struct {
		name        string
		rbac        *mockRBACChecker
		path        string
		wantApplied string
	}{
		{
			name: "all-realm request narrowed to the caller's own realm names it",
			rbac: &mockRBACChecker{policies: policyFor("tA", "master")},
			path: "/amfa/stats",
			// The caller asked for every realm and got one, so the response
			// names it.
			wantApplied: "master",
		},
		{
			name: "all-realm request from an unrestricted caller stays unlabelled",
			rbac: &mockRBACChecker{},
			path: "/amfa/stats",
		},
		{
			name: "explicitly requested realm is not a narrowing",
			rbac: &mockRBACChecker{policies: policyFor("tA", "master")},
			path: "/amfa/stats?realm_id=master",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &stubAmfaService{stats: amfa.Stats{Total: 12}}
			router := setupTestAmfaRouter(NewAmfaHandlers(
				svc, nil, tt.rbac, newTestLogger(),
				testAuthMiddleware, NewRBACMiddleware(tt.rbac, newTestLogger()), nil,
			))

			req := httptest.NewRequest(http.MethodGet, "/api/tenants/tA"+tt.path, nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
			}

			var resp struct {
				Total          int64  `json:"total"`
				AppliedRealmID string `json:"applied_realm_id"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatalf("decode: %v; body = %s", err, rec.Body.String())
			}
			if resp.Total != 12 {
				t.Errorf("total = %d, want 12", resp.Total)
			}
			if resp.AppliedRealmID != tt.wantApplied {
				t.Errorf("applied_realm_id = %q, want %q", resp.AppliedRealmID, tt.wantApplied)
			}
			if tt.wantApplied == "" && strings.Contains(rec.Body.String(), "applied_realm_id") {
				t.Errorf("applied_realm_id present for an unnarrowed answer: %s", rec.Body.String())
			}
		})
	}
}

func TestAmfaHandlers_RealmPolicy_RequiresAuthenticatedCaller(t *testing.T) {
	svc := &stubAmfaService{}
	h := newTestAmfaHandlers(svc)

	req := httptest.NewRequest(http.MethodGet, "/amfa/events?realm_id=master", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("tenantID", "tA")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rec := httptest.NewRecorder()
	h.handleAmfaEvents(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}
	if svc.calls != 0 {
		t.Errorf("AMFA service called %d times without an authenticated caller, want 0", svc.calls)
	}
}

func TestAmfaHandlers_GetEvents_LimitCapAndDefaults(t *testing.T) {
	svc := &stubAmfaService{}
	h := newTestAmfaHandlers(svc)

	// Request limit way above MaxLimit — handler must cap it.
	req := newAmfaRequest("/amfa/events?realm_id=master&limit=999999&offset=10&risk_level=3&event_type=LOGIN_ERROR", "tA")
	rec := httptest.NewRecorder()
	h.handleAmfaEvents(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if svc.lastListOpts.Limit != MaxLimit {
		t.Errorf("Limit = %d, want capped %d", svc.lastListOpts.Limit, MaxLimit)
	}
	if svc.lastListOpts.Offset != 10 {
		t.Errorf("Offset = %d, want 10", svc.lastListOpts.Offset)
	}
	if svc.lastListOpts.RiskLevel == nil || *svc.lastListOpts.RiskLevel != 3 {
		t.Errorf("RiskLevel = %+v, want 3", svc.lastListOpts.RiskLevel)
	}
	if svc.lastListOpts.EventType != "LOGIN_ERROR" {
		t.Errorf("EventType = %q, want LOGIN_ERROR", svc.lastListOpts.EventType)
	}
}

// Compile-time interface assertions also live in the production file, but
// re-asserting here protects against accidental signature changes during refactors.
var _ amfa.Service = (*stubAmfaService)(nil)

// fakeCheckerService implements amfacheck.Service for handler tests.
type fakeCheckerService struct{ runErr error }

func (fakeCheckerService) Start(context.Context) error         { return nil }
func (fakeCheckerService) Stop() error                         { return nil }
func (fakeCheckerService) AddCheck(amfacheck.Check)            {}
func (f fakeCheckerService) RunCheckNow(context.Context) error { return f.runErr }

func newAmfaCheckerTestHandler(services map[string]amfacheck.Service) *AmfaHandlers {
	return &AmfaHandlers{
		checkerRunner: amfacheck.NewRunner(services),
		log:           newTestLogger(),
	}
}

func doReq(t *testing.T, h http.HandlerFunc, method, tenantID string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, "/", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("tenantID", tenantID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rec := httptest.NewRecorder()
	h(rec, req)
	return rec
}

func TestAmfaCheckerStatus_EnabledTrue(t *testing.T) {
	h := newAmfaCheckerTestHandler(map[string]amfacheck.Service{"t1": fakeCheckerService{}})
	rec := doReq(t, h.handleAmfaCheckerStatus, http.MethodGet, "t1")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"enabled":true`) {
		t.Errorf("body = %s, want enabled:true", rec.Body.String())
	}
}

func TestAmfaCheckerStatus_EnabledFalse(t *testing.T) {
	h := newAmfaCheckerTestHandler(map[string]amfacheck.Service{"t1": fakeCheckerService{}})
	rec := doReq(t, h.handleAmfaCheckerStatus, http.MethodGet, "other")
	if !strings.Contains(rec.Body.String(), `"enabled":false`) {
		t.Errorf("body = %s, want enabled:false", rec.Body.String())
	}
}

func TestAmfaCheckerRun_OK(t *testing.T) {
	h := newAmfaCheckerTestHandler(map[string]amfacheck.Service{"t1": fakeCheckerService{}})
	rec := doReq(t, h.handleAmfaCheckerRun, http.MethodPost, "t1")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"status":"ok"`) {
		t.Errorf("body = %s, want status:ok", rec.Body.String())
	}
}

func TestAmfaCheckerRun_NotEnabled(t *testing.T) {
	h := newAmfaCheckerTestHandler(map[string]amfacheck.Service{"t1": fakeCheckerService{}})
	rec := doReq(t, h.handleAmfaCheckerRun, http.MethodPost, "other")
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", rec.Code)
	}
}

func TestAmfaCheckerRun_AmfaUnavailable(t *testing.T) {
	// The run reports an ErrAmfaUnavailable-wrapped error (AMFA DB unreachable);
	// the handler must surface it as 503 instead of a silent 200.
	svc := fakeCheckerService{runErr: fmt.Errorf("amfa-risk-rejected: %w", amfa.ErrAmfaUnavailable)}
	h := newAmfaCheckerTestHandler(map[string]amfacheck.Service{"t1": svc})
	rec := doReq(t, h.handleAmfaCheckerRun, http.MethodPost, "t1")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "amfa_unavailable") {
		t.Errorf("body = %s, want amfa_unavailable", rec.Body.String())
	}
}
