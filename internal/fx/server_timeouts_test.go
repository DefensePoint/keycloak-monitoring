package fx

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/DefensePoint/keycloak-monitoring/internal/config"
	chihttp "github.com/DefensePoint/keycloak-monitoring/internal/http/chi"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/internal/metrics"
)

// The API server had no timeouts, so a client could hold a connection open
// forever by never finishing its request. These tests check the limits it
// is built with and run it on a real socket.

type pingRoutes struct{}

func (pingRoutes) RegisterRoutes(r chi.Router) {
	r.Get("/api/ping", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "pong")
	})
}

func newAPIServer(t *testing.T) *http.Server {
	t.Helper()
	log := logger.NewNoop()
	pass := func(next http.Handler) http.Handler { return next }
	srv, err := provideHTTPServer(HTTPServerParams{
		Config:  &config.AppConfig{},
		Router:  chihttp.NewRouter(log, pass, nil, pingRoutes{}),
		Logger:  log,
		Metrics: metrics.New(),
	})
	if err != nil {
		t.Fatalf("provideHTTPServer: %v", err)
	}
	return srv
}

func serveOnLoopback(t *testing.T, srv *http.Server) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
	return ln.Addr().String()
}

func TestAPIServer_HasTimeouts(t *testing.T) {
	srv := newAPIServer(t)
	for _, tc := range []struct {
		name string
		got  time.Duration
		want time.Duration
	}{
		{"ReadHeaderTimeout", srv.ReadHeaderTimeout, 10 * time.Second},
		{"ReadTimeout", srv.ReadTimeout, 30 * time.Second},
		{"WriteTimeout", srv.WriteTimeout, 2 * time.Minute},
		{"IdleTimeout", srv.IdleTimeout, 120 * time.Second},
	} {
		if tc.got != tc.want {
			t.Errorf("%s = %s, want %s", tc.name, tc.got, tc.want)
		}
	}
}

func TestAPIServer_ClosesAConnectionThatNeverFinishesItsHeaders(t *testing.T) {
	// Waits out the real 10 second limit, so it runs in every job.
	addr := serveOnLoopback(t, newAPIServer(t))

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close() }()
	if _, err := io.WriteString(conn, "GET /api/ping HTTP/1.1\r\nHost: kmt\r\n"); err != nil {
		t.Fatalf("write: %v", err)
	}

	// The headers never end. The server must give up on its own, well before
	// this deadline does.
	start := time.Now()
	_ = conn.SetReadDeadline(time.Now().Add(20 * time.Second))
	_, err = conn.Read(make([]byte, 1))
	took := time.Since(start)
	if ne, ok := err.(net.Error); ok && ne.Timeout() {
		t.Fatalf("the server still held the connection open after %s", took)
	}
	if took < 9*time.Second || took > 15*time.Second {
		t.Errorf("the server closed the connection after %s, want about 10s", took)
	}
}

func TestAPIServer_AnswersANormalRequest(t *testing.T) {
	addr := serveOnLoopback(t, newAPIServer(t))

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close() }()
	if _, err := io.WriteString(conn, "GET /api/ping HTTP/1.1\r\nHost: kmt\r\n\r\n"); err != nil {
		t.Fatalf("write: %v", err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || string(body) != "pong" {
		t.Errorf("got %d %q, want 200 pong", resp.StatusCode, body)
	}
}
