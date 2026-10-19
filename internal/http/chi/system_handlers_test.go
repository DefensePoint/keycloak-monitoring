package chi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/events"
)

type mockRealmLister struct {
	realms []*domain.KeycloakRealmInfo
	err    error
}

func (m *mockRealmLister) ListRealms(_ context.Context, _ string) ([]*domain.KeycloakRealmInfo, error) {
	return m.realms, m.err
}

// stubEventRepo records the options every query ran with and counts calls, so a
// denied query can be shown never to have reached the repository.
type stubEventRepo struct {
	events   []*domain.Event
	total    int64
	calls    int
	lastOpts *events.ListOptions
}

func (s *stubEventRepo) List(_ context.Context, opts *events.ListOptions) ([]*domain.Event, error) {
	s.calls++
	s.lastOpts = opts
	return s.events, nil
}

func (s *stubEventRepo) CountWithFilter(_ context.Context, opts *events.ListOptions) (int64, error) {
	s.calls++
	s.lastOpts = opts
	return s.total, nil
}

// GetByTenantAndEventID matches on both columns, as the real repository does.
func (s *stubEventRepo) GetByTenantAndEventID(_ context.Context, tenantID, eventID string) (*domain.Event, error) {
	s.calls++
	for _, e := range s.events {
		if e.TenantID == tenantID && e.EventID == eventID {
			return e, nil
		}
	}
	return nil, events.ErrNotFound
}

func realms(names ...string) []*domain.KeycloakRealmInfo {
	out := make([]*domain.KeycloakRealmInfo, 0, len(names))
	for _, n := range names {
		out = append(out, &domain.KeycloakRealmInfo{RealmName: n})
	}
	return out
}

func policyFor(tenantID string, realms ...string) []*domain.TenantPolicy {
	return []*domain.TenantPolicy{{TenantID: tenantID, AllowedRealms: realms}}
}

func newSystemRequest(path, tenantID string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("tenantID", tenantID)
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
	ctx = context.WithValue(ctx, UserInfoKey, &UserDetails{ID: 1, Email: "test@example.com"})
	return req.WithContext(ctx)
}

// realmScopeCase describes one caller against a tenant holding realmA and
// realmB. wantSources is the exact source set the events query may run with;
// empty means the query must not run at all.
type realmScopeCase struct {
	name        string
	rbac        *mockRBACChecker
	source      string
	realm       string
	wantSources []string
}

func realmScopeCases() []realmScopeCase {
	return []realmScopeCase{
		{
			name:        "caller with no policy row keeps every realm",
			rbac:        &mockRBACChecker{},
			wantSources: []string{"keycloak:realmA", "amfa:realmA", "keycloak:realmB", "amfa:realmB"},
		},
		{
			name:        "admin keeps every realm despite a restricting policy",
			rbac:        &mockRBACChecker{isAdmin: true, policies: policyFor("tenant-a", "realmB")},
			wantSources: []string{"keycloak:realmA", "amfa:realmA", "keycloak:realmB", "amfa:realmB"},
		},
		{
			name:        "policy restricted to one realm narrows an unfiltered query",
			rbac:        &mockRBACChecker{policies: policyFor("tenant-a", "realmB")},
			wantSources: []string{"keycloak:realmB", "amfa:realmB"},
		},
		{
			name: "policy with an empty realm list denies every realm",
			rbac: &mockRBACChecker{policies: policyFor("tenant-a")},
		},
		{
			name:        "policy for another tenant does not restrict this one",
			rbac:        &mockRBACChecker{policies: policyFor("tenant-other", "realmB")},
			wantSources: []string{"keycloak:realmA", "amfa:realmA", "keycloak:realmB", "amfa:realmB"},
		},
		{
			name:   "source outside the allowed realm is denied",
			rbac:   &mockRBACChecker{policies: policyFor("tenant-a", "realmB")},
			source: "keycloak:realmA",
		},
		{
			name:        "source inside the allowed realm still narrows",
			rbac:        &mockRBACChecker{policies: policyFor("tenant-a", "realmB")},
			source:      "keycloak:realmB",
			wantSources: []string{"keycloak:realmB"},
		},
		{
			// The realm filter only ever narrows, so a caller restricted to
			// realmB asking for realmA gets nothing.
			name:   "realm outside the allowed set cannot widen the scope",
			rbac:   &mockRBACChecker{policies: policyFor("tenant-a", "realmB")},
			realm:  "realmA",
			source: "",
		},
		{
			name:        "realm inside the allowed set still narrows to both its feeds",
			rbac:        &mockRBACChecker{policies: policyFor("tenant-a", "realmB")},
			realm:       "realmB",
			wantSources: []string{"keycloak:realmB", "amfa:realmB"},
		},
		{
			name:        "unrestricted caller may name either realm",
			rbac:        &mockRBACChecker{},
			realm:       "realmA",
			wantSources: []string{"keycloak:realmA", "amfa:realmA"},
		},
	}
}

