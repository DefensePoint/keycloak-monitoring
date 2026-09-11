package httpclient

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/DefensePoint/keycloak-monitoring/amfa"
)

// classifyHTTPError translates a transport-level failure into
// amfa.ErrAmfaUnavailable so handlers can answer 503, mirroring what
// amfa/postgres does for driver errors. Anything else is wrapped with the
// operation label so the original message survives for ops to grep.
func classifyHTTPError(err error, op string) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return fmt.Errorf("%s: %w: %v", op, amfa.ErrAmfaUnavailable, err)
	}

	var netErr *net.OpError
	if errors.As(err, &netErr) {
		return fmt.Errorf("%s: %w: %v", op, amfa.ErrAmfaUnavailable, err)
	}

	// Errors that arrive as wrapped plain text rather than a typed value.
	// Best-effort; the typed checks above cover the common cases.
	msg := err.Error()
	switch {
	case strings.Contains(msg, "connection refused"),
		strings.Contains(msg, "no such host"),
		strings.Contains(msg, "i/o timeout"),
		strings.Contains(msg, "context deadline exceeded"),
		strings.Contains(msg, "broken pipe"),
		strings.Contains(msg, "connection reset"),
		strings.Contains(msg, "EOF"),
		strings.Contains(msg, "TLS handshake"):
		return fmt.Errorf("%s: %w: %v", op, amfa.ErrAmfaUnavailable, err)
	}

	return fmt.Errorf("%s: %w", op, err)
}

// classifyStatus turns a non-2xx response into an error.
//
// 5xx, 408 and 429 are availability problems: AMFA is reachable but cannot
// answer right now, and a caller that retries later may well succeed, so these
// map to ErrAmfaUnavailable.
//
// 401 and 403 do not. A rejected or wrong-realm token is a configuration fault
// that retrying cannot fix, and reporting it as "unavailable" would hide a
// misconfigured client behind what looks like a transient outage.
func classifyStatus(status int, body []byte, op string) error {
	if status >= 200 && status < 300 {
		return nil
	}

	detail := strings.TrimSpace(string(body))
	if len(detail) > 512 {
		// A raw byte slice can split a multi-byte UTF-8 sequence mid-rune;
		// walk back to the last full rune boundary within the limit instead.
		cut := 512
		for cut > 0 && !utf8.RuneStart(detail[cut]) {
			cut--
		}
		detail = detail[:cut] + "..."
	}

	switch {
	case status >= 500,
		status == http.StatusRequestTimeout,
		status == http.StatusTooManyRequests:
		return fmt.Errorf("%s: %w: HTTP %d: %s", op, amfa.ErrAmfaUnavailable, status, detail)
	case status == http.StatusUnauthorized, status == http.StatusForbidden:
		return fmt.Errorf("%s: authentication rejected by AMFA: HTTP %d: %s", op, status, detail)
	default:
		return fmt.Errorf("%s: HTTP %d: %s", op, status, detail)
	}
}
