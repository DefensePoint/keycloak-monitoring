package chi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

// The time-range picker sends an explicit start and end. An endpoint that reads
// only hours answers a different window than the one asked for, silently, so
// every time-scoped Keycloak endpoint must honour the pair.

type windowStubService struct {
	*stubKeycloakService
	windows [][2]time.Time
}

func (s *windowStubService) record(from, to time.Time) {
	s.windows = append(s.windows, [2]time.Time{from, to})
}

func (s *windowStubService) GetHealthHistory(_ context.Context, _ string, from, to time.Time) ([]*domain.KeycloakHealth, error) {
	s.record(from, to)
	return nil, nil
}

func (s *windowStubService) GetMetricsHistoryByRealm(_ context.Context, _, _ string, from, to time.Time) ([]*domain.KeycloakMetrics, error) {
	s.record(from, to)
	return nil, nil
}

func (s *windowStubService) ListEventsByRealm(_ context.Context, _, _ string, from, to time.Time, _, _ int) ([]*domain.KeycloakEvent, error) {
	s.record(from, to)
	return nil, nil
}

func (s *windowStubService) ListEventsByType(_ context.Context, _, _, _ string, from, to time.Time, _, _ int) ([]*domain.KeycloakEvent, error) {
	s.record(from, to)
	return nil, nil
}

// windowEndpoints are the time-scoped endpoints, each with the window it
// answers when the request names none.
var windowEndpoints = []struct {
	name        string
	path        string
	defaultSpan time.Duration
	maxSpan     time.Duration // 0: no limit on an explicit range
}{
	{"health history", "/api/tenants/tenant-a/keycloak/health/history?x=1", 24 * time.Hour, 7 * 24 * time.Hour},
	{"metrics history", "/api/tenants/tenant-a/keycloak/metrics/history?realm=realmA", 24 * time.Hour, 7 * 24 * time.Hour},
	{"events", "/api/tenants/tenant-a/keycloak/events?realm=realmA", time.Hour, 0},
	{"events by type", "/api/tenants/tenant-a/keycloak/events?realm=realmA&type=LOGIN_ERROR", time.Hour, 0},
}

func getWindow(t *testing.T, path string) (*windowStubService, *httptest.ResponseRecorder) {
	t.Helper()
	svc := &windowStubService{stubKeycloakService: &stubKeycloakService{
		realms: []*domain.KeycloakRealmInfo{{TenantID: "tenant-a", RealmName: "realmA"}},
	}}
	router := setupTestKeycloakRouter(newTestKeycloakHandlers(svc, &mockRBACChecker{isAdmin: true}))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return svc, rec
}

func rangeQuery(start, end time.Time) string {
	return "&start=" + start.Format(time.RFC3339) + "&end=" + end.Format(time.RFC3339)
}

func onlyWindow(t *testing.T, svc *windowStubService, rec *httptest.ResponseRecorder) (time.Time, time.Time) {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}
	if len(svc.windows) != 1 {
		t.Fatalf("expected one read, got %d", len(svc.windows))
	}
	return svc.windows[0][0].UTC(), svc.windows[0][1].UTC()
}

func TestTimeWindow_ExplicitRangeIsHonoured(t *testing.T) {
	// Entirely in the past, so a handler that ignores it and looks back from
	// now cannot pass by accident.
	end := time.Now().UTC().Add(-10 * 24 * time.Hour).Truncate(time.Second)
	start := end.Add(-3 * 24 * time.Hour)

	for _, ep := range windowEndpoints {
		t.Run(ep.name, func(t *testing.T) {
			svc, rec := getWindow(t, ep.path+rangeQuery(start, end))
			from, to := onlyWindow(t, svc, rec)
			if !from.Equal(start) || !to.Equal(end) {
				t.Errorf("window = %s .. %s, want the requested %s .. %s", from, to, start, end)
			}
		})
	}
}

func TestTimeWindow_HoursAndDefaultsUnchanged(t *testing.T) {
	for _, ep := range windowEndpoints {
		t.Run(ep.name+"/default", func(t *testing.T) {
			svc, rec := getWindow(t, ep.path)
			from, to := onlyWindow(t, svc, rec)
			if got := to.Sub(from); got != ep.defaultSpan {
				t.Errorf("default window = %s, want %s", got, ep.defaultSpan)
			}
		})
		t.Run(ep.name+"/hours", func(t *testing.T) {
			svc, rec := getWindow(t, ep.path+"&hours=5")
			from, to := onlyWindow(t, svc, rec)
			if got := to.Sub(from); got != 5*time.Hour {
				t.Errorf("hours=5 window = %s, want 5h", got)
			}
		})
		t.Run(ep.name+"/hours above the cap", func(t *testing.T) {
			svc, rec := getWindow(t, ep.path+"&hours=1000")
			from, to := onlyWindow(t, svc, rec)
			if got := to.Sub(from); got != 168*time.Hour {
				t.Errorf("hours=1000 window = %s, want the existing 168h cap", got)
			}
		})
	}
}

// A malformed or inverted pair degrades to the default rather than erroring.
func TestTimeWindow_BadRangeFallsBackToDefault(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	for _, ep := range windowEndpoints {
		for name, q := range map[string]string{
			"malformed": "&start=yesterday&end=today",
			"inverted":  rangeQuery(now, now.Add(-2*time.Hour)),
			"only one":  "&start=" + now.Add(-2*time.Hour).Format(time.RFC3339),
		} {
			t.Run(ep.name+"/"+name, func(t *testing.T) {
				svc, rec := getWindow(t, ep.path+q)
				from, to := onlyWindow(t, svc, rec)
				if got := to.Sub(from); got != ep.defaultSpan {
					t.Errorf("window = %s, want the default %s", got, ep.defaultSpan)
				}
			})
		}
	}
}

// The history endpoints return every row in the window, unpaginated, so an
// explicit range is limited; a longer one is refused, never silently trimmed.
// The events endpoint is bounded by its row limit instead and takes any range.
func TestTimeWindow_RangeLimit(t *testing.T) {
	end := time.Now().UTC().Truncate(time.Second)
	for _, ep := range windowEndpoints {
		if ep.maxSpan == 0 {
			t.Run(ep.name+"/long range accepted", func(t *testing.T) {
				start := end.Add(-30 * 24 * time.Hour)
				svc, rec := getWindow(t, ep.path+rangeQuery(start, end))
				from, to := onlyWindow(t, svc, rec)
				if !from.Equal(start) || !to.Equal(end) {
					t.Errorf("window = %s .. %s, want the requested %s .. %s", from, to, start, end)
				}
			})
			continue
		}
		t.Run(ep.name+"/at the limit", func(t *testing.T) {
			start := end.Add(-ep.maxSpan)
			svc, rec := getWindow(t, ep.path+rangeQuery(start, end))
			from, to := onlyWindow(t, svc, rec)
			if !from.Equal(start) || !to.Equal(end) {
				t.Errorf("window = %s .. %s, want the requested %s .. %s", from, to, start, end)
			}
		})
		t.Run(ep.name+"/over the limit", func(t *testing.T) {
			start := end.Add(-ep.maxSpan - time.Second)
			svc, rec := getWindow(t, ep.path+rangeQuery(start, end))
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body = %s", rec.Code, rec.Body.String())
			}
			if len(svc.windows) != 0 {
				t.Errorf("history read %d times for a refused range, want 0", len(svc.windows))
			}
		})
	}
}
