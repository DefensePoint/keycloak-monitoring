package metrics_test

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"repos.defensepoint.com/iam/oss-pmt/kmt/internal/metrics"
)

func TestRecorderTenantGauges(t *testing.T) {
	m := metrics.New()

	m.SetTenantMonitorUp("t1", "Tenant One", true)
	if got := testutil.ToFloat64(m.TenantMonitorUp.WithLabelValues("t1", "Tenant One")); got != 1 {
		t.Fatalf("expected up=1, got %v", got)
	}

	m.SetTenantMonitorUp("t1", "Tenant One", false)
	if got := testutil.ToFloat64(m.TenantMonitorUp.WithLabelValues("t1", "Tenant One")); got != 0 {
		t.Fatalf("expected up=0, got %v", got)
	}

	m.SetTenantMonitorsActive(3)
	if got := testutil.ToFloat64(m.TenantMonitorsActive); got != 3 {
		t.Fatalf("expected 3 active, got %v", got)
	}
}

func TestRecorderCounters(t *testing.T) {
	m := metrics.New()

	m.AddEventsCollected("t1", "master", 5)
	m.AddEventsCollected("t1", "master", 3)
	if got := testutil.ToFloat64(m.EventsCollected.WithLabelValues("t1", "master")); got != 8 {
		t.Fatalf("expected 8 events, got %v", got)
	}

	m.IncAlert("critical", "realm_security")
	if got := testutil.ToFloat64(m.AlertsTotal.WithLabelValues("critical", "realm_security")); got != 1 {
		t.Fatalf("expected 1 alert, got %v", got)
	}

	m.IncNotification("slack", "success")
	m.IncNotification("slack", "failure")
	if got := testutil.ToFloat64(m.NotificationsSent.WithLabelValues("slack", "success")); got != 1 {
		t.Fatalf("expected 1 success, got %v", got)
	}
	if got := testutil.ToFloat64(m.NotificationsSent.WithLabelValues("slack", "failure")); got != 1 {
		t.Fatalf("expected 1 failure, got %v", got)
	}

	m.IncPollCycleError("t1", "events")
	if got := testutil.ToFloat64(m.PollCycleErrors.WithLabelValues("t1", "events")); got != 1 {
		t.Fatalf("expected 1 poll error, got %v", got)
	}
}

func TestAddEventsCollectedIgnoresNonPositive(t *testing.T) {
	m := metrics.New()
	m.AddEventsCollected("t1", "master", 0)
	m.AddEventsCollected("t1", "master", -2)

	if got := testutil.ToFloat64(m.EventsCollected.WithLabelValues("t1", "master")); got != 0 {
		t.Fatalf("expected counter untouched, got %v", got)
	}
}

func TestObservePollCycle(t *testing.T) {
	m := metrics.New()
	m.ObservePollCycle("t1", "events", 250*time.Millisecond)

	if got := testutil.CollectAndCount(m.PollCycleDuration); got == 0 {
		t.Fatal("expected poll cycle observation to be recorded")
	}
}

func TestNopRecorderSatisfiesInterface(t *testing.T) {
	var r metrics.Recorder = metrics.NopRecorder{}
	r.SetTenantMonitorUp("t", "n", true)
	r.SetTenantMonitorsActive(1)
	r.ObservePollCycle("t", "events", time.Second)
	r.IncPollCycleError("t", "events")
	r.AddEventsCollected("t", "r", 1)
	r.IncAlert("info", "check")
	r.IncNotification("slack", "success")
}

func TestRegistrySatisfiesRecorder(t *testing.T) {
	var _ metrics.Recorder = metrics.New()
}
