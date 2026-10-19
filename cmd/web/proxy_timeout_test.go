package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// startProxy serves the proxy the way main does: behind an http.Server whose
// write deadline is the one main sets for this upstream timeout.
func startProxy(t *testing.T, backendURL string, timeout time.Duration) *httptest.Server {
	t.Helper()
	srv := httptest.NewUnstartedServer(newProxyHandler(backendURL, timeout))
	srv.Config.WriteTimeout = writeTimeoutFor(timeout)
	srv.Start()
	t.Cleanup(srv.Close)
	return srv
}

// An API call slower than the proxy's limit used to end with no response at
// all: the 502 was written after the server's write deadline, so the
// connection closed empty and the browser silently retried the GET, running
// the slow request again on the API every time.
func TestProxyAnswersATimedOutUpstreamWith504(t *testing.T) {
	var hits atomic.Int32
	release := make(chan struct{})
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(func() { close(release); api.Close() })
	proxy := startProxy(t, api.URL, 200*time.Millisecond)

	resp, err := http.Get(proxy.URL + "/api/slow")
	if err != nil {
		t.Fatalf("no response through the proxy: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusGatewayTimeout {
		t.Errorf("status = %d, want 504", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var body struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil || body.Error == "" {
		t.Errorf("body is not a JSON error: %v %+v", err, body)
	}
	if n := hits.Load(); n != 1 {
		t.Errorf("API received the request %d times, want 1", n)
	}
}

// A browser that gives up (TanStack Query cancels a refetch it replaces)
// should not leave the API working on an answer nobody will read.
func TestProxyCancelsTheUpstreamWhenTheBrowserLeaves(t *testing.T) {
	cancelled := make(chan struct{})
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
		close(cancelled)
	}))
	t.Cleanup(api.Close)
	proxy := startProxy(t, api.URL, 5*time.Second)

	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, proxy.URL+"/api/slow", nil)
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()
	if resp, err := http.DefaultClient.Do(req); err == nil {
		_ = resp.Body.Close()
	}

	select {
	case <-cancelled:
	case <-time.After(2 * time.Second):
		t.Fatal("the API request was not cancelled when the browser left")
	}
}

func TestProxyStillAnswers502WhenTheAPIIsUnreachable(t *testing.T) {
	api := httptest.NewServer(http.NotFoundHandler())
	api.Close()
	proxy := startProxy(t, api.URL, time.Second)

	resp, err := http.Get(proxy.URL + "/api/x")
	if err != nil {
		t.Fatalf("no response through the proxy: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", resp.StatusCode)
	}
}

func TestProxyPassesAFastAnswerThroughUnchanged(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	t.Cleanup(api.Close)
	proxy := startProxy(t, api.URL, time.Second)

	resp, err := http.Get(proxy.URL + "/api/fast")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated || string(body) != `{"ok":true}` {
		t.Errorf("got %d %q, want 201 {\"ok\":true}", resp.StatusCode, body)
	}
}
