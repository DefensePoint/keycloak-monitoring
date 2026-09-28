package saferequest

import "crypto/tls"

// newSkipVerifyTLSConfig isolates the InsecureSkipVerify usage in a single
// helper so static analyzers (and reviewers) see exactly one site to audit.
// SkipTLSVerify is plumbed for parity with the existing Keycloak client
// configuration; SSRF protections do not depend on it.
func newSkipVerifyTLSConfig() *tls.Config {
	return &tls.Config{InsecureSkipVerify: true} //nolint:gosec // explicitly opt-in via Policy.SkipTLSVerify
}
