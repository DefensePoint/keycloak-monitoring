package postgres

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"net"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/DefensePoint/keycloak-monitoring/amfa"
)

func TestClassifyDBError_Nil(t *testing.T) {
	if err := classifyDBError(nil, "any op"); err != nil {
		t.Errorf("expected nil for nil input, got %v", err)
	}
}

func TestClassifyDBError_ContextDeadline(t *testing.T) {
	err := classifyDBError(context.DeadlineExceeded, "op")
	if !errors.Is(err, amfa.ErrAmfaUnavailable) {
		t.Errorf("expected ErrAmfaUnavailable, got %v", err)
	}
}

func TestClassifyDBError_ContextCanceled(t *testing.T) {
	err := classifyDBError(context.Canceled, "op")
	if !errors.Is(err, amfa.ErrAmfaUnavailable) {
		t.Errorf("expected ErrAmfaUnavailable, got %v", err)
	}
}

func TestClassifyDBError_BadConn(t *testing.T) {
	err := classifyDBError(driver.ErrBadConn, "op")
	if !errors.Is(err, amfa.ErrAmfaUnavailable) {
		t.Errorf("expected ErrAmfaUnavailable, got %v", err)
	}
}

func TestClassifyDBError_PgConnectionException(t *testing.T) {
	// SQLSTATE class "08" = Connection Exception
	pgErr := &pgconn.PgError{Code: "08006", Message: "connection failure"}
	err := classifyDBError(pgErr, "op")
	if !errors.Is(err, amfa.ErrAmfaUnavailable) {
		t.Errorf("expected ErrAmfaUnavailable for SQLSTATE 08006, got %v", err)
	}
}

func TestClassifyDBError_PgOtherError_NotUnavailable(t *testing.T) {
	// SQLSTATE class "42" = Syntax error — should NOT be classified as unavailable
	pgErr := &pgconn.PgError{Code: "42P01", Message: "relation does not exist"}
	err := classifyDBError(pgErr, "list")
	if errors.Is(err, amfa.ErrAmfaUnavailable) {
		t.Errorf("syntax errors should NOT be ErrAmfaUnavailable, got %v", err)
	}
	if err == nil || err.Error() == "" {
		t.Errorf("expected wrapped error, got %v", err)
	}
}

func TestClassifyDBError_NetOpError(t *testing.T) {
	netErr := &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}
	err := classifyDBError(netErr, "op")
	if !errors.Is(err, amfa.ErrAmfaUnavailable) {
		t.Errorf("expected ErrAmfaUnavailable for net.OpError, got %v", err)
	}
}

func TestClassifyDBError_HeuristicConnectionRefused(t *testing.T) {
	// Plain error with the substring — should still classify as unavailable
	err := classifyDBError(errors.New("dial tcp 10.0.0.1:5432: connection refused"), "op")
	if !errors.Is(err, amfa.ErrAmfaUnavailable) {
		t.Errorf("expected ErrAmfaUnavailable for substring match, got %v", err)
	}
}

func TestClassifyDBError_RandomError_NotUnavailable(t *testing.T) {
	err := classifyDBError(fmt.Errorf("some unrelated bug"), "list")
	if errors.Is(err, amfa.ErrAmfaUnavailable) {
		t.Errorf("random error should NOT be ErrAmfaUnavailable, got %v", err)
	}
}
