package reports

import (
	"strings"
	"testing"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

// The report gathers the same data the alerts, events and operator metrics
// endpoints guard with alerts:read, keycloak:read and metrics:read. It used to
// need only tenant access, so a caller without those permissions got the data
// in the PDF anyway.

func accessService() (*service, *mockAlertReader, *mockEventLister, *mockOperatorReader) {
	el := &mockEventLister{events: []*domain.Event{{Type: "LOGIN", Source: "keycloak:realmA"}}}
	ar := &mockAlertReader{alertsList: []*domain.Alert{{RealmName: "realmA", Title: "Secret finding", Severity: domain.AlertSeverityCritical}}}
	or := &mockOperatorReader{summaries: []*domain.OperatorMetricsSummary{{OperatorEmail: "op@example.com"}}}
	rl := &mockRealmLister{realms: realmInfos("realmA")}
	return NewService(el, ar, or, rl, testLogger(), nil).(*service), ar, el, or
}

func TestAggregateData_LeavesOutWhatTheCallerMayNotSee(t *testing.T) {
	for _, tc := range []struct {
		name   string
		access SectionAccess
	}{
		{"nothing", SectionAccess{}},
		{"alerts only", SectionAccess{Alerts: true}},
		{"events only", SectionAccess{Events: true}},
		{"operators only", SectionAccess{Operators: true}},
		{"everything", SectionAccess{Alerts: true, Events: true, Operators: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, ar, el, or := accessService()
			r := req()
			r.Access = tc.access
			data, err := svc.AggregateData(t.Context(), r)
			if err != nil {
				t.Fatal(err)
			}
			if ar.called != tc.access.Alerts || el.called != tc.access.Events || or.called != tc.access.Operators {
				t.Fatalf("queried alerts=%v events=%v operators=%v, access %+v", ar.called, el.called, or.called, tc.access)
			}
			if !tc.access.Alerts && (data.TotalAlerts != 0 || len(data.AlertList) != 0 || !data.AlertsOmitted) {
				t.Fatalf("alerts leaked or not marked omitted: total=%d list=%d omitted=%v", data.TotalAlerts, len(data.AlertList), data.AlertsOmitted)
			}
			if !tc.access.Events && data.TotalEvents != 0 {
				t.Fatalf("events counted without access: %d", data.TotalEvents)
			}
			if !tc.access.Operators && len(data.OperatorMetrics) != 0 {
				t.Fatalf("operator metrics leaked: %d", len(data.OperatorMetrics))
			}
			wantOmitted := 0
			if !tc.access.Alerts {
				wantOmitted++
			}
			if !tc.access.Operators {
				wantOmitted++
			}
			if len(data.Omitted) != wantOmitted {
				t.Fatalf("omitted = %+v, want %d entries", data.Omitted, wantOmitted)
			}
		})
	}
}

// The PDF itself must not carry the alert or operator data, and must say what
// was left out.
func TestReportTemplate_HidesOmittedSections(t *testing.T) {
	svc, _, _, _ := accessService()
	r := req()
	r.Access = SectionAccess{Events: true}
	data, err := svc.AggregateData(t.Context(), r)
	if err != nil {
		t.Fatal(err)
	}
	tmpl, err := loadReportTemplate("templates/report.html")
	if err != nil {
		t.Fatal(err)
	}
	var buf strings.Builder
	if err := tmpl.Execute(&buf, data); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := buf.String()
	for _, leaked := range []string{"Secret finding", "op@example.com", "Operator Performance Metrics"} {
		if strings.Contains(html, leaked) {
			t.Errorf("rendered report contains %q", leaked)
		}
	}
	for _, want := range []string{"Not included in this report", "requires alerts:read", "requires metrics:read"} {
		if !strings.Contains(html, want) {
			t.Errorf("rendered report lacks %q", want)
		}
	}
}
