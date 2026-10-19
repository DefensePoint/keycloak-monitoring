package notifications_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

// An alert that joins a closed issue reopens it, and an alert that replaces
// the one an issue is named after renames it. Both are edits of the issue, and
// they come before the comment: posting "it reopened this issue" before a
// reopen that then failed left a false comment on a closed issue, and the
// hourly retry of the failed send posted it again.

// notes returns the comments posted from index from of the recorded calls on.
func notesFrom(rec *recorder, from int) []string {
	var bodies []string
	for _, r := range rec.all()[from:] {
		if r.Method == http.MethodPost && strings.HasSuffix(r.Path, "/notes") {
			body, _ := r.Body["body"].(string)
			bodies = append(bodies, body)
		}
	}
	return bodies
}

// edits returns the issue edits (PUT) from index from of the recorded calls on.
func editsFrom(rec *recorder, from int) []recordedRequest {
	var puts []recordedRequest
	for _, r := range rec.all()[from:] {
		if r.Method == http.MethodPut {
			puts = append(puts, r)
		}
	}
	return puts
}

func issueState(rec *recorder, iid int) (closed bool, title string) {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	return rec.closed[iid], rec.titles[iid]
}

func lastLogFor(repo *mockRepository, alertID string) *domain.NotificationLog {
	var last *domain.NotificationLog
	for _, log := range repo.loggedEntries {
		if log.AlertID == alertID {
			last = log
		}
	}
	return last
}

// A reopen GitLab refuses posts nothing: the issue is not said to be
// reopened while it stays closed, the send is recorded as failed, and a retry
// while the refusal lasts posts nothing either.
func TestNotifyAlert_ARefusedReopenPostsNothing(t *testing.T) {
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
	rec.refuseUpdates = true
	rec.mu.Unlock()
	before := len(rec.all())

	_ = svc.NotifyAlert(ctx, fetch)
	_ = svc.RetryFailedNotifications(ctx, fetch)

	if notes := notesFrom(&rec, before); len(notes) != 0 {
		t.Errorf("posted %d comment(s) while the reopen was refused, want none: %q", len(notes), notes)
	}
	if closed, _ := issueState(&rec, 1); !closed {
		t.Error("the fake issue is open; the refusal did not apply")
	}
	if log := lastLogFor(repo, fetch.AlertID); log == nil || log.Status != "failed" {
		t.Errorf("last send for the joining alert = %+v, want it recorded as failed", log)
	}
}

// Once GitLab accepts the reopen, the retry reopens the issue first and then
// posts the one comment, which says it reopened the issue.
func TestRetryFailedNotifications_ReopensThenCommentsOnce(t *testing.T) {
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
	rec.refuseUpdates = true
	rec.mu.Unlock()
	_ = svc.NotifyAlert(ctx, fetch)

	rec.mu.Lock()
	rec.refuseUpdates = false
	rec.mu.Unlock()
	before := len(rec.all())
	if err := svc.RetryFailedNotifications(ctx, fetch); err != nil {
		t.Fatalf("RetryFailedNotifications: %v", err)
	}

	calls := rec.all()[before:]
	reopenAt, noteAt := -1, -1
	for i, r := range calls {
		if r.Method == http.MethodPut && r.Body["state_event"] == "reopen" && reopenAt < 0 {
			reopenAt = i
		}
		if r.Method == http.MethodPost && strings.HasSuffix(r.Path, "/notes") && noteAt < 0 {
			noteAt = i
		}
	}
	if reopenAt < 0 || noteAt < 0 || reopenAt > noteAt {
		t.Fatalf("reopen at call %d, comment at call %d; want the reopen first", reopenAt, noteAt)
	}
	notes := notesFrom(&rec, before)
	if len(notes) != 1 || !strings.Contains(notes[0], "reopened this issue") {
		t.Errorf("comments = %q, want one that says the issue was reopened", notes)
	}
	if closed, _ := issueState(&rec, 1); closed {
		t.Error("the issue is still closed after the retry")
	}
	if log := lastLogFor(repo, fetch.AlertID); log == nil || log.Status != "sent" || log.ExternalID != "1" {
		t.Errorf("last send for the joining alert = %+v, want sent on issue 1", log)
	}
}

