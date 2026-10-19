package chi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Test connection sends a request to a server the caller names. Its errors can
// describe that server's reply, such as the body of a failed token request, so
// echoing them would hand the caller whatever the server returned. They belong
// in the log only.

// upstreamReply stands in for content a target server returned.
const upstreamReply = "UPSTREAM-REPLY-must-not-reach-the-caller"

type clientCreationFailure struct{}

func (clientCreationFailure) Error() string {
	return "authentication failed with status 400: " + upstreamReply
}
func (clientCreationFailure) IsClientCreationError() bool { return true }

type failingConnectionTester struct{ err error }

func (t failingConnectionTester) TestConnection(context.Context, string, string, string, string) error {
	return t.err
}
func (t failingConnectionTester) ValidateServerURL(context.Context, string) error { return nil }

func TestTestConnection_DoesNotReturnTheTargetsReply(t *testing.T) {
	h := NewTenantHandlers(nil, nil, failingConnectionTester{err: clientCreationFailure{}}, newTestLogger(), nil, nil)

	body := `{"server_url":"https://keycloak.example.com","client_id":"monitoring-service","client_secret":"s"}`
	req := httptest.NewRequest(http.MethodPost, "/api/tenants/test-connection", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.testConnection(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if strings.Contains(rec.Body.String(), upstreamReply) {
		t.Errorf("response carries the target server's reply: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Failed to create Keycloak client") {
		t.Errorf("response = %s, want the generic failure message", rec.Body.String())
	}
}