// scopePath builds the query string for a realmScopeCase.
func (tt realmScopeCase) scopePath(base string) string {
	q := url.Values{}
	if tt.source != "" {
		q.Set("source", tt.source)
	}
	if tt.realm != "" {
		q.Set("realm", tt.realm)
	}
	if len(q) == 0 {
		return base
	}
	return base + "?" + q.Encode()
}

func TestTenantEventSources(t *testing.T) {
	tests := []struct {
		name            string
		realms          []*domain.KeycloakRealmInfo
		rbac            *mockRBACChecker
		requestedSource string
		requestedRealm  string
		want            []string
	}{
		{
			name:   "all sources for the tenant's realms",
			realms: realms("realmA", "realmB"),
			rbac:   &mockRBACChecker{},
			want:   []string{"keycloak:realmA", "amfa:realmA", "keycloak:realmB", "amfa:realmB"},
		},
		{
			name:            "narrow to one exact source within the tenant",
			realms:          realms("realmA", "realmB"),
			rbac:            &mockRBACChecker{},
			requestedSource: "keycloak:realmA",
			want:            []string{"keycloak:realmA"},
		},
		{
			name:            "narrow by realm name matches both feeds",
			realms:          realms("realmA", "realmB"),
			rbac:            &mockRBACChecker{},
			requestedSource: "realmB",
			want:            []string{"keycloak:realmB", "amfa:realmB"},
		},
		{
			name:            "cross-tenant source is denied",
			realms:          realms("realmA"),
			rbac:            &mockRBACChecker{},
			requestedSource: "keycloak:realmB",
			want:            []string{},
		},
		{
			// The bug this parameter exists for: a realm has to match every
			// source system it produces, not just Keycloak, or the realm's
			// AMFA-mirrored events vanish from the page that exists to show
			// them.
			name:           "realm matches both of its source systems",
			realms:         realms("realmA", "realmB"),
			rbac:           &mockRBACChecker{},
			requestedRealm: "realmA",
			want:           []string{"keycloak:realmA", "amfa:realmA"},
		},
		{
			// Exact, not substring: a realm named as a prefix of another must
			// not drag the other realm's events in.
			name:           "realm is matched exactly, not by prefix",
			realms:         realms("prod", "prod-staging"),
			rbac:           &mockRBACChecker{},
			requestedRealm: "prod",
			want:           []string{"keycloak:prod", "amfa:prod"},
		},
		{
			name:           "realm outside the tenant is denied",
			realms:         realms("realmA"),
			rbac:           &mockRBACChecker{},
			requestedRealm: "realmB",
			want:           []string{},
		},
		{
			name:            "realm and source together narrow to one system",
			realms:          realms("realmA", "realmB"),
			rbac:            &mockRBACChecker{},
			requestedRealm:  "realmA",
			requestedSource: "amfa:",
			want:            []string{"amfa:realmA"},
		},
		{
			name:   "tenant with no realms yields no sources",
			realms: nil,
			rbac:   &mockRBACChecker{},
			want:   []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &SystemHandlers{realmLister: &mockRealmLister{realms: tt.realms}, rbacService: tt.rbac}

			got, err := h.tenantEventSources(context.Background(), 1, "tenant-a", tt.requestedSource, tt.requestedRealm)
			if err != nil {
				t.Fatalf("tenantEventSources() error = %v", err)
			}
			if len(got) == 0 && len(tt.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("tenantEventSources() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTenantEventSources_RealmPolicy(t *testing.T) {
	for _, tt := range realmScopeCases() {
		t.Run(tt.name, func(t *testing.T) {
			h := &SystemHandlers{
				realmLister: &mockRealmLister{realms: realms("realmA", "realmB")},
				rbacService: tt.rbac,
			}

			got, err := h.tenantEventSources(context.Background(), 1, "tenant-a", tt.source, tt.realm)
			if err != nil {
				t.Fatalf("tenantEventSources() error = %v", err)
			}
			if len(got) == 0 && len(tt.wantSources) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tt.wantSources) {
				t.Errorf("tenantEventSources() = %v, want %v", got, tt.wantSources)
			}
		})
	}
}

func TestSystemHandlers_Events_RealmPolicy(t *testing.T) {
	for _, tt := range realmScopeCases() {
		t.Run(tt.name, func(t *testing.T) {
			repo := &stubEventRepo{events: []*domain.Event{{EventID: "e1"}}, total: 1}
			h := &SystemHandlers{
				eventRepo:   repo,
				realmLister: &mockRealmLister{realms: realms("realmA", "realmB")},
				rbacService: tt.rbac,
				log:         newTestLogger(),
			}

			rec := httptest.NewRecorder()
			h.handleEvents(rec, newSystemRequest(tt.scopePath("/events"), "tenant-a"))

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
			}

			var resp struct {
				Events []*domain.Event `json:"events"`
				Count  int             `json:"count"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatalf("decode: %v; body = %s", err, rec.Body.String())
			}

			if len(tt.wantSources) == 0 {
				if repo.calls != 0 {
					t.Errorf("repository queried %d times for a denied caller, want 0 (opts = %+v)", repo.calls, repo.lastOpts)
				}
				if resp.Count != 0 || len(resp.Events) != 0 {
					t.Errorf("count = %d, events = %d, want an empty result", resp.Count, len(resp.Events))
				}
				return
			}

			if repo.lastOpts == nil {
				t.Fatalf("repository was never queried, want sources %v", tt.wantSources)
			}
			if !reflect.DeepEqual(repo.lastOpts.Sources, tt.wantSources) {
				t.Errorf("sources = %v, want %v", repo.lastOpts.Sources, tt.wantSources)
			}
		})
	}
}

func TestSystemHandlers_Stats_RealmPolicy(t *testing.T) {
	for _, tt := range realmScopeCases() {
		t.Run(tt.name, func(t *testing.T) {
			repo := &stubEventRepo{total: 7}
			h := &SystemHandlers{
				eventRepo:   repo,
				realmLister: &mockRealmLister{realms: realms("realmA", "realmB")},
				rbacService: tt.rbac,
				log:         newTestLogger(),
			}

			rec := httptest.NewRecorder()
			h.handleStats(rec, newSystemRequest(tt.scopePath("/stats"), "tenant-a"))

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
			}

			var resp struct {
				TotalEvents int64 `json:"total_events"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatalf("decode: %v; body = %s", err, rec.Body.String())
			}

			if len(tt.wantSources) == 0 {
				if repo.calls != 0 {
					t.Errorf("repository queried %d times for a denied caller, want 0 (opts = %+v)", repo.calls, repo.lastOpts)
				}
				if resp.TotalEvents != 0 {
					t.Errorf("total_events = %d, want 0", resp.TotalEvents)
				}
				return
			}

			if repo.lastOpts == nil {
				t.Fatalf("repository was never queried, want sources %v", tt.wantSources)
			}
			if !reflect.DeepEqual(repo.lastOpts.Sources, tt.wantSources) {
				t.Errorf("sources = %v, want %v", repo.lastOpts.Sources, tt.wantSources)
			}
			if resp.TotalEvents != 7 {
				t.Errorf("total_events = %d, want 7", resp.TotalEvents)
			}
		})
	}
}

func TestSystemHandlers_RequireUserForScoping(t *testing.T) {
	tests := []struct {
		name    string
		handler func(*SystemHandlers) http.HandlerFunc
		path    string
	}{
		{name: "events", handler: func(h *SystemHandlers) http.HandlerFunc { return h.handleEvents }, path: "/events"},
		{name: "stats", handler: func(h *SystemHandlers) http.HandlerFunc { return h.handleStats }, path: "/stats"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &stubEventRepo{}
			h := &SystemHandlers{
				eventRepo:   repo,
				realmLister: &mockRealmLister{realms: realms("realmA")},
				rbacService: &mockRBACChecker{},
				log:         newTestLogger(),
			}

			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("tenantID", "tenant-a")
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

			rec := httptest.NewRecorder()
			tt.handler(h)(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusUnauthorized, rec.Body.String())
			}
			if repo.calls != 0 {
				t.Errorf("repository queried %d times without an authenticated caller, want 0", repo.calls)
			}
		})
	}
}