// An expiry that takes over the issue its warning opened renames the issue:
// it tracks an expired certificate now, not one expiring soon.
func TestNotifyAlert_AReplacementRenamesTheIssueItsWarningNamed(t *testing.T) {
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
	expired.Severity = domain.AlertSeverityCritical
	expired.Supersedes = []string{expiring.AlertID}
	repo.track(expired)
	if err := svc.NotifyAlert(ctx, expired); err != nil {
		t.Fatalf("NotifyAlert: %v", err)
	}

	want := "[CRITICAL] SAML Identity Provider 'okta' certificate has expired - okta (prod)"
	if _, title := issueState(&rec, 1); title != want {
		t.Errorf("issue title = %q, want %q", title, want)
	}
	if note := lastNote(&rec); !strings.HasPrefix(note, "Replaces: "+expiring.Title) {
		t.Errorf("comment = %q, want it to say what the expiry replaces", note)
	}
}

// Taking over a closed issue renames and reopens it in one edit, before the
// comment, so a refused edit changes neither and posts nothing.
func TestNotifyAlert_AReplacementOnAClosedIssueRenamesAndReopensInOneEdit(t *testing.T) {
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
	rec.mu.Lock()
	rec.closed = map[int]bool{1: true}
	rec.mu.Unlock()
	before := len(rec.all())

	expired := idpAlert("idp-cert-expired-1", "okta", "prod", "SAML Identity Provider 'okta' certificate has expired")
	expired.Severity = domain.AlertSeverityCritical
	expired.Supersedes = []string{expiring.AlertID}
	repo.track(expired)
	if err := svc.NotifyAlert(ctx, expired); err != nil {
		t.Fatalf("NotifyAlert: %v", err)
	}

	edits := editsFrom(&rec, before)
	if len(edits) != 1 {
		t.Fatalf("made %d issue edits, want one: %+v", len(edits), edits)
	}
	if edits[0].Body["state_event"] != "reopen" || edits[0].Body["title"] == nil {
		t.Errorf("edit body = %v, want both the reopen and the new title", edits[0].Body)
	}
	if closed, _ := issueState(&rec, 1); closed {
		t.Error("the issue is still closed")
	}
}

// The title changes only when the issue still carries the replaced alert's
// name: an issue another alert opened keeps that alert's name, and a title a
// person changed in GitLab is left as they wrote it.
func TestNotifyAlert_AReplacementKeepsATitleItDidNotGive(t *testing.T) {
	cases := map[string]func(rec *recorder){
		"named after another alert": func(*recorder) {},
		"renamed by a person": func(rec *recorder) {
			rec.mu.Lock()
			rec.titles[1] = "okta SSO certificate rotation (owner: IAM team)"
			rec.mu.Unlock()
		},
		"generated name with a person's prefix": func(rec *recorder) {
			rec.mu.Lock()
			rec.titles[1] = "[IAM] tracking: [WARNING] SAML Identity Provider 'okta' certificate expiring soon - okta (prod)"
			rec.mu.Unlock()
		},
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
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
			setup(&rec)
			_, titleBefore := issueState(&rec, 1)
			expiring.Status = domain.AlertStatusResolved
			before := len(rec.all())

			expired := idpAlert("idp-cert-expired-1", "okta", "prod", "SAML Identity Provider 'okta' certificate has expired")
			expired.Severity = domain.AlertSeverityCritical
			expired.Supersedes = []string{expiring.AlertID}
			repo.track(expired)
			if err := svc.NotifyAlert(ctx, expired); err != nil {
				t.Fatalf("NotifyAlert: %v", err)
			}

			if _, title := issueState(&rec, 1); title != titleBefore {
				t.Errorf("issue title = %q, want it kept as %q", title, titleBefore)
			}
			if edits := editsFrom(&rec, before); len(edits) != 0 {
				t.Errorf("edited the open issue %d time(s), want none: %+v", len(edits), edits)
			}
		})
	}
}
