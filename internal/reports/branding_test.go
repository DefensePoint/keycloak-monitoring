package reports

import (
	"strings"
	"testing"
)

func TestReportTemplate_CarriesTheProductName(t *testing.T) {
	tmpl, err := loadReportTemplate("templates/report.html")
	if err != nil {
		t.Fatal(err)
	}
	var buf strings.Builder
	if err := tmpl.Execute(&buf, &Data{ProductName: "Acme Monitoring Platform", TenantName: "Tenant A"}); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := buf.String()
	for _, want := range []string{
		"<title>Tenant A Acme Monitoring Platform Report</title>",
		"Tenant A<br>Acme Monitoring Platform Report<br>",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("rendered report lacks %q", want)
		}
	}
	if strings.Contains(html, "Keycloak Monitoring Tool") {
		t.Error("rendered report still names Keycloak Monitoring Tool")
	}
}
