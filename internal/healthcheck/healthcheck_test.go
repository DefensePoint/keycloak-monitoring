package healthcheck

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestProbeHealthy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	if err := Probe(srv.URL, time.Second); err != nil {
		t.Fatalf("expected healthy, got %v", err)
	}
}

func TestProbeUnhealthyStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	if err := Probe(srv.URL, time.Second); err == nil {
		t.Fatal("expected an error for a non-200 response")
	}
}

func TestProbeUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	srv.Close()

	if err := Probe(srv.URL, time.Second); err == nil {
		t.Fatal("expected an error for an unreachable server")
	}
}
