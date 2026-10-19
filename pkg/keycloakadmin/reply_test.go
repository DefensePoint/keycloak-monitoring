package keycloakadmin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Error messages reach API callers (Test Connection, a tenant's health and last
// error), and the server they describe is chosen by whoever creates the
// tenant. They quote Keycloak's own error fields, which explain a failure, and
// never the rest of a reply.

const replyMarker = "REPLY-MARKER-must-stay-out-of-errors"

func TestReplyDetail(t *testing.T) {
	c := &Client{logger: noopLogger{}}
	for _, tc := range []struct {
		name string
		body string
		want string
	}{
		{"token error", `{"error":"invalid_client","error_description":"Invalid client or Invalid client credentials"}`,
			", error: invalid_client - Invalid client or Invalid client credentials"},
		{"admin API error", `{"error":"HTTP 403 Forbidden"}`, ", error: HTTP 403 Forbidden"},
		{"admin API errorMessage", `{"errorMessage":"Realm does not exist"}`, ", error: Realm does not exist"},
		{"HTML page", `<html><body>` + replyMarker + `</body></html>`, ""},
		{"plain text", replyMarker, ""},
		{"other JSON", `{"secret":"` + replyMarker + `","items":[1,2]}`, ""},
		{"empty", ``, ""},
		{"control characters", `{"error":"bad\nline\ttab"}`, ", error: bad line tab"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := c.replyDetail("op", 400, []byte(tc.body)); got != tc.want {
				t.Errorf("replyDetail() = %q, want %q", got, tc.want)
			}
		})
	}

	long := strings.Repeat("x", 500)
	got := c.replyDetail("op", 400, []byte(`{"error":"`+long+`"}`))
	if n := len([]rune(got)); n > len(", error: ")+replyFieldMax+len("...") {
		t.Errorf("a long field was not cut: %d runes", n)
	}
}

// replyServer answers the token request with tokenStatus and tokenBody, and
// every Admin API request with adminStatus and adminBody.
func replyServer(t *testing.T, tokenStatus int, tokenBody string, adminStatus int, adminBody string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/protocol/openid-connect/token") {
			if tokenStatus == http.StatusOK {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "t", "expires_in": 300})
				return
			}
			w.WriteHeader(tokenStatus)
			_, _ = w.Write([]byte(tokenBody))
			return
		}
		w.WriteHeader(adminStatus)
		_, _ = w.Write([]byte(adminBody))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func newReplyClient(srv *httptest.Server) (*Client, error) {
	return NewClient(context.Background(), &ClientConfig{
		ServerURL: srv.URL, AdminRealm: "master", ClientID: "monitoring-service", ClientSecret: "s",
		HTTPClient: srv.Client(), MaxRetries: 1,
	}, noopLogger{})
}

func TestTokenRequestError_QuotesKeycloakFieldsOnly(t *testing.T) {
	_, err := newReplyClient(replyServer(t, http.StatusBadRequest, `<html>`+replyMarker+`</html>`, 0, ""))
	if err == nil || strings.Contains(err.Error(), replyMarker) {
		t.Errorf("token error = %v, want an error without the reply", err)
	}
	if err != nil && !strings.Contains(err.Error(), "status 400") {
		t.Errorf("token error = %v, want the status", err)
	}

	_, err = newReplyClient(replyServer(t, http.StatusUnauthorized,
		`{"error":"invalid_client","error_description":"Invalid client or Invalid client credentials"}`, 0, ""))
	if err == nil || !strings.Contains(err.Error(), "invalid_client - Invalid client or Invalid client credentials") {
		t.Errorf("token error = %v, want Keycloak's error fields", err)
	}
}

func TestAdminAPIError_QuotesKeycloakFieldsOnly(t *testing.T) {
	c, err := newReplyClient(replyServer(t, http.StatusOK, "", http.StatusForbidden, `<html>`+replyMarker+`</html>`))
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	_, err = c.GetServerInfo(context.Background())
	if err == nil || strings.Contains(err.Error(), replyMarker) || !strings.Contains(err.Error(), "status: 403") {
		t.Errorf("server info error = %v, want the status without the reply", err)
	}

	c, err = newReplyClient(replyServer(t, http.StatusOK, "", http.StatusForbidden, `{"error":"HTTP 403 Forbidden"}`))
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	_, err = c.GetServerInfo(context.Background())
	if err == nil || !strings.Contains(err.Error(), "error: HTTP 403 Forbidden") {
		t.Errorf("server info error = %v, want Keycloak's error field", err)
	}
}
