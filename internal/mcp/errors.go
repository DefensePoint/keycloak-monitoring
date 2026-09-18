package mcp

import (
	"errors"
	"fmt"
)

// Sentinel errors whose messages are safe to return to MCP clients. Anything
// not classified below is reported as errInternal; the full error only ever
// reaches the server log.
var (
	ErrUnauthenticated       = errors.New("authentication required")
	ErrForbidden             = errors.New("access denied")
	ErrTenantNotAvailable    = errors.New("tenant not found or disabled")
	ErrInvalidInput          = errors.New("invalid input")
	ErrAlertNotFound         = errors.New("alert not found")
	ErrEventNotFound         = errors.New("event not found")
	ErrAmfaNotAvailable      = errors.New("AMFA not available for this tenant")
	ErrAmfaSourceUnavailable = errors.New("AMFA data source temporarily unavailable")
	ErrRateLimited           = errors.New("rate limited, retry later")
	ErrResultTooLarge        = errors.New("result too large, retry with a smaller limit")

	errInternal = errors.New("internal error")
)

// invalidInputError carries a client-safe validation message. Only messages
// constructed through Invalidf are ever forwarded verbatim to a client, so
// callers must not interpolate internal error values into them.
type invalidInputError struct {
	msg string
}

func (e *invalidInputError) Error() string { return e.msg }

func (e *invalidInputError) Is(target error) bool { return target == ErrInvalidInput }

// Invalidf builds a validation error whose message is returned to the client.
func Invalidf(format string, args ...any) error {
	return &invalidInputError{msg: "invalid input: " + fmt.Sprintf(format, args...)}
}

// toClientError maps any error to one safe for a client: Invalidf messages
// pass through, known sentinels map to their fixed message, everything else
// collapses to a generic internal error.
func toClientError(err error) error {
	var invalid *invalidInputError
	if errors.As(err, &invalid) {
		return invalid
	}
	for _, sentinel := range []error{
		ErrUnauthenticated, ErrForbidden, ErrTenantNotAvailable, ErrInvalidInput,
		ErrAlertNotFound, ErrEventNotFound, ErrAmfaNotAvailable, ErrAmfaSourceUnavailable,
		ErrRateLimited, ErrResultTooLarge,
	} {
		if errors.Is(err, sentinel) {
			return sentinel
		}
	}
	return errInternal
}
