package keycloakadmin

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// ErrInvalidPathSegment is returned for a realm name or ID that cannot be used
// as one segment of an Admin API path.
var ErrInvalidPathSegment = errors.New("invalid path segment")

// CheckPathSegment refuses a value that is empty, a dot segment, or holds a path
// separator. Keycloak resolves those against the segments around them, so a
// user ID of ../../master/users/<id> read a user of another realm.
func CheckPathSegment(value string) error {
	if value == "" || value == "." || value == ".." || strings.ContainsAny(value, `/\`) {
		return fmt.Errorf("%w: %q", ErrInvalidPathSegment, value)
	}
	return nil
}

func pathSegment(value string) (string, error) {
	if err := CheckPathSegment(value); err != nil {
		return "", err
	}
	return url.PathEscape(value), nil
}
