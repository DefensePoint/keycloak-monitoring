package auth

import (
	"bytes"
	"strings"
	"testing"

	"github.com/rs/zerolog"

	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
)

// An absent email_verified claim and an IdP that reports false decode to the
// same Go value, so a realm whose client is missing the email scope marks
// everyone unverified with nothing in the logs to explain it. The warning is
// the only signal that distinguishes the two, and it has to stay rare: this
// runs on every login, so a misconfiguration that logged per request would bury
// the log rather than inform it.

func newClaimWarnProvider(buf *bytes.Buffer) *OIDCProvider {
	return &OIDCProvider{
		config: &Config{ProviderURL: "https://idp.invalid/realms/x", ClientID: "point"},
		log:    logger.New(zerolog.New(buf)),
	}
}

func TestWarnIfEmailVerifiedAbsent_WarnsOnceWhenClaimMissing(t *testing.T) {
	var buf bytes.Buffer
	p := newClaimWarnProvider(&buf)

	missing := map[string]interface{}{"sub": "abc", "email": "person@example.com"}
	for i := 0; i < 3; i++ {
		p.warnIfEmailVerifiedAbsent(missing, "id_token")
	}

	got := strings.Count(buf.String(), "email_verified")
	if got != 1 {
		t.Errorf("warned %d times for %d logins, want exactly 1: the warning must survive a "+
			"busy login path without flooding it", got, 3)
	}
	if !strings.Contains(buf.String(), "email scope") {
		t.Error("the warning should name the client configuration to check, not just report a symptom")
	}
}

func TestWarnIfEmailVerifiedAbsent_SilentWhenClaimPresent(t *testing.T) {
	for _, claim := range []interface{}{true, false} {
		var buf bytes.Buffer
		p := newClaimWarnProvider(&buf)

		p.warnIfEmailVerifiedAbsent(map[string]interface{}{
			"sub": "abc", "email_verified": claim,
		}, "id_token")

		if buf.Len() != 0 {
			t.Errorf("claim=%v: warned although the IdP sent the claim: %s", claim, buf.String())
		}
	}
}

// A provider built without a logger must not panic: the field is only set by
// NewOIDCProvider, and tests and future callers may construct one directly.
func TestWarnIfEmailVerifiedAbsent_NoLoggerIsSafe(t *testing.T) {
	p := &OIDCProvider{config: &Config{}}
	p.warnIfEmailVerifiedAbsent(map[string]interface{}{"sub": "abc"}, "id_token")
}
