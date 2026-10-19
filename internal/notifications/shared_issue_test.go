package notifications_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/config"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/internal/notifications"
)

// The open alerts on one identity provider share one GitLab issue, and that
// issue is closed only by the last of them to resolve. Resolving one used to
// close the issue the others were still pointing at, and the provider was
// matched by a loose substring of its name that ignored the realm, so
// unrelated providers shared an issue too.

func idpAlert(alertID, provider, realm, title string) *domain.Alert {
	return &domain.Alert{
		TenantID: "tenant-1", AlertID: alertID, Title: title,
		Type: domain.AlertTypeIdentityProvider, Severity: domain.AlertSeverityWarning,
		Status: domain.AlertStatusActive, ResourceType: "identity_provider",
		ResourceID: provider + "-internal", ResourceName: provider, RealmName: realm,
	}
}

// gitLabService returns a service with only GitLab enabled, pointed at a
// server that records every call and gives each created issue its own IID.
func gitLabService(t *testing.T, repo *mockRepository, rec *recorder) notifications.Service {
	t.Helper()
	var issues int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		raw, _ := io.ReadAll(req.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		rec.mu.Lock()
		rec.requests = append(rec.requests, recordedRequest{
			Method: req.Method, Path: req.URL.EscapedPath(),
			Token: req.Header.Get("PRIVATE-TOKEN"), Body: body,
		})
		if req.Method == http.MethodPost && strings.HasSuffix(req.URL.Path, "/issues") {
			issues++
		}
		iid := issues
		if n, err := strconv.Atoi(path.Base(req.URL.Path)); err == nil {
			iid = n
		}
		if rec.closed == nil {
			rec.closed = map[int]bool{}
		}
		if rec.titles == nil {
			rec.titles = map[int]string{}
		}
		if req.Method == http.MethodPut && rec.refuseUpdates {
			rec.mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_, _ = io.WriteString(w, `{"message":"403 Forbidden"}`)
			return
		}
		switch body["state_event"] {
		case "close":
			rec.closed[iid] = true
		case "reopen":
			rec.closed[iid] = false
		}
		if title, ok := body["title"].(string); ok {
			rec.titles[iid] = title
		}
		state := "opened"
		if rec.closed[iid] {
			state = "closed"
		}
		title := rec.titles[iid]
		rec.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		if req.Method == http.MethodPost {
			w.WriteHeader(http.StatusCreated)
		} else {
			w.WriteHeader(http.StatusOK)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"id": 1000 + iid, "iid": iid, "state": state, "title": title})
	}))
	t.Cleanup(srv.Close)

	return notifications.NewFullService(repo, &config.NotificationsConfig{
		GitLab: config.GitLabConfig{
			Enabled: true, URL: srv.URL, Token: "t", ProjectID: "g/p", MinSeverity: "info",
		},
	}, logger.NewNoop())
}

// created returns the paths of the issue-creating calls.
func created(rec *recorder) []string {
	var paths []string
	for _, r := range rec.all() {
		if r.Method == http.MethodPost && strings.HasSuffix(r.Path, "/issues") {
			paths = append(paths, r.Path)
		}
	}
	return paths
}

func TestNotifyAlert_PutsOneProvidersAlertsOnOneIssue(t *testing.T) {
	var rec recorder
	repo := newMockRepository()
	svc := gitLabService(t, repo, &rec)

	missing := idpAlert("idp-metadata-1", "okta", "prod", "SAML Identity Provider 'okta' missing metadata descriptor URL")
	expiring := idpAlert("idp-cert-1", "okta", "prod", "SAML Identity Provider 'okta' certificate expiring soon")
	repo.track(missing, expiring)

	ctx := context.Background()
	if err := svc.NotifyAlert(ctx, missing); err != nil {
		t.Fatalf("NotifyAlert: %v", err)
	}
	if err := svc.NotifyAlert(ctx, expiring); err != nil {
		t.Fatalf("NotifyAlert: %v", err)
	}

	if issues := created(&rec); len(issues) != 1 {
		t.Fatalf("created %d issues for one provider, want 1: %v", len(issues), issues)
	}
	if len(repo.loggedEntries) != 2 {
		t.Fatalf("logged %d notifications, want one per alert", len(repo.loggedEntries))
	}
	first, second := repo.loggedEntries[0], repo.loggedEntries[1]
	if second.ExternalID != first.ExternalID {
		t.Errorf("the second alert holds issue %q, want the first alert's %q",
			second.ExternalID, first.ExternalID)
	}

	var note string
	for _, r := range rec.all() {
		if strings.HasSuffix(r.Path, "/notes") {
			note, _ = r.Body["body"].(string)
		}
	}
	if !strings.Contains(note, "certificate expiring soon") {
		t.Errorf("the shared issue got comment %q, want it to name the alert that joined", note)
	}
}

