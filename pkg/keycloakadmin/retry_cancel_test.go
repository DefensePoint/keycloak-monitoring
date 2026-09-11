package keycloakadmin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// A cancelled context must cut short the retry backoff.
//
// The backoff used to be a plain time.Sleep, which no amount of cancellation
// can interrupt. With the default 5s backoff growing per attempt, that parked
// a Keycloak monitor's events poller well past the 10s drain in Monitor.Stop,
// so shutdown logged a stop timeout and abandoned the worker.
func TestDoRequest_CancelledContextAbortsRetryBackoff(t *testing.T) {
	// Always 500, so every attempt retries and hits the backoff.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/realms/master/protocol/openid-connect/token" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"t","expires_in":300}`))
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := &Client{
		baseURL:    srv.URL,
		httpClient: srv.Client(),
		config: &ClientConfig{
			MaxRetries: 5,
			// Far longer than the test is willing to wait: if the backoff
			// ignores cancellation this cannot finish in time.
			RetryBackoff: 30 * time.Second,
		},
		logger:      noopLogger{},
		accessToken: "test-token",
		tokenExpiry: time.Now().Add(time.Hour),
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	_, err := c.doRequest(ctx, http.MethodGet, "/admin/realms/master/events", nil)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("doRequest() returned no error after its context was cancelled")
	}
	if elapsed > 5*time.Second {
		t.Errorf("doRequest() took %s after cancellation, want a prompt return: "+
			"the retry backoff is not honouring ctx", elapsed)
	}
}
