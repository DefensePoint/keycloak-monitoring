package keycloak

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/rs/zerolog"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/pkg/keycloakadmin"
)

// Keycloak answers for a realm the monitoring client cannot view with a stub:
// no id and none of the settings. These tests cover what the realm-info
// collector does with such a realm: keep its stored row, not retire it, and
// say so in the log once.

// logBuffer collects JSON log lines; it is safe for the monitor's goroutines.
type logBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *logBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

// count returns how many lines have this level and message, for this tenant
// and realm.
func (b *logBuffer) count(level, message, tenantID, realm string) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := 0
	for _, line := range strings.Split(b.buf.String(), "\n") {
		var entry map[string]any
		if json.Unmarshal([]byte(line), &entry) != nil {
			continue
		}
		if entry["level"] == level && entry["message"] == message &&
			entry["tenant_id"] == tenantID && entry["realm"] == realm {
			n++
		}
	}
	return n
}

const (
	cannotViewRealmLog = "Monitoring client cannot view this realm's settings; keeping its stored realm information as it was"
	canViewRealmLog    = "Monitoring client can view this realm's settings again"
)

// realmAccessFixture is one tenant whose Keycloak and stored rows the test
// sets per collection.
type realmAccessFixture struct {
	client    *mockAdminAPI
	realmRepo *mockRealmRepository
	logs      *logBuffer
	monitor   *Monitor

	mu sync.Mutex
	// listed is what GetAllRealms returns.
	listed []*keycloakadmin.RealmRepresentation
	// byName is what GetRealmInfo returns per realm; a missing realm is a 404.
	byName map[string]*keycloakadmin.RealmRepresentation
	// errByName makes GetRealmInfo fail for a realm with that error.
	errByName map[string]error
	// stored are the tenant's live rows.
	stored []*domain.KeycloakRealmInfo
}

func newRealmAccessFixture(t *testing.T, tenantID string, configured []string) *realmAccessFixture {
	t.Helper()
	f := &realmAccessFixture{
		client:    newMockAdminAPI(),
		realmRepo: newMockRealmRepository(),
		logs:      &logBuffer{},
		byName:    map[string]*keycloakadmin.RealmRepresentation{},
		errByName: map[string]error{},
	}
	f.client.GetAllRealmsFunc = func(ctx context.Context) ([]*keycloakadmin.RealmRepresentation, error) {
		f.mu.Lock()
		defer f.mu.Unlock()
		return f.listed, nil
	}
	f.client.GetRealmInfoFunc = func(ctx context.Context, name string) (*keycloakadmin.RealmRepresentation, error) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if err, ok := f.errByName[name]; ok {
			return nil, err
		}
		if realm, ok := f.byName[name]; ok {
			return realm, nil
		}
		return nil, fmt.Errorf("failed to get realm info, status: 404: %w", keycloakadmin.ErrNotFound)
	}
	f.realmRepo.GetRealmsFunc = func(ctx context.Context, id string) ([]*domain.KeycloakRealmInfo, error) {
		f.mu.Lock()
		defer f.mu.Unlock()
		return f.stored, nil
	}

	cfg := newTestConfig()
	cfg.Realms = configured
	log := logger.New(zerolog.New(f.logs))
	f.monitor = NewMonitor(tenantID, "Tenant "+tenantID, f.client, cfg, log,
		newMockEventRepository(), newMockGeneralEventRepository(), newMockMetricsRepository(),
		newMockHealthRepository(), f.realmRepo, nil, nil)
	return f
}

func realRealm(id, name string) *keycloakadmin.RealmRepresentation {
	return &keycloakadmin.RealmRepresentation{ID: id, Realm: name, Enabled: true, EventsEnabled: true,
		EventsListeners: []string{"jboss-logging"}}
}

// stubRealm is what Keycloak answers for a realm the client cannot view.
func stubRealm(name string) *keycloakadmin.RealmRepresentation {
	return &keycloakadmin.RealmRepresentation{Realm: name}
}

