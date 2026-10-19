package auth

import (
	"crypto/sha256"
	"crypto/subtle"
	"io"
	"net/http"
	"time"

	"github.com/gorilla/sessions"
	"golang.org/x/crypto/hkdf"
)

// Keys of the values the login cookie carries.
const (
	loginStateKey    = "state"
	loginNonceKey    = "nonce"
	loginVerifierKey = "verifier"
)

// LoginCookie keeps an SSO login in progress in the browser that started it,
// instead of in this process's memory, so any API instance can complete the
// login and a restart in between does not lose it.
//
// The cookie has its own store rather than reusing the session's, for two
// properties the session store does not give:
//   - It is encrypted as well as signed. The PKCE verifier and the nonce must
//     not be readable, not even by the browser holding them.
//   - Its codec's max age is the login's, not the session's. gorilla enforces
//     expiry from the signed timestamp using the store-wide max age, so a
//     short MaxAge set on one cookie of the session store would only tell the
//     browser when to drop it.
//
// Both keys are derived from the session secret, so every instance sharing
// that secret reads the cookie, and no new setting is needed.
type LoginCookie struct {
	store  *sessions.CookieStore
	name   string
	maxAge time.Duration
}

// NewLoginCookie returns the login cookie for the session named sessionName.
//
// SameSite is Lax whatever the session cookie's setting: the provider sends
// the browser back with a cross-site top-level redirect, which a Strict cookie
// would not accompany, and the callback would then refuse every login.
func NewLoginCookie(secret, sessionName string, maxAge time.Duration, secure bool) (*LoginCookie, error) {
	hashKey, err := deriveLoginCookieKey(secret, "kmt sso login cookie hmac-sha256")
	if err != nil {
		return nil, err
	}
	blockKey, err := deriveLoginCookieKey(secret, "kmt sso login cookie aes-256")
	if err != nil {
		return nil, err
	}

	store := sessions.NewCookieStore(hashKey, blockKey)
	store.Options = &sessions.Options{
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	}
	// Sets both the cookie's MaxAge and the codecs' max age, so an expired
	// login cookie fails to decode however long the browser kept it.
	store.MaxAge(int(maxAge / time.Second))

	return &LoginCookie{store: store, name: sessionName + "_login", maxAge: maxAge}, nil
}

// deriveLoginCookieKey derives a 32-byte key for one purpose from the session
// secret. Separate keys per purpose keep the login cookie's keys apart from
// the session cookie's, which uses the secret itself.
func deriveLoginCookieKey(secret, purpose string) ([]byte, error) {
	key := make([]byte, 32)
	if _, err := io.ReadFull(hkdf.New(sha256.New, []byte(secret), nil, []byte(purpose)), key); err != nil {
		return nil, err
	}
	return key, nil
}

// Name returns the cookie's name.
func (c *LoginCookie) Name() string {
	return c.name
}

// MaxAge is how long a login may take between its start and its callback.
func (c *LoginCookie) MaxAge() time.Duration {
	return c.maxAge
}

// Save records the login attempt in the browser that started it. A browser
// holds one login at a time: starting another replaces it.
func (c *LoginCookie) Save(w http.ResponseWriter, r *http.Request, attempt LoginAttempt) error {
	if attempt.State == "" {
		return &AuthError{Code: ErrCodeInvalidState, Message: "login attempt has no state"}
	}
	// A cookie that no longer decodes (expired, or from before a key change)
	// still yields a usable new session alongside the error.
	session, err := c.store.Get(r, c.name)
	if session == nil {
		return err
	}
	session.Values = map[interface{}]interface{}{
		loginStateKey:    attempt.State,
		loginNonceKey:    attempt.Nonce,
		loginVerifierKey: attempt.CodeVerifier,
	}
	secureOverHTTPS(r, session)
	return c.store.Save(r, w, session)
}

// Take returns the login this browser started if its state is the given one,
// and clears the cookie: that login is being completed.
//
// It reports false when the browser holds no login, holds an expired or
// tampered one, or holds another login's. A callback for another state leaves
// the cookie alone: it may be a stale tab or a link someone else sent, and
// neither should cancel the login this browser actually has in progress.
//
// Clearing the cookie does not stop a replay that carries a copy of it. The
// authorization code does: the provider accepts each code once (RFC 6749
// section 4.1.2), so a replayed callback fails at the code exchange.
func (c *LoginCookie) Take(w http.ResponseWriter, r *http.Request, state string) (LoginAttempt, bool) {
	session, err := c.store.Get(r, c.name)
	if err != nil || session == nil || session.IsNew {
		return LoginAttempt{}, false
	}
	bound, _ := session.Values[loginStateKey].(string)
	if bound == "" || subtle.ConstantTimeCompare([]byte(bound), []byte(state)) != 1 {
		return LoginAttempt{}, false
	}
	nonce, _ := session.Values[loginNonceKey].(string)
	verifier, _ := session.Values[loginVerifierKey].(string)

	session.Values = map[interface{}]interface{}{}
	session.Options.MaxAge = -1
	secureOverHTTPS(r, session)
	_ = c.store.Save(r, w, session)

	return LoginAttempt{State: bound, Nonce: nonce, CodeVerifier: verifier}, true
}
