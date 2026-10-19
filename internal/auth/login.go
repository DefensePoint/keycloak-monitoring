package auth

import (
	"golang.org/x/oauth2"
)

// LoginAttempt is what one SSO login has to present again at its callback.
//
// State ties the callback to the login that started it. Nonce is sent with
// the authorization request and must come back in the ID token, so an ID token
// issued for another login is refused. CodeVerifier is the PKCE secret: the
// authorization request carries only its S256 hash, and the code can only be
// redeemed with the verifier itself, so a stolen or injected code is useless.
type LoginAttempt struct {
	State        string
	Nonce        string
	CodeVerifier string
}

// NewLoginAttempt returns a login attempt with fresh random values. It fails
// rather than falling back to anything predictable: each value is only worth
// having if nobody else can guess it.
func NewLoginAttempt() (LoginAttempt, error) {
	state, err := GenerateRandomState()
	if err != nil {
		return LoginAttempt{}, &AuthError{Code: ErrCodeProviderError, Message: "failed to generate login state", Err: err}
	}
	nonce, err := GenerateRandomState()
	if err != nil {
		return LoginAttempt{}, &AuthError{Code: ErrCodeProviderError, Message: "failed to generate login nonce", Err: err}
	}
	return LoginAttempt{State: state, Nonce: nonce, CodeVerifier: oauth2.GenerateVerifier()}, nil
}
