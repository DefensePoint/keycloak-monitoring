package auth

import (
	"net/http"
	"strings"

	"github.com/gorilla/sessions"
)

// RequestIsHTTPS reports whether the browser reached KMT over HTTPS: over TLS
// to this server, or through a proxy that terminated TLS and said so in
// X-Forwarded-Proto, as Traefik does. The first value is the client's side of
// the outermost proxy.
//
// It is only used to add the Secure flag to a cookie, never to remove it, so
// it needs no trusted-proxy check: a client that forges the header only keeps
// its own cookie off plain HTTP.
func RequestIsHTTPS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	proto, _, _ := strings.Cut(r.Header.Get("X-Forwarded-Proto"), ",")
	return strings.EqualFold(strings.TrimSpace(proto), "https")
}

// secureOverHTTPS marks the cookie a session is about to write Secure when the
// request came over HTTPS, whatever auth.session.secure says. A cookie set
// over HTTPS has no reason to travel over plain HTTP later, which is when a
// network observer could take it.
func secureOverHTTPS(r *http.Request, s *sessions.Session) {
	if s != nil && s.Options != nil && RequestIsHTTPS(r) {
		s.Options.Secure = true
	}
}