// newEventRequest is newSystemRequest for GET /events/{eventID}.
func newEventRequest(tenantID, eventID string, caller *UserDetails) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/api/tenants/"+tenantID+"/events/"+url.PathEscape(eventID), nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("tenantID", tenantID)
	rctx.URLParams.Add("eventID", eventID)
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
	if caller != nil {
		ctx = context.WithValue(ctx, UserInfoKey, caller)
	}
	return req.WithContext(ctx)
}

func TestSystemHandlers_Event_RealmPolicy(t *testing.T) {
	// The event lives in realmA; the tenant holds realmA and realmB.
	cases := []struct {
		name     string
		rbac     *mockRBACChecker
		wantCode int
	}{
		{"caller with no policy row sees it", &mockRBACChecker{}, http.StatusOK},
		{"admin sees it despite a restricting policy", &mockRBACChecker{isAdmin: true, policies: policyFor("tenant-a", "realmB")}, http.StatusOK},
		{"policy for another tenant does not hide it", &mockRBACChecker{policies: policyFor("tenant-b", "realmB")}, http.StatusOK},
		{"policy restricted to the other realm hides it", &mockRBACChecker{policies: policyFor("tenant-a", "realmB")}, http.StatusNotFound},
		{"policy with an empty realm list hides it", &mockRBACChecker{policies: policyFor("tenant-a")}, http.StatusNotFound},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			repo := &stubEventRepo{events: []*domain.Event{
				{TenantID: "tenant-a", EventID: "e1", Source: "keycloak:realmA", Username: "alice"},
			}}
			h := &SystemHandlers{
				eventRepo:   repo,
				realmLister: &mockRealmLister{realms: realms("realmA", "realmB")},
				rbacService: tt.rbac,
				log:         newTestLogger(),
			}

			rec := httptest.NewRecorder()
			h.handleEvent(rec, newEventRequest("tenant-a", "e1", &UserDetails{ID: 1}))

			if rec.Code != tt.wantCode {
				t.Fatalf("status = %d, want %d; body = %s", rec.Code, tt.wantCode, rec.Body.String())
			}
			if tt.wantCode == http.StatusOK {
				var got domain.Event
				if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
					t.Fatalf("decode: %v; body = %s", err, rec.Body.String())
				}
				if got.EventID != "e1" || got.Username != "alice" {
					t.Errorf("event = %+v, want e1 for alice", got)
				}
			}
		})
	}
}

