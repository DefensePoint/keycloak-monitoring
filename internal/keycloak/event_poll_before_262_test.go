package keycloak

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/pkg/keycloakadmin"
)

// Keycloak before 26.2 reads dateFrom only as yyyy-MM-dd, answers anything
// else with a 400, ignores direction and lists events newest first. Polling
// such a Keycloak through the real client must still collect every event
// once.

type quietLogger struct{}

func (quietLogger) Info(string, ...any)  {}
func (quietLogger) Warn(string, ...any)  {}
func (quietLogger) Error(string, ...any) {}
func (quietLogger) Debug(string, ...any) {}

var keycloakDate = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// before262 serves k's events over HTTP the way Keycloak 26.0 does.
type before262 struct {
	k        *simKeycloak
	mu       sync.Mutex
	rejected int
}

func (s *before262) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	switch {
	case strings.HasSuffix(r.URL.Path, "/protocol/openid-connect/token"):
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "t", "expires_in": 300})
	case strings.HasSuffix(r.URL.Path, "/admin/serverinfo"):
		_ = json.NewEncoder(w).Encode(map[string]any{"systemInfo": map[string]any{"version": "26.0.6.vendor-00001"}})
	case strings.HasSuffix(r.URL.Path, "/events"):
		q := r.URL.Query()
		from := q.Get("dateFrom")
		if !keycloakDate.MatchString(from) {
			s.mu.Lock()
			s.rejected++
			s.mu.Unlock()
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "Invalid value for 'Date(From)', expected format is yyyy-MM-dd"})
			return
		}
		day, _ := time.ParseInLocation("2006-01-02", from, time.Local)
		first, _ := strconv.Atoi(q.Get("first"))
		max, _ := strconv.Atoi(q.Get("max"))

		s.k.mu.Lock()
		var matched []*keycloakadmin.EventRepresentation
		for _, se := range s.k.events {
			if !se.visibleAt.After(s.k.clock) && se.ev.Time >= day.UnixMilli() {
				matched = append(matched, se.ev)
			}
		}
		s.k.mu.Unlock()
		sort.SliceStable(matched, func(i, j int) bool { return matched[i].Time > matched[j].Time })
		if first > len(matched) {
			first = len(matched)
		}
		matched = matched[first:]
		if max > 0 && len(matched) > max {
			matched = matched[:max]
		}
		_ = json.NewEncoder(w).Encode(matched)
	default:
		http.NotFound(w, r)
	}
}

func TestCollectEvents_KeycloakBefore262CollectsEveryEventOnce(t *testing.T) {
	for seed := int64(1); seed <= 3; seed++ {
		t.Run(fmt.Sprintf("seed %d across midnight", seed), func(t *testing.T) {
			pollKeycloakBefore262(t, seed, time.Date(2026, 10, 14, 23, 40, 0, 0, time.Local))
		})
	}
}

func pollKeycloakBefore262(t *testing.T, seed int64, start time.Time) {
	const (
		pollEvery   = 30 * time.Second
		generateFor = 30 * time.Minute
		runFor      = 45 * time.Minute
	)
	rnd := rand.New(rand.NewSource(seed))
	nodeSkews := []time.Duration{-45 * time.Second, 0, 20 * time.Second}

	k := &simKeycloak{clock: start}
	srv := &before262{k: k}
	server := httptest.NewServer(srv)
	defer server.Close()

	client, err := keycloakadmin.NewClient(context.Background(), &keycloakadmin.ClientConfig{
		ServerURL:        server.URL,
		AdminRealm:       "master",
		ClientID:         "monitoring-service",
		ClientSecret:     "s",
		HTTPClient:       server.Client(),
		MaxEventsPerPoll: 1000,
	}, quietLogger{})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	repo := newSavedIDsEventRepo()
	cfg := newTestConfig()
	cfg.Events.LookbackDuration = time.Minute
	m := NewMonitor("tenant-1", "Test Tenant", client, cfg, newTestLogger(),
		repo, newMockGeneralEventRepository(), newMockMetricsRepository(), newMockHealthRepository(), newMockRealmRepository(), nil, nil)
	m.now = func() time.Time {
		k.mu.Lock()
		defer k.mu.Unlock()
		return k.clock
	}

	n := 0
	for now := start; now.Before(start.Add(runFor)); now = now.Add(pollEvery) {
		k.mu.Lock()
		if now.Before(start.Add(generateFor)) {
			for i := 0; i < 5+rnd.Intn(15); i++ {
				happened := now.Add(time.Duration(rnd.Int63n(int64(pollEvery))))
				var delay time.Duration
				if rnd.Float64() < 0.2 {
					delay = time.Duration(rnd.Int63n(int64(defaultLateCommitWindow - time.Second)))
				}
				stamped := happened.Add(nodeSkews[rnd.Intn(len(nodeSkews))])
				k.events = append(k.events, simEvent{
					ev:        &keycloakadmin.EventRepresentation{ID: fmt.Sprintf("e%05d", n), Time: stamped.UnixMilli(), Type: "LOGIN", RealmID: "realm-1"},
					visibleAt: happened.Add(delay),
				})
				n++
			}
		}
		k.clock = now
		k.mu.Unlock()
		m.collectEvents(context.Background())
	}

	if srv.rejected > 0 {
		t.Errorf("Keycloak rejected %d events requests; the client must send it a date", srv.rejected)
	}
	missing, repeated := 0, 0
	for _, se := range k.events {
		switch c := repo.count(se.ev.ID); {
		case c == 0:
			missing++
		case c > 1:
			repeated++
		}
	}
	if missing > 0 || repeated > 0 {
		t.Errorf("of %d events, %d were never saved and %d saved more than once", n, missing, repeated)
	}
}
