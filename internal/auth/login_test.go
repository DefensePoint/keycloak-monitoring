package auth

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
)

// The SSO login used only state. A stolen or injected authorization code could
// be redeemed, and an ID token from another login accepted. The login now sends
// a PKCE challenge and a nonce, and the callback checks both.

func testOAuthProvider(tokenURL string) *OIDCProvider {
	return &OIDCProvider{
		config: &Config{},
		oauth2Config: &oauth2.Config{
			ClientID:     "kmt",
			ClientSecret: "secret",
			RedirectURL:  "https://kmt.test/auth/callback",
			Endpoint:     oauth2.Endpoint{AuthURL: "https://idp.test/auth", TokenURL: tokenURL},
		},
		log: logger.NewNoop(),
	}
}

func TestLogin_SendsAnS256ChallengeAndANonce(t *testing.T) {
	svc := NewService(testOAuthProvider("https://idp.test/token"), nil, nil, logger.NewNoop(), &Config{})

	raw, attempt, err := svc.Login(context.Background())
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if attempt.State == "" || attempt.Nonce == "" || attempt.CodeVerifier == "" {
		t.Fatalf("attempt = %+v, want state, nonce and verifier all set", attempt)
	}
	if attempt.State == attempt.Nonce {
		t.Error("state and nonce must be independent values")
	}

	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse %q: %v", raw, err)
	}
	q := u.Query()
	if got := q.Get("state"); got != attempt.State {
		t.Errorf("state = %q, want %q", got, attempt.State)
	}
	if got := q.Get("nonce"); got != attempt.Nonce {
		t.Errorf("nonce = %q, want %q", got, attempt.Nonce)
	}
	if got := q.Get("code_challenge_method"); got != "S256" {
		t.Errorf("code_challenge_method = %q, want S256", got)
	}
	if got, want := q.Get("code_challenge"), oauth2.S256ChallengeFromVerifier(attempt.CodeVerifier); got != want {
		t.Errorf("code_challenge = %q, want the S256 hash of the verifier %q", got, want)
	}
	if q.Get("code_verifier") != "" {
		t.Error("the verifier itself must never leave the server in the authorization request")
	}

	_, second, err := svc.Login(context.Background())
	if err != nil {
		t.Fatalf("second Login: %v", err)
	}
	if second.State == attempt.State || second.Nonce == attempt.Nonce || second.CodeVerifier == attempt.CodeVerifier {
		t.Error("each login must get fresh values")
	}
}

func TestExchange_SendsTheVerifier(t *testing.T) {
	var gotVerifier string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotVerifier = r.PostForm.Get("code_verifier")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"a","token_type":"Bearer","expires_in":60}`))
	}))
	defer srv.Close()

	p := testOAuthProvider(srv.URL)
	if _, err := p.Exchange(context.Background(), "code", oauth2.VerifierOption("the-verifier")); err != nil {
		t.Fatalf("Exchange: %v", err)
	}
	if gotVerifier != "the-verifier" {
		t.Errorf("token request code_verifier = %q, want the login's verifier", gotVerifier)
	}
}

// signIDToken returns an RS256 ID token for issuer and audience "kmt", with the
// nonce claim when nonce is not empty.
func signIDToken(t *testing.T, key *rsa.PrivateKey, issuer, nonce string) string {
	t.Helper()
	enc := func(v interface{}) string {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		return base64.RawURLEncoding.EncodeToString(b)
	}
	claims := map[string]interface{}{
		"iss": issuer, "aud": "kmt", "sub": "user-1",
		"iat": time.Now().Unix(), "exp": time.Now().Add(time.Hour).Unix(),
	}
	if nonce != "" {
		claims["nonce"] = nonce
	}
	signingInput := enc(map[string]string{"alg": "RS256", "typ": "JWT"}) + "." + enc(claims)
	sum := sha256.Sum256([]byte(signingInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
	if err != nil {
		t.Fatal(err)
	}
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func TestVerifyLoginIDToken_RequiresTheLoginsNonce(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	const issuer = "https://idp.test/realms/r"
	p := &OIDCProvider{
		config: &Config{},
		verifier: oidc.NewVerifier(issuer,
			&oidc.StaticKeySet{PublicKeys: []crypto.PublicKey{&key.PublicKey}},
			&oidc.Config{ClientID: "kmt"}),
		log: logger.NewNoop(),
	}
	ctx := context.Background()

	withNonce := signIDToken(t, key, issuer, "nonce-of-this-login")
	if info, err := p.VerifyLoginIDToken(ctx, withNonce, "nonce-of-this-login"); err != nil || info.Subject != "user-1" {
		t.Fatalf("matching nonce: info=%+v err=%v, want user-1 accepted", info, err)
	}

	for name, tc := range map[string]struct{ token, nonce string }{
		"another login's nonce":   {withNonce, "nonce-of-another-login"},
		"token without a nonce":   {signIDToken(t, key, issuer, ""), "nonce-of-this-login"},
		"login without a nonce":   {withNonce, ""},
		"token signed by someone": {signIDToken(t, mustKey(t), issuer, "nonce-of-this-login"), "nonce-of-this-login"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := p.VerifyLoginIDToken(ctx, tc.token, tc.nonce)
			var authErr *AuthError
			if !errors.As(err, &authErr) || authErr.Code != ErrCodeInvalidToken {
				t.Errorf("err = %v, want an %s refusal", err, ErrCodeInvalidToken)
			}
		})
	}
}

func mustKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return k
}