// A caller left with no realm at all must not reach the repository, the same
// rule the list follows.
func TestSystemHandlers_Event_NoAllowedRealmNeverQueries(t *testing.T) {
	repo := &stubEventRepo{events: []*domain.Event{{TenantID: "tenant-a", EventID: "e1", Source: "keycloak:realmA"}}}
	h := &SystemHandlers{
		eventRepo:   repo,
		realmLister: &mockRealmLister{realms: realms("realmA")},
		rbacService: &mockRBACChecker{policies: policyFor("tenant-a")},
		log:         newTestLogger(),
	}

	rec := httptest.NewRecorder()
	h.handleEvent(rec, newEventRequest("tenant-a", "e1", &UserDetails{ID: 1}))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
	if repo.calls != 0 {
		t.Errorf("repository queried %d times for a caller with no allowed realm, want 0", repo.calls)
	}
}

func TestSystemHandlers_Event_NotFoundCases(t *testing.T) {
	cases := []struct {
		name    string
		eventID string
	}{
		{"another tenant's event with the same id", "shared"},
		{"an id no tenant has", "missing"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			repo := &stubEventRepo{events: []*domain.Event{
				{TenantID: "tenant-b", EventID: "shared", Source: "keycloak:realmA"},
			}}
			h := &SystemHandlers{
				eventRepo:   repo,
				realmLister: &mockRealmLister{realms: realms("realmA")},
				rbacService: &mockRBACChecker{},
				log:         newTestLogger(),
			}

			rec := httptest.NewRecorder()
			h.handleEvent(rec, newEventRequest("tenant-a", tt.eventID, &UserDetails{ID: 1}))

			if rec.Code != http.StatusNotFound {
				t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
			}
		})
	}
}