func storedRealm(tenantID, id, name string) *domain.KeycloakRealmInfo {
	return &domain.KeycloakRealmInfo{TenantID: tenantID, RealmID: id, RealmName: name, Enabled: true}
}

func (f *realmAccessFixture) set(listed []*keycloakadmin.RealmRepresentation, byName ...*keycloakadmin.RealmRepresentation) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.listed = listed
	f.byName = map[string]*keycloakadmin.RealmRepresentation{}
	for _, realm := range byName {
		f.byName[realm.Realm] = realm
	}
}

func (f *realmAccessFixture) savedNames() []string {
	var names []string
	for _, realm := range f.realmRepo.getSavedRealms() {
		names = append(names, realm.RealmName)
	}
	return names
}

func (f *realmAccessFixture) lastKept(t *testing.T) []string {
	t.Helper()
	f.realmRepo.mu.Lock()
	defer f.realmRepo.mu.Unlock()
	if len(f.realmRepo.retiredWith) == 0 {
		t.Fatal("RetireRealmsNotIn was not called")
	}
	kept := slices.Clone(f.realmRepo.retiredWith[len(f.realmRepo.retiredWith)-1])
	slices.Sort(kept)
	return kept
}

// A listed realm the client cannot view comes back as a stub. Saving it
// replaced the realm's row with an empty one; retiring by id then removed the
// real row too.
func TestCollectRealmInfo_ListedStubKeepsItsRow(t *testing.T) {
	f := newRealmAccessFixture(t, "tenant-a", nil)
	f.stored = []*domain.KeycloakRealmInfo{storedRealm("tenant-a", "id-a", "alpha"), storedRealm("tenant-a", "id-b", "beta")}
	f.set([]*keycloakadmin.RealmRepresentation{realRealm("id-a", "alpha"), stubRealm("beta")})

	f.monitor.collectRealmInfo(context.Background())

	if got := f.savedNames(); !slices.Equal(got, []string{"alpha"}) {
		t.Errorf("saved realms = %v, want [alpha]: a stub must not overwrite the stored row", got)
	}
	if got := f.lastKept(t); !slices.Equal(got, []string{"id-a", "id-b"}) {
		t.Errorf("kept ids = %v, want [id-a id-b]: the unreadable realm's row must not be retired", got)
	}
	if n := f.logs.count("warn", cannotViewRealmLog, "tenant-a", "beta"); n != 1 {
		t.Errorf("cannot-view warnings for beta = %d, want 1", n)
	}
}

// With configured realms each one is fetched by name, and an unreadable one
// is a stub too.
func TestCollectRealmInfo_ConfiguredStubKeepsItsRow(t *testing.T) {
	f := newRealmAccessFixture(t, "tenant-a", []string{"alpha", "beta"})
	f.set(nil, realRealm("id-a", "alpha"), stubRealm("beta"))

	f.monitor.collectRealmInfo(context.Background())

	if got := f.savedNames(); !slices.Equal(got, []string{"alpha"}) {
		t.Errorf("saved realms = %v, want [alpha]", got)
	}
	if n := f.logs.count("warn", cannotViewRealmLog, "tenant-a", "beta"); n != 1 {
		t.Errorf("cannot-view warnings for beta = %d, want 1", n)
	}
}

// Two stubs in one tenant both had an empty id and were saved into the same
// row, which flipped between their names.
func TestCollectRealmInfo_TwoStubsInOneTenant(t *testing.T) {
	f := newRealmAccessFixture(t, "tenant-a", nil)
	f.stored = []*domain.KeycloakRealmInfo{
		storedRealm("tenant-a", "id-a", "alpha"), storedRealm("tenant-a", "id-b", "beta"), storedRealm("tenant-a", "id-c", "gamma"),
	}
	f.set([]*keycloakadmin.RealmRepresentation{realRealm("id-a", "alpha"), stubRealm("beta"), stubRealm("gamma")})

	f.monitor.collectRealmInfo(context.Background())

	if got := f.savedNames(); !slices.Equal(got, []string{"alpha"}) {
		t.Errorf("saved realms = %v, want [alpha]", got)
	}
	if got := f.lastKept(t); !slices.Equal(got, []string{"id-a", "id-b", "id-c"}) {
		t.Errorf("kept ids = %v, want [id-a id-b id-c]", got)
	}
	for _, realm := range []string{"beta", "gamma"} {
		if n := f.logs.count("warn", cannotViewRealmLog, "tenant-a", realm); n != 1 {
			t.Errorf("cannot-view warnings for %s = %d, want 1", realm, n)
		}
	}
}

