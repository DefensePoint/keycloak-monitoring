// Package configcheck provides configuration checking and validation services for Keycloak security.
package configcheck

import (
	"time"
)

// SAMLCertificate represents a SAML certificate.
type SAMLCertificate struct {
	Alias       string
	Certificate string
	NotBefore   time.Time
	NotAfter    time.Time
}