func TestSystemHandlers_Event_RequiresACaller(t *testing.T) {
	repo := &stubEventRepo{}
	h := &SystemHandlers{
		eventRepo:   repo,
		realmLister: &mockRealmLister{realms: realms("realmA")},
		rbacService: &mockRBACChecker{},
		log:         newTestLogger(),
	}

	rec := httptest.NewRecorder()
	h.handleEvent(rec, newEventRequest("tenant-a", "e1", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if repo.calls != 0 {
		t.Errorf("repository queried %d times without a caller, want 0", repo.calls)
	}
}

// The other tests inject route params, so they cannot see how chi decodes a
// path. This one routes for real: an id carrying a character Go keeps encoded
// in RawPath (":", "+", "/", "&", "=") used to reach the lookup still escaped,
// while a literal "%41" must survive untouched.
func TestSystemHandlers_Event_DecodesTheRoutedID(t *testing.T) {
	ids := []string{
		"0b6f1c2e-5d3a-4c1e-9f00-1a2b3c4d5e6f",
		"realm:with:colons-1790000000000-LOGIN",
		"plus+sign",
		"slash/inside",
		"query?x=1&y=2",
		"has space",
		"percent%41literal",
		"unicodé",
	}
	stored := make([]*domain.Event, 0, len(ids))
	for _, id := range ids {
		stored = append(stored, &domain.Event{TenantID: "tenant-a", EventID: id, Source: "keycloak:realmA"})
	}
	h := &SystemHandlers{
		eventRepo:   &stubEventRepo{events: stored},
		realmLister: &mockRealmLister{realms: realms("realmA")},
		rbacService: &mockRBACChecker{},
		log:         newTestLogger(),
	}
	router := chi.NewRouter()
	router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), UserInfoKey, &UserDetails{ID: 1})))
		})
	})
	router.Get("/api/tenants/{tenantID}/events/{eventID}", h.handleEvent)

	for _, id := range ids {
		t.Run(id, func(t *testing.T) {
			// url.PathEscape plus the characters encodeURIComponent also escapes,
			// which is what the web client sends.
			escaped := strings.NewReplacer(":", "%3A", "+", "%2B", "&", "%26", "=", "%3D").
				Replace(url.PathEscape(id))
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/tenants/tenant-a/events/"+escaped, nil))

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d for %q (sent %s), want 200; body = %s", rec.Code, id, escaped, rec.Body.String())
			}
			var got domain.Event
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if got.EventID != id {
				t.Errorf("event_id = %q, want %q", got.EventID, id)
			}
		})
	}
}