// A client with no roles left in a realm no longer sees it listed, but
// Keycloak still answers 200 for it with a stub: it exists, so its row stays.
func TestCollectRealmInfo_UnlistedButAnsweringRealmKeepsItsRow(t *testing.T) {
	f := newRealmAccessFixture(t, "tenant-a", nil)
	f.stored = []*domain.KeycloakRealmInfo{storedRealm("tenant-a", "id-a", "alpha"), storedRealm("tenant-a", "id-b", "beta")}
	f.set([]*keycloakadmin.RealmRepresentation{realRealm("id-a", "alpha")}, stubRealm("beta"))

	f.monitor.collectRealmInfo(context.Background())

	if got := f.lastKept(t); !slices.Equal(got, []string{"id-a", "id-b"}) {
		t.Errorf("kept ids = %v, want [id-a id-b]", got)
	}
	if n := f.logs.count("warn", cannotViewRealmLog, "tenant-a", "beta"); n != 1 {
		t.Errorf("cannot-view warnings for beta = %d, want 1", n)
	}
}

// A realm deleted in Keycloak is a 404, and is retired as before.
func TestCollectRealmInfo_DeletedRealmIsRetired(t *testing.T) {
	f := newRealmAccessFixture(t, "tenant-a", nil)
	f.stored = []*domain.KeycloakRealmInfo{storedRealm("tenant-a", "id-a", "alpha"), storedRealm("tenant-a", "id-b", "beta")}
	f.set([]*keycloakadmin.RealmRepresentation{realRealm("id-a", "alpha")})

	f.monitor.collectRealmInfo(context.Background())

	if got := f.lastKept(t); !slices.Equal(got, []string{"id-a"}) {
		t.Errorf("kept ids = %v, want [id-a]: a deleted realm is retired", got)
	}
	if n := f.logs.count("warn", cannotViewRealmLog, "tenant-a", "beta"); n != 0 {
		t.Errorf("cannot-view warnings for a deleted realm = %d, want 0", n)
	}
}

// When Keycloak cannot say whether an unlisted realm still exists, its row
// stays: retiring it on a guess would drop a realm that may only be
// unreadable.
func TestCollectRealmInfo_UnlistedRealmLookupErrorKeepsItsRow(t *testing.T) {
	f := newRealmAccessFixture(t, "tenant-a", nil)
	f.stored = []*domain.KeycloakRealmInfo{storedRealm("tenant-a", "id-a", "alpha"), storedRealm("tenant-a", "id-b", "beta")}
	f.set([]*keycloakadmin.RealmRepresentation{realRealm("id-a", "alpha")})
	f.errByName["beta"] = fmt.Errorf("failed to get realm info, status: 503")

	f.monitor.collectRealmInfo(context.Background())

	if got := f.lastKept(t); !slices.Equal(got, []string{"id-a", "id-b"}) {
		t.Errorf("kept ids = %v, want [id-a id-b]", got)
	}
}

