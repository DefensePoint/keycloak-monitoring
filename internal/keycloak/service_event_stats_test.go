package keycloak

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// countingEventRepo answers CountEventsByType from a table and fails the one
// event type named in failType. Every other EventRepository method is left to
// the embedded nil interface: GetEventStats must not call them.
type countingEventRepo struct {
	EventRepository
	counts   map[string]int64
	failType string
	asked    []string
}

var errCountFailed = errors.New("count query failed")

func (r *countingEventRepo) CountEventsByType(_ context.Context, _, _, eventType string, _, _ time.Time) (int64, error) {
	r.asked = append(r.asked, eventType)
	if eventType == r.failType {
		return 0, errCountFailed
	}
	return r.counts[eventType], nil
}

func newStatsService(repo EventRepository) Service {
	return NewService(repo, nil, nil, nil, nil, nil)
}

func TestGetEventStats_SumsEveryCount(t *testing.T) {
	repo := &countingEventRepo{counts: map[string]int64{
		"LOGIN": 5, "LOGIN_ERROR": 3, "LOGOUT": 2, "REGISTER": 1, "CODE_TO_TOKEN": 7,
	}}
	from, to := time.Now().Add(-time.Hour), time.Now()

	stats, err := newStatsService(repo).GetEventStats(context.Background(), "tenant-a", "realmB", from, to)
	if err != nil {
		t.Fatalf("GetEventStats: %v", err)
	}
	if stats.LoginCount != 5 || stats.LoginErrorCount != 3 || stats.LogoutCount != 2 ||
		stats.RegisterCount != 1 || stats.CodeToTokenCount != 7 {
		t.Errorf("counts = %+v, want 5/3/2/1/7", stats)
	}
	if stats.TotalEvents != 18 {
		t.Errorf("TotalEvents = %d, want 18", stats.TotalEvents)
	}
	if stats.Realm != "realmB" || !stats.Start.Equal(from) || !stats.End.Equal(to) {
		t.Errorf("realm/window = %q %v %v, want realmB and the requested window", stats.Realm, stats.Start, stats.End)
	}
}

// A failing count used to become a silent zero on the dashboard tile while
// the request still succeeded. Each count must now fail the call instead.
func TestGetEventStats_AFailedCountIsAnErrorNotAZero(t *testing.T) {
	for _, eventType := range []string{"LOGIN", "LOGIN_ERROR", "LOGOUT", "REGISTER", "CODE_TO_TOKEN"} {
		t.Run(eventType, func(t *testing.T) {
			repo := &countingEventRepo{counts: map[string]int64{"LOGIN": 5}, failType: eventType}

			stats, err := newStatsService(repo).GetEventStats(context.Background(), "tenant-a", "realmB", time.Now().Add(-time.Hour), time.Now())
			if err == nil {
				t.Fatalf("GetEventStats returned stats %+v and no error while the %s count failed", stats, eventType)
			}
			if stats != nil {
				t.Errorf("stats = %+v, want nil alongside the error", stats)
			}
			if !errors.Is(err, errCountFailed) {
				t.Errorf("error %v does not wrap the repository error", err)
			}
			if !strings.Contains(err.Error(), eventType) || !strings.Contains(err.Error(), "realmB") {
				t.Errorf("error %q does not name the event type and realm", err)
			}
		})
	}
}
