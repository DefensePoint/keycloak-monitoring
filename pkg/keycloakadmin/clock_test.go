package keycloakadmin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type warnLogger struct {
	noopLogger
	mu    sync.Mutex
	warns []string
}

func (l *warnLogger) Warn(msg string, fields ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.warns = append(l.warns, fmt.Sprint(append([]any{msg}, fields...)...))
}

func (l *warnLogger) clockWarnings() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []string
	for _, w := range l.warns {
		if strings.Contains(w, "clock differs") {
			out = append(out, w)
		}
	}
	return out
}

// clockServer answers as a Keycloak whose clock is off by skew.
func clockServer(skew time.Duration) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Date", time.Now().Add(skew).UTC().Format(http.TimeFormat))
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/protocol/openid-connect/token") {
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "t", "expires_in": 300})
			return
		}
		_, _ = w.Write([]byte("[]"))
	}))
}

func clockClient(t *testing.T, srv *httptest.Server, log Logger) *Client {
	t.Helper()
	c, err := NewClient(context.Background(), &ClientConfig{
		ServerURL: srv.URL, AdminRealm: "master", ClientID: "monitoring-service", ClientSecret: "s", HTTPClient: srv.Client(),
	}, log)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	return c
}

func TestCheckClock_WarnsOnceWhenKeycloaksClockIsOff(t *testing.T) {
	srv := clockServer(2 * time.Minute)
	defer srv.Close()
	log := &warnLogger{}
	c := clockClient(t, srv, log)

	for i := 0; i < 3; i++ {
		if _, _, err := c.GetEventsSince(context.Background(), "acme", time.Now(), 0, EventsPoll); err != nil {
			t.Fatalf("GetEventsSince() error = %v", err)
		}
	}
	warns := log.clockWarnings()
	if len(warns) != 1 {
		t.Fatalf("got %d clock warnings, want 1 within %v: %v", len(warns), clockSkewWarnEvery, warns)
	}
	if !strings.Contains(warns[0], "2m0s") || !strings.Contains(warns[0], srv.URL) {
		t.Errorf("warning %q does not give the 2m difference and the Keycloak it is about", warns[0])
	}

	c.now = func() time.Time { return time.Now().Add(clockSkewWarnEvery + time.Second) }
	if _, _, err := c.GetEventsSince(context.Background(), "acme", time.Now(), 0, EventsPoll); err != nil {
		t.Fatalf("GetEventsSince() error = %v", err)
	}
	if got := len(log.clockWarnings()); got != 2 {
		t.Errorf("got %d clock warnings after %v, want the warning repeated", got, clockSkewWarnEvery)
	}
}

func TestCheckClock_QuietWhenKeycloakIsInStep(t *testing.T) {
	for _, skew := range []time.Duration{0, -5 * time.Second, 9 * time.Second} {
		srv := clockServer(skew)
		log := &warnLogger{}
		c := clockClient(t, srv, log)
		if _, _, err := c.GetEventsSince(context.Background(), "acme", time.Now(), 0, EventsPoll); err != nil {
			t.Fatalf("GetEventsSince() error = %v", err)
		}
		srv.Close()
		if w := log.clockWarnings(); len(w) != 0 {
			t.Errorf("skew %v: unexpected clock warning %v", skew, w)
		}
	}
}