// The warning is logged when a realm becomes unreadable, not on every
// collection, and once more when it can be read again.
func TestCollectRealmInfo_LogsUnreadableOnceAndRecoveryOnce(t *testing.T) {
	f := newRealmAccessFixture(t, "tenant-a", nil)
	f.stored = []*domain.KeycloakRealmInfo{storedRealm("tenant-a", "id-a", "alpha"), storedRealm("tenant-a", "id-b", "beta")}
	ctx := context.Background()

	f.set([]*keycloakadmin.RealmRepresentation{realRealm("id-a", "alpha"), stubRealm("beta")})
	for range 3 {
		f.monitor.collectRealmInfo(ctx)
	}
	f.set([]*keycloakadmin.RealmRepresentation{realRealm("id-a", "alpha"), realRealm("id-b", "beta")})
	for range 2 {
		f.monitor.collectRealmInfo(ctx)
	}

	if n := f.logs.count("warn", cannotViewRealmLog, "tenant-a", "beta"); n != 1 {
		t.Errorf("cannot-view warnings = %d, want 1 over 3 unreadable collections", n)
	}
	if n := f.logs.count("info", canViewRealmLog, "tenant-a", "beta"); n != 1 {
		t.Errorf("can-view-again lines = %d, want 1", n)
	}
}

// A realm deleted while it was unreadable is not readable again; saying so
// would be false.
func TestCollectRealmInfo_DeletedWhileUnreadableIsNotARecovery(t *testing.T) {
	f := newRealmAccessFixture(t, "tenant-a", nil)
	f.stored = []*domain.KeycloakRealmInfo{storedRealm("tenant-a", "id-a", "alpha"), storedRealm("tenant-a", "id-b", "beta")}
	ctx := context.Background()

	f.set([]*keycloakadmin.RealmRepresentation{realRealm("id-a", "alpha"), stubRealm("beta")})
	f.monitor.collectRealmInfo(ctx)
	f.set([]*keycloakadmin.RealmRepresentation{realRealm("id-a", "alpha")})
	f.monitor.collectRealmInfo(ctx)

	if n := f.logs.count("info", canViewRealmLog, "tenant-a", "beta"); n != 0 {
		t.Errorf("can-view-again lines for a deleted realm = %d, want 0", n)
	}
	if got := f.lastKept(t); !slices.Equal(got, []string{"id-a"}) {
		t.Errorf("kept ids = %v, want [id-a]", got)
	}
}

// A collection that cannot list the realms reads nothing, so it neither
// reports a realm unreadable nor readable again.
func TestCollectRealmInfo_FailedListingChangesNothing(t *testing.T) {
	f := newRealmAccessFixture(t, "tenant-a", nil)
	f.stored = []*domain.KeycloakRealmInfo{storedRealm("tenant-a", "id-a", "alpha"), storedRealm("tenant-a", "id-b", "beta")}
	ctx := context.Background()

	f.set([]*keycloakadmin.RealmRepresentation{realRealm("id-a", "alpha"), stubRealm("beta")})
	f.monitor.collectRealmInfo(ctx)
	f.client.GetAllRealmsFunc = func(ctx context.Context) ([]*keycloakadmin.RealmRepresentation, error) {
		return nil, fmt.Errorf("keycloak unreachable")
	}
	f.monitor.collectRealmInfo(ctx)

	if n := f.logs.count("warn", cannotViewRealmLog, "tenant-a", "beta"); n != 1 {
		t.Errorf("cannot-view warnings = %d, want 1", n)
	}
	if n := f.logs.count("info", canViewRealmLog, "tenant-a", "beta"); n != 0 {
		t.Errorf("can-view-again lines = %d, want 0 after a failed listing", n)
	}
}