func TestNotifyAlert_KeepsProvidersWithOverlappingNamesApart(t *testing.T) {
	var rec recorder
	repo := newMockRepository()
	svc := gitLabService(t, repo, &rec)

	eu := idpAlert("idp-metadata-eu", "okta-eu", "prod", "SAML Identity Provider 'okta-eu' metadata fetch failed")
	okta := idpAlert("idp-metadata-okta", "okta", "prod", "SAML Identity Provider 'okta' missing metadata descriptor URL")
	repo.track(eu, okta)

	ctx := context.Background()
	if err := svc.NotifyAlert(ctx, eu); err != nil {
		t.Fatalf("NotifyAlert: %v", err)
	}
	if err := svc.NotifyAlert(ctx, okta); err != nil {
		t.Fatalf("NotifyAlert: %v", err)
	}

	if issues := created(&rec); len(issues) != 2 {
		t.Fatalf("created %d issues for okta-eu and okta, want one each: %v", len(issues), issues)
	}
}

func TestNotifyAlert_KeepsOneAliasInTwoRealmsApart(t *testing.T) {
	var rec recorder
	repo := newMockRepository()
	svc := gitLabService(t, repo, &rec)

	prod := idpAlert("idp-metadata-prod", "okta", "prod", "SAML Identity Provider 'okta' missing metadata descriptor URL")
	staging := idpAlert("idp-metadata-staging", "okta", "staging", "SAML Identity Provider 'okta' missing metadata descriptor URL")
	repo.track(prod, staging)

	ctx := context.Background()
	if err := svc.NotifyAlert(ctx, prod); err != nil {
		t.Fatalf("NotifyAlert: %v", err)
	}
	if err := svc.NotifyAlert(ctx, staging); err != nil {
		t.Fatalf("NotifyAlert: %v", err)
	}

	if issues := created(&rec); len(issues) != 2 {
		t.Fatalf("created %d issues for okta in prod and staging, want one each: %v", len(issues), issues)
	}
}

func TestNotifyAlert_GivesEveryOtherResourceItsOwnIssue(t *testing.T) {
	var rec recorder
	repo := newMockRepository()
	svc := gitLabService(t, repo, &rec)

	first := idpAlert("realm-1", "prod", "prod", "Realm 'prod' has brute force detection disabled")
	first.ResourceType = "realm"
	second := idpAlert("realm-2", "prod", "prod", "Realm 'prod' allows unverified email")
	second.ResourceType = "realm"
	repo.track(first, second)

	ctx := context.Background()
	if err := svc.NotifyAlert(ctx, first); err != nil {
		t.Fatalf("NotifyAlert: %v", err)
	}
	if err := svc.NotifyAlert(ctx, second); err != nil {
		t.Fatalf("NotifyAlert: %v", err)
	}

	if issues := created(&rec); len(issues) != 2 {
		t.Fatalf("created %d issues for two realm alerts, want one each: %v", len(issues), issues)
	}
}

// A certificate that expires raises an alert that supersedes the warning that
// it was about to, and the configuration check resolves that warning without a
// notice of its own. The expiry takes over its issue rather than leaving it
// open with nobody left to close it.
func TestNotifyAlert_TakesOverTheIssueOfTheAlertItSupersedes(t *testing.T) {
	var rec recorder
	repo := newMockRepository()
	svc := gitLabService(t, repo, &rec)

	expiring := idpAlert("idp-cert-1", "okta", "prod", "SAML Identity Provider 'okta' certificate expiring soon")
	repo.track(expiring)

	ctx := context.Background()
	if err := svc.NotifyAlert(ctx, expiring); err != nil {
		t.Fatalf("NotifyAlert: %v", err)
	}
	expiring.Status = domain.AlertStatusResolved

	expired := idpAlert("idp-cert-expired-1", "okta", "prod", "SAML Identity Provider 'okta' certificate has expired")
	expired.Supersedes = []string{expiring.AlertID}
	repo.track(expired)
	if err := svc.NotifyAlert(ctx, expired); err != nil {
		t.Fatalf("NotifyAlert: %v", err)
	}

	if issues := created(&rec); len(issues) != 1 {
		t.Fatalf("created %d issues for one certificate, want 1: %v", len(issues), issues)
	}
	warning, expiry := repo.loggedEntries[0], repo.loggedEntries[1]
	if expiry.ExternalID != warning.ExternalID {
		t.Errorf("the expiry holds issue %q, want the warning's %q", expiry.ExternalID, warning.ExternalID)
	}
	share := lastNote(&rec)
	if !strings.HasPrefix(share, "Replaces: "+expiring.Title) || !strings.Contains(share, "Now open: "+expired.Title) || strings.Contains(share, "Also open") {
		t.Errorf("takeover comment = %q, want it to say the expiry replaces the warning", share)
	}

	if err := svc.NotifyAlertResolution(ctx, resolveNow(expired)); err != nil {
		t.Fatalf("NotifyAlertResolution: %v", err)
	}
	if !closes(&rec) {
		t.Error("the expiry resolved and its issue stayed open")
	}
}

