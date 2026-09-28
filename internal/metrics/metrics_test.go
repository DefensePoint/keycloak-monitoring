package metrics_test

import (
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/DefensePoint/keycloak-monitoring/internal/metrics"
)

func TestNewRegistersCollectors(t *testing.T) {
	m := metrics.New()

	m.HTTPRequestsTotal.WithLabelValues("/api/tenants", "GET", "200").Inc()

	if got := testutil.ToFloat64(m.HTTPRequestsTotal.WithLabelValues("/api/tenants", "GET", "200")); got != 1 {
		t.Fatalf("expected counter to be 1, got %v", got)
	}
}

func TestSetBuildInfo(t *testing.T) {
	m := metrics.New()
	m.SetBuildInfo("1.2.3", "abc123")

	families, err := m.Gatherer().Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}

	for _, f := range families {
		if f.GetName() == "pmp_build_info" {
			labels := f.GetMetric()[0].GetLabel()
			found := map[string]string{}
			for _, l := range labels {
				found[l.GetName()] = l.GetValue()
			}
			if found["version"] != "1.2.3" || found["commit"] != "abc123" {
				t.Fatalf("unexpected build info labels: %v", found)
			}
			return
		}
	}
	t.Fatal("pmp_build_info not found")
}

func TestGathererExposesNamespacedMetrics(t *testing.T) {
	m := metrics.New()
	m.AlertsTotal.WithLabelValues("critical", "realm_security").Inc()

	families, err := m.Gatherer().Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}

	var names []string
	for _, f := range families {
		names = append(names, f.GetName())
	}
	joined := strings.Join(names, ",")

	for _, want := range []string{"pmp_alerts_total", "go_goroutines"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("expected %q in gathered metrics, got %s", want, joined)
		}
	}
}
