package tenant

import (
	"context"
	"testing"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

func TestNormalizeKeycloakVersion(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"", "", false},
		{"   ", "", false},
		{"26.7.5", "26.7.5", false},
		{" 26.7.5 ", "26.7.5", false},
		{"26.7", "26.7", false},
		{"26.6.9.redhat-00002", "26.6.9.redhat-00002", false},
		{"26.0.0-SNAPSHOT", "26.0.0-SNAPSHOT", false},
		{"26", "", true},
		{"v26.7.5", "", true},
		{"latest", "", true},
		{"26.7.5 or so", "", true},
		{"26.7.5.1.2", "", true},
		{"26." + string(make([]byte, 70)), "", true},
	}
	for _, tt := range tests {
		got, err := normalizeKeycloakVersion(tt.in)
		if (err != nil) != tt.wantErr {
			t.Errorf("normalizeKeycloakVersion(%q) error = %v, want error %v", tt.in, err, tt.wantErr)
			continue
		}
		if got != tt.want {
			t.Errorf("normalizeKeycloakVersion(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func versionCreateRequest(version string) *CreateRequest {
	return &CreateRequest{
		TenantID:        "t1",
		Name:            "T1",
		ServerURL:       "https://keycloak.example.com",
		AdminRealm:      "master",
		ClientID:        "monitoring-service",
		ClientSecret:    "secret",
		KeycloakVersion: version,
	}
}

// The entered version is kept with the time it was set. Saving the same value
// again, as every config sync at startup and every unrelated form save does,
// must not move that time, or the page would always call the version new.
func TestKeycloakVersionIsStoredWithWhenItWasSet(t *testing.T) {
	ctx := context.Background()
	svc := NewService(&mockRepository{})

	created, err := svc.CreateTenant(ctx, versionCreateRequest(" 26.7.5 "))
	if err != nil {
		t.Fatalf("CreateTenant: %v", err)
	}
	if created.KeycloakVersion != "26.7.5" || created.KeycloakVersionSetAt == nil {
		t.Fatalf("created with %q set at %v, want 26.7.5 with a time", created.KeycloakVersion, created.KeycloakVersionSetAt)
	}
	firstSet := *created.KeycloakVersionSetAt

	same := "26.7.5"
	name := "Renamed"
	updated, err := svc.UpdateTenant(ctx, "t1", &UpdateRequest{Name: &name, KeycloakVersion: &same})
	if err != nil {
		t.Fatalf("UpdateTenant same version: %v", err)
	}
	if updated.KeycloakVersionSetAt == nil || !updated.KeycloakVersionSetAt.Equal(firstSet) {
		t.Errorf("saving the same version moved its time from %v to %v", firstSet, updated.KeycloakVersionSetAt)
	}

	updated, err = svc.UpdateTenant(ctx, "t1", &UpdateRequest{Name: &name})
	if err != nil {
		t.Fatalf("UpdateTenant without a version: %v", err)
	}
	if updated.KeycloakVersion != "26.7.5" || !updated.KeycloakVersionSetAt.Equal(firstSet) {
		t.Errorf("an update without a version changed it to %q set at %v", updated.KeycloakVersion, updated.KeycloakVersionSetAt)
	}

	time.Sleep(time.Millisecond)
	newer := "26.8.1"
	updated, err = svc.UpdateTenant(ctx, "t1", &UpdateRequest{KeycloakVersion: &newer})
	if err != nil {
		t.Fatalf("UpdateTenant new version: %v", err)
	}
	if updated.KeycloakVersion != "26.8.1" || updated.KeycloakVersionSetAt == nil || !updated.KeycloakVersionSetAt.After(firstSet) {
		t.Errorf("a new version gave %q set at %v, want 26.8.1 set after %v", updated.KeycloakVersion, updated.KeycloakVersionSetAt, firstSet)
	}

	bad := "latest"
	if _, err := svc.UpdateTenant(ctx, "t1", &UpdateRequest{KeycloakVersion: &bad}); err == nil {
		t.Error("an update to a version that is not one was accepted")
	}
	current, _ := svc.GetTenant(ctx, "t1")
	if current.KeycloakVersion != "26.8.1" {
		t.Errorf("a refused update changed the version to %q", current.KeycloakVersion)
	}

	none := ""
	updated, err = svc.UpdateTenant(ctx, "t1", &UpdateRequest{KeycloakVersion: &none})
	if err != nil {
		t.Fatalf("UpdateTenant removing the version: %v", err)
	}
	if updated.KeycloakVersion != "" || updated.KeycloakVersionSetAt != nil {
		t.Errorf("removing the version left %q set at %v", updated.KeycloakVersion, updated.KeycloakVersionSetAt)
	}
}

func TestCreateTenantRefusesAVersionThatIsNotOne(t *testing.T) {
	svc := NewService(&mockRepository{})
	if _, err := svc.CreateTenant(context.Background(), versionCreateRequest("v26")); err == nil {
		t.Fatal("CreateTenant accepted keycloak_version v26")
	}
}

func TestCreateTenantWithoutAVersionHasNone(t *testing.T) {
	svc := NewService(&mockRepository{})
	created, err := svc.CreateTenant(context.Background(), versionCreateRequest(""))
	if err != nil {
		t.Fatalf("CreateTenant: %v", err)
	}
	if created.KeycloakVersion != "" || created.KeycloakVersionSetAt != nil {
		t.Errorf("a tenant created without a version has %q set at %v", created.KeycloakVersion, created.KeycloakVersionSetAt)
	}
}

// A clone made for an update must not share the set-at time with the cached
// tenant, or a refused update could still change what readers see.
func TestCloneTenantCopiesTheVersionTime(t *testing.T) {
	at := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	orig := &domain.KeycloakTenant{KeycloakVersion: "26.7.5", KeycloakVersionSetAt: &at}
	clone := cloneTenant(orig)
	*clone.KeycloakVersionSetAt = at.Add(time.Hour)
	if !orig.KeycloakVersionSetAt.Equal(at) {
		t.Error("changing the clone's version time changed the original's")
	}
}