// Each tenant has its own monitor, so the same realm name in two tenants is
// tracked and logged separately.
func TestCollectRealmInfo_SameRealmNameInTwoTenants(t *testing.T) {
	a := newRealmAccessFixture(t, "tenant-a", nil)
	b := newRealmAccessFixture(t, "tenant-b", nil)
	a.stored = []*domain.KeycloakRealmInfo{storedRealm("tenant-a", "id-b", "beta")}
	b.stored = []*domain.KeycloakRealmInfo{storedRealm("tenant-b", "id-b", "beta")}
	ctx := context.Background()

	a.set([]*keycloakadmin.RealmRepresentation{stubRealm("beta")})
	b.set([]*keycloakadmin.RealmRepresentation{realRealm("id-b", "beta")})
	a.monitor.collectRealmInfo(ctx)
	b.monitor.collectRealmInfo(ctx)
	b.set([]*keycloakadmin.RealmRepresentation{stubRealm("beta")})
	b.monitor.collectRealmInfo(ctx)
	a.set([]*keycloakadmin.RealmRepresentation{realRealm("id-b", "beta")})
	a.monitor.collectRealmInfo(ctx)

	for _, tc := range []struct {
		f        *realmAccessFixture
		tenantID string
		warn     int
		recover  int
	}{
		{a, "tenant-a", 1, 1},
		{b, "tenant-b", 1, 0},
	} {
		if n := tc.f.logs.count("warn", cannotViewRealmLog, tc.tenantID, "beta"); n != tc.warn {
			t.Errorf("%s cannot-view warnings = %d, want %d", tc.tenantID, n, tc.warn)
		}
		if n := tc.f.logs.count("info", canViewRealmLog, tc.tenantID, "beta"); n != tc.recover {
			t.Errorf("%s can-view-again lines = %d, want %d", tc.tenantID, n, tc.recover)
		}
	}
	if got := b.savedNames(); !slices.Equal(got, []string{"beta"}) {
		t.Errorf("tenant-b saved %v, want [beta] once: its stub must not be saved", got)
	}
}

// marked returns every realm name the collector marked unreadable, sorted.
func (f *realmAccessFixture) marked() []string {
	f.realmRepo.mu.Lock()
	defer f.realmRepo.mu.Unlock()
	var names []string
	for _, call := range f.realmRepo.markedUnreadable {
		for _, name := range call {
			if !slices.Contains(names, name) {
				names = append(names, name)
			}
		}
	}
	slices.Sort(names)
	return names
}

// The collector marks the stored row of a realm it could not read, so the
// Realms page can say its data is not current; a realm it read, one that is
// gone, and one it could not judge are not marked.
func TestCollectRealmInfo_MarksOnlyUnreadableRealms(t *testing.T) {
	for _, tc := range []struct {
		name       string
		configured []string
		setup      func(f *realmAccessFixture)
		want       []string
	}{
		{name: "listed as a stub", setup: func(f *realmAccessFixture) {
			f.set([]*keycloakadmin.RealmRepresentation{realRealm("id-a", "alpha"), stubRealm("beta")})
		}, want: []string{"beta"}},
		{name: "configured, read as a stub", configured: []string{"alpha", "beta"}, setup: func(f *realmAccessFixture) {
			f.set(nil, realRealm("id-a", "alpha"), stubRealm("beta"))
		}, want: []string{"beta"}},
		{name: "unlisted, answering as a stub", setup: func(f *realmAccessFixture) {
			f.set([]*keycloakadmin.RealmRepresentation{realRealm("id-a", "alpha")}, stubRealm("beta"))
		}, want: []string{"beta"}},
		{name: "deleted", setup: func(f *realmAccessFixture) {
			f.set([]*keycloakadmin.RealmRepresentation{realRealm("id-a", "alpha")})
		}},
		{name: "unlisted, lookup failing", setup: func(f *realmAccessFixture) {
			f.set([]*keycloakadmin.RealmRepresentation{realRealm("id-a", "alpha")})
			f.errByName["beta"] = fmt.Errorf("failed to get realm info, status: 503")
		}},
		{name: "readable", setup: func(f *realmAccessFixture) {
			f.set([]*keycloakadmin.RealmRepresentation{realRealm("id-a", "alpha"), realRealm("id-b", "beta")})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newRealmAccessFixture(t, "tenant-a", tc.configured)
			f.stored = []*domain.KeycloakRealmInfo{storedRealm("tenant-a", "id-a", "alpha"), storedRealm("tenant-a", "id-b", "beta")}
			tc.setup(f)

			f.monitor.collectRealmInfo(context.Background())

			if got := f.marked(); !slices.Equal(got, tc.want) {
				t.Errorf("marked unreadable = %v, want %v", got, tc.want)
			}
		})
	}
}
