package auth

import (
	"crypto/rand"
	"encoding/base64"
	"time"
)

// timeFromUnix converts a Unix timestamp to time.Time.
func timeFromUnix(timestamp int64) time.Time {
	return time.Unix(timestamp, 0)
}

// GenerateRandomState generates a cryptographically secure random state string.
// This is used for CSRF protection in the OAuth2 flow.
func GenerateRandomState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// GenerateSessionID generates a cryptographically secure session identifier.
func GenerateSessionID() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}