// closes reports whether the recorded calls closed an issue.
func closes(rec *recorder) bool {
	for _, r := range rec.all() {
		if r.Method == http.MethodPut && r.Body["state_event"] == "close" {
			return true
		}
	}
	return false
}

func resolveNow(alert *domain.Alert) *domain.Alert {
	resolved := *alert
	resolved.Status = domain.AlertStatusResolved
	alert.Status = domain.AlertStatusResolved
	now := time.Now()
	resolved.ResolvedAt = &now
	return &resolved
}

func TestNotifyAlertResolution_KeepsTheIssueOpenForTheProvidersOtherAlerts(t *testing.T) {
	var rec recorder
	repo := newMockRepository()
	svc := gitLabService(t, repo, &rec)

	missing := idpAlert("idp-metadata-1", "okta", "prod", "SAML Identity Provider 'okta' missing metadata descriptor URL")
	expiring := idpAlert("idp-cert-1", "okta", "prod", "SAML Identity Provider 'okta' certificate expiring soon")
	repo.track(missing, expiring)

	ctx := context.Background()
	if err := svc.NotifyAlert(ctx, missing); err != nil {
		t.Fatalf("NotifyAlert: %v", err)
	}
	if err := svc.NotifyAlert(ctx, expiring); err != nil {
		t.Fatalf("NotifyAlert: %v", err)
	}

	if err := svc.NotifyAlertResolution(ctx, resolveNow(missing)); err != nil {
		t.Fatalf("NotifyAlertResolution: %v", err)
	}
	if closes(&rec) {
		t.Fatal("closed the issue while the certificate alert still needs it")
	}

	var resolution string
	for _, r := range rec.all() {
		if strings.HasSuffix(r.Path, "/notes") {
			if body, _ := r.Body["body"].(string); strings.HasPrefix(body, "Resolved:") {
				resolution = body
			}
		}
	}
	if !strings.Contains(resolution, "missing metadata descriptor URL") {
		t.Errorf("resolution comment = %q, want it to name the alert that resolved", resolution)
	}
	if !strings.Contains(resolution, "1 alert still unresolved") {
		t.Errorf("resolution comment = %q, want it to say why the issue stays open", resolution)
	}
}

func TestNotifyAlertResolution_ClosesTheIssueWithTheLastOfItsAlerts(t *testing.T) {
	var rec recorder
	repo := newMockRepository()
	svc := gitLabService(t, repo, &rec)

	missing := idpAlert("idp-metadata-1", "okta", "prod", "SAML Identity Provider 'okta' missing metadata descriptor URL")
	expiring := idpAlert("idp-cert-1", "okta", "prod", "SAML Identity Provider 'okta' certificate expiring soon")
	repo.track(missing, expiring)

	ctx := context.Background()
	if err := svc.NotifyAlert(ctx, missing); err != nil {
		t.Fatalf("NotifyAlert: %v", err)
	}
	if err := svc.NotifyAlert(ctx, expiring); err != nil {
		t.Fatalf("NotifyAlert: %v", err)
	}
	if err := svc.NotifyAlertResolution(ctx, resolveNow(missing)); err != nil {
		t.Fatalf("NotifyAlertResolution: %v", err)
	}

	if err := svc.NotifyAlertResolution(ctx, resolveNow(expiring)); err != nil {
		t.Fatalf("NotifyAlertResolution: %v", err)
	}
	if !closes(&rec) {
		t.Fatal("the last alert of the issue resolved and the issue stayed open")
	}

	all := rec.all()
	last := all[len(all)-1]
	if last.Method != http.MethodPut || last.Body["state_event"] != "close" {
		t.Errorf("last call = %s %s %v, want the close", last.Method, last.Path, last.Body)
	}
}

// lastNote returns the body of the most recent comment.
func lastNote(rec *recorder) string {
	var body string
	for _, r := range rec.all() {
		if strings.HasSuffix(r.Path, "/notes") {
			body, _ = r.Body["body"].(string)
		}
	}
	return body
}

