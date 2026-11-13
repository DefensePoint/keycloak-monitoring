package reports

import (
	"strings"
	"testing"
)

func renderReport(t *testing.T, d *Data) string {
	t.Helper()
	tmpl, err := loadReportTemplate("templates/report.html")
	if err != nil {
		t.Fatal(err)
	}
	var buf strings.Builder
	if err := tmpl.Execute(&buf, d); err != nil {
		t.Fatalf("render: %v", err)
	}
	return buf.String()
}

func TestReportTemplate_CarriesTheProductName(t *testing.T) {
	html := renderReport(t, &Data{ProductName: "Acme Monitoring Platform", TenantName: "Tenant A"})
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

func TestReportTemplate_CreditsTheVendor(t *testing.T) {
	html := renderReport(t, &Data{ProductName: "Acme Monitoring Platform", Vendor: "Acme", ContactEmail: "security@acme.example"})

	for _, want := range []string{
		"<strong>Authors:</strong> Acme Team",
		"<td>Acme Team</td>",
		"<td>Acme Security Team</td>",
		"<th>Email Addresses</th>",
		"<td>security@acme.example</td>",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("rendered report lacks %q", want)
		}
	}
	if strings.Contains(strings.ToLower(html), "defensepoint") {
		t.Error("rendered report still credits DefensePoint")
	}
}

func TestReportTemplate_LeavesOutAMissingContactEmail(t *testing.T) {
	html := renderReport(t, &Data{ProductName: "Acme Monitoring Platform", Vendor: "Acme"})

	for _, unwanted := range []string{"Email Addresses", "<strong>Email</strong>", "<td></td>"} {
		if strings.Contains(html, unwanted) {
			t.Errorf("rendered report has %q with no contact address", unwanted)
		}
	}
}
