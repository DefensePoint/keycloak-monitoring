package healthcheck

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func serveReady(t *testing.T, r *Readiness) (int, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ready", nil))
	return rec.Code, rec.Body.String()
}

func TestReadiness_NoCheckIsReady(t *testing.T) {
	code, body := serveReady(t, NewReadiness(nil))
	if code != http.StatusOK || !strings.Contains(body, `"ready"`) {
		t.Fatalf("got %d %s, want 200 ready", code, body)
	}
}

func TestReadiness_PassingCheckIsReady(t *testing.T) {
	r := NewReadiness(nil)
	r.SetCheck(func(context.Context) error { return nil })
	if code, body := serveReady(t, r); code != http.StatusOK || !strings.Contains(body, `"ready"`) {
		t.Fatalf("got %d %s, want 200 ready", code, body)
	}
}

func TestReadiness_FailingCheckIsNotReady(t *testing.T) {
	r := NewReadiness(nil)
	r.SetCheck(func(context.Context) error { return errors.New("dial tcp 10.0.0.5:5432: connection refused") })
	code, body := serveReady(t, r)
	if code != http.StatusServiceUnavailable || !strings.Contains(body, `"not ready"`) {
		t.Fatalf("got %d %s, want 503 not ready", code, body)
	}
	// /ready is unauthenticated: the reason goes to the log, not the reply.
	if strings.Contains(body, "10.0.0.5") || strings.Contains(body, "refused") {
		t.Fatalf("reply leaks the check's error: %s", body)
	}
}

func TestReadiness_SlowCheckTimesOut(t *testing.T) {
	r := NewReadiness(nil)
	r.timeout = 50 * time.Millisecond
	r.SetCheck(func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() })
	start := time.Now()
	code, _ := serveReady(t, r)
	if code != http.StatusServiceUnavailable {
		t.Fatalf("got %d, want 503 for a check that does not finish", code)
	}
	if took := time.Since(start); took > time.Second {
		t.Fatalf("answered after %v, want about the timeout", took)
	}
}

func TestReadiness_RecoversWhenTheCheckPassesAgain(t *testing.T) {
	r := NewReadiness(nil)
	fail := true
	r.SetCheck(func(context.Context) error {
		if fail {
			return errors.New("down")
		}
		return nil
	})
	if code, _ := serveReady(t, r); code != http.StatusServiceUnavailable {
		t.Fatalf("while failing: got %d, want 503", code)
	}
	fail = false
	if code, _ := serveReady(t, r); code != http.StatusOK {
		t.Fatalf("after recovery: got %d, want 200", code)
	}
}
