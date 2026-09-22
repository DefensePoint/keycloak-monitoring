package postgres

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/DefensePoint/keycloak-monitoring/amfa"
)

// classifyDBError translates a low-level DB/driver/network error into
// amfa.ErrAmfaUnavailable when the failure looks like a connectivity or
// availability problem (so HTTP handlers can return 503), and otherwise
// returns the original error wrapped with the supplied operation context.
//
// The classification covers:
//   - context cancellation / deadline exceeded (request aborted, server
//     overload, slow AMFA)
//   - driver.ErrBadConn (broken pooled connection)
//   - pgconn.PgError with SQLSTATE class "08" (connection exception)
//   - net.OpError (TCP refused, no route, EOF mid-handshake)
//   - heuristic substring match on a small set of well-known dial errors
//     for cases that don't surface as a typed error (e.g. lib/pq wrappers)
//
// Anything not matching is treated as a query/logic error and wrapped with
// the op label so the original message is preserved for ops to grep.
func classifyDBError(err error, op string) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, context.Canceled) ||
		errors.Is(err, driver.ErrBadConn) {
		return fmt.Errorf("%s: %w: %v", op, amfa.ErrAmfaUnavailable, err)
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		// SQLSTATE class "08" — Connection Exception.
		// https://www.postgresql.org/docs/current/errcodes-appendix.html
		if strings.HasPrefix(pgErr.Code, "08") {
			return fmt.Errorf("%s: %w: %v", op, amfa.ErrAmfaUnavailable, err)
		}
	}

	var netErr *net.OpError
	if errors.As(err, &netErr) {
		return fmt.Errorf("%s: %w: %v", op, amfa.ErrAmfaUnavailable, err)
	}

	// Heuristic fallback for errors that come back as wrapped plain text
	// (driver libraries that don't always produce typed errors). This is
	// best-effort only; the typed checks above catch the common cases.
	msg := err.Error()
	switch {
	case strings.Contains(msg, "connection refused"),
		strings.Contains(msg, "no such host"),
		strings.Contains(msg, "i/o timeout"),
		strings.Contains(msg, "broken pipe"),
		strings.Contains(msg, "connection reset"):
		return fmt.Errorf("%s: %w: %v", op, amfa.ErrAmfaUnavailable, err)
	}

	return fmt.Errorf("%s: %w", op, err)
}