// An alert that joins an issue closed while its resource has an open alert,
// such as one an earlier resolution closed too early or a person closed in
// GitLab, reopens it.
func TestNotifyAlert_ReopensAClosedIssueItJoins(t *testing.T) {
	var rec recorder
	repo := newMockRepository()
	svc := gitLabService(t, repo, &rec)

	missing := idpAlert("idp-metadata-1", "okta", "prod", "SAML Identity Provider 'okta' missing metadata descriptor URL")
	fetch := idpAlert("idp-fetch-1", "okta", "prod", "SAML Identity Provider 'okta' metadata fetch failed")
	repo.track(missing, fetch)

	ctx := context.Background()
	if err := svc.NotifyAlert(ctx, missing); err != nil {
		t.Fatalf("NotifyAlert: %v", err)
	}
	rec.mu.Lock()
	rec.closed = map[int]bool{1: true}
	rec.mu.Unlock()

	if err := svc.NotifyAlert(ctx, fetch); err != nil {
		t.Fatalf("NotifyAlert: %v", err)
	}
	reopened := false
	for _, r := range rec.all() {
		if r.Method == http.MethodPut && r.Body["state_event"] == "reopen" && strings.HasSuffix(r.Path, "/issues/1") {
			reopened = true
		}
	}
	if !reopened {
		t.Fatal("the issue stayed closed after an alert joined it")
	}
	if note := lastNote(&rec); !strings.Contains(note, "reopened this issue") {
		t.Errorf("join comment = %q, want it to say the issue was reopened", note)
	}

	before := len(rec.all())
	open := idpAlert("idp-cert-1", "okta", "prod", "SAML Identity Provider 'okta' certificate expiring soon")
	repo.track(open)
	if err := svc.NotifyAlert(ctx, open); err != nil {
		t.Fatalf("NotifyAlert: %v", err)
	}
	for _, r := range rec.all()[before:] {
		if r.Body["state_event"] == "reopen" {
			t.Fatal("reopened an issue that was already open")
		}
	}
}

// Ignoring an alert lets its provider's issue close; the closing comment says
// an ignored alert remains.
func TestNotifyAlertResolution_ClosingNamesAnIgnoredAlert(t *testing.T) {
	var rec recorder
	repo := newMockRepository()
	svc := gitLabService(t, repo, &rec)

	missing := idpAlert("idp-metadata-1", "okta", "prod", "SAML Identity Provider 'okta' missing metadata descriptor URL")
	expiring := idpAlert("idp-cert-1", "okta", "prod", "SAML Identity Provider 'okta' certificate expiring soon")
	repo.track(missing, expiring)

	ctx := context.Background()
	for _, a := range []*domain.Alert{missing, expiring} {
		if err := svc.NotifyAlert(ctx, a); err != nil {
			t.Fatalf("NotifyAlert: %v", err)
		}
	}
	missing.Status = domain.AlertStatusIgnored

	if err := svc.NotifyAlertResolution(ctx, resolveNow(expiring)); err != nil {
		t.Fatalf("NotifyAlertResolution: %v", err)
	}
	if !closes(&rec) {
		t.Fatal("the issue stayed open for an ignored alert")
	}
	if note := lastNote(&rec); !strings.Contains(note, "Ignored, not resolved, on this resource: 1 alert") {
		t.Errorf("closing comment = %q, want it to say an ignored alert remains", note)
	}
}

// An expiry that joins its provider's issue, still held by another open alert,
// says it replaces the warning that held the same issue.
func TestNotifyAlert_SaysWhatItReplacesOnASharedIssue(t *testing.T) {
	var rec recorder
	repo := newMockRepository()
	svc := gitLabService(t, repo, &rec)

	missing := idpAlert("idp-metadata-1", "okta", "prod", "SAML Identity Provider 'okta' missing metadata descriptor URL")
	expiring := idpAlert("idp-cert-1", "okta", "prod", "SAML Identity Provider 'okta' certificate expiring soon")
	repo.track(missing, expiring)
	ctx := context.Background()
	for _, a := range []*domain.Alert{missing, expiring} {
		if err := svc.NotifyAlert(ctx, a); err != nil {
			t.Fatalf("NotifyAlert: %v", err)
		}
	}
	expiring.Status = domain.AlertStatusResolved

	expired := idpAlert("idp-cert-expired-1", "okta", "prod", "SAML Identity Provider 'okta' certificate has expired")
	expired.Supersedes = []string{expiring.AlertID}
	repo.track(expired)
	if err := svc.NotifyAlert(ctx, expired); err != nil {
		t.Fatalf("NotifyAlert: %v", err)
	}
	if note := lastNote(&rec); !strings.HasPrefix(note, "Replaces: "+expiring.Title) {
		t.Errorf("join comment = %q, want it to say the expiry replaces the warning", note)
	}
}
