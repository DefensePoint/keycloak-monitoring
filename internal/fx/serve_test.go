package fx

import (
	"context"
	"net/http"
	"testing"
	"time"

	"go.uber.org/fx"

	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
)

type fakeShutdowner struct {
	calls int
}

func (f *fakeShutdowner) Shutdown(...fx.ShutdownOption) error {
	f.calls++
	return nil
}

func TestServeOrShutdownFatalError(t *testing.T) {
	sd := &fakeShutdowner{}
	srv := &http.Server{Addr: "127.0.0.1:-1"}

	serveOrShutdown("test", srv, logger.NewDefault(), sd)

	if sd.calls != 1 {
		t.Fatalf("expected one shutdown request on a fatal listener error, got %d", sd.calls)
	}
}

// The exit code is the entire point of the fix: restart policies only fire on
// a nonzero exit, so assert it end to end through a real fx app rather than
// trusting a fake that would stay green if the ExitCode option were dropped.
func TestServeOrShutdownExitCode(t *testing.T) {
	var sd fx.Shutdowner
	app := fx.New(fx.NopLogger, fx.Populate(&sd))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := app.Start(ctx); err != nil {
		t.Fatalf("starting fx app: %v", err)
	}
	defer func() { _ = app.Stop(context.Background()) }()

	srv := &http.Server{Addr: "127.0.0.1:-1"}
	serveOrShutdown("test", srv, logger.NewDefault(), sd)

	select {
	case sig := <-app.Wait():
		if sig.ExitCode != 1 {
			t.Fatalf("expected exit code 1 in the shutdown signal, got %d", sig.ExitCode)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no shutdown signal received after a fatal listener error")
	}
}

func TestServeOrShutdownServerClosed(t *testing.T) {
	sd := &fakeShutdowner{}
	srv := &http.Server{Addr: "127.0.0.1:0"}
	if err := srv.Close(); err != nil {
		t.Fatalf("closing server: %v", err)
	}

	serveOrShutdown("test", srv, logger.NewDefault(), sd)

	if sd.calls != 0 {
		t.Fatalf("graceful close must not request shutdown, got %d calls", sd.calls)
	}
}
