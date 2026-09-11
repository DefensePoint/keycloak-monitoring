// Package saferequest provides SSRF protections for outbound HTTP requests
// made on behalf of user-supplied URLs (e.g. tenant Keycloak server_url).
//
// Two layers of defense are exposed:
//
//  1. ValidateURL — parses the URL, resolves the hostname, and rejects targets
//     that resolve to ranges that are never legitimate destinations (loopback,
//     link-local / cloud metadata, unspecified, multicast). RFC1918 / ULA
//     ranges are rejected only when Policy.AllowPrivateRanges is false.
//
//  2. HTTPClient — returns an *http.Client whose Transport revalidates the
//     resolved IP at the moment of the connect() syscall, defeating DNS
//     rebinding (resolver returns a public IP at validation, a private one at
//     dial). Redirects are disabled so a malicious server cannot bounce the
//     client to an internal target via the Location header.
//
// The package is deliberately conservative about error messages: callers
// should surface a generic "connection failed" to end users, while the
// detailed reason is available in the error wrapped here for server-side
// logging.
package saferequest

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"time"
)

// LookupFunc resolves a hostname to a list of IPs. Production code uses
// net.DefaultResolver.LookupIPAddr; tests inject a deterministic stub.
type LookupFunc func(ctx context.Context, host string) ([]net.IPAddr, error)

// Policy controls how outbound URLs are validated.
type Policy struct {
	// AllowPrivateRanges, when true, permits RFC1918 (10.0.0.0/8,
	// 172.16.0.0/12, 192.168.0.0/16) and IPv6 ULA (fc00::/7) destinations.
	// On-prem deployments should keep this true so customers can target
	// Keycloak instances on their own internal network.
	AllowPrivateRanges bool

	// Timeout is the per-request HTTP timeout used by HTTPClient.
	// Zero defers to a sane default (30s).
	Timeout time.Duration

	// SkipTLSVerify is plumbed through unchanged for parity with the
	// existing Keycloak client; SSRF protections do not depend on it.
	SkipTLSVerify bool

	// Lookup overrides hostname resolution. Leave nil in production.
	Lookup LookupFunc
}

func (p Policy) lookup() LookupFunc {
	if p.Lookup != nil {
		return p.Lookup
	}
	return net.DefaultResolver.LookupIPAddr
}

// ErrBlockedAddress signals that a URL or its post-resolution IP fell into a
// disallowed range. Callers should not echo the wrapped detail back to the
// HTTP client; surface a generic "connection failed" to the user instead.
var ErrBlockedAddress = errors.New("address blocked by SSRF policy")

// ValidateURL parses rawURL, ensures the scheme is http(s), resolves the host
// to one or more IPs, and rejects the request if ANY resolved IP falls into a
// blocked range. Multi-A records with a single bad answer are rejected — the
// resolver controls which address would be dialed, so a single tainted entry
// is enough to make the request unsafe.
func ValidateURL(rawURL string, policy Policy) error {
	return ValidateURLContext(context.Background(), rawURL, policy)
}

// ValidateURLContext is ValidateURL with an explicit context for the DNS
// lookup.
func ValidateURLContext(ctx context.Context, rawURL string, policy Policy) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("%w: invalid URL", ErrBlockedAddress)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("%w: scheme %q not allowed", ErrBlockedAddress, u.Scheme)
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("%w: empty host", ErrBlockedAddress)
	}

	// If the host is already an IP literal, validate it directly.
	if ip := net.ParseIP(host); ip != nil {
		if reason := classify(ip, policy.AllowPrivateRanges); reason != "" {
			return fmt.Errorf("%w: %s", ErrBlockedAddress, reason)
		}
		return nil
	}

	// Otherwise resolve and validate every answer. We check ALL of them: if
	// any one is unsafe, the resolver's choice at dial time could land on it.
	addrs, err := policy.lookup()(ctx, host)
	if err != nil {
		return fmt.Errorf("%w: dns lookup failed: %v", ErrBlockedAddress, err)
	}
	if len(addrs) == 0 {
		return fmt.Errorf("%w: no addresses resolved", ErrBlockedAddress)
	}
	for _, a := range addrs {
		if reason := classify(a.IP, policy.AllowPrivateRanges); reason != "" {
			return fmt.Errorf("%w: %s", ErrBlockedAddress, reason)
		}
	}
	return nil
}

// classify returns a human-readable reason if the IP is disallowed, or "" if
// it is acceptable. The reason is intentionally specific for server-side
// logging; do NOT surface it to API callers.
func classify(ip net.IP, allowPrivate bool) string {
	if ip == nil {
		return "nil ip"
	}
	// Normalize IPv4-mapped IPv6 (e.g. ::ffff:169.254.169.254) to its v4
	// form so the standard-library predicates apply uniformly.
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	}

	switch {
	case ip.IsUnspecified():
		return "unspecified address"
	case ip.IsLoopback():
		return "loopback address"
	case ip.IsLinkLocalUnicast(), ip.IsLinkLocalMulticast():
		return "link-local address"
	case ip.IsInterfaceLocalMulticast(), ip.IsMulticast():
		return "multicast address"
	}

	// Broadcast-ish v4 addresses that the stdlib does not catch above.
	if v4 := ip.To4(); v4 != nil && v4[3] == 255 && v4[0] == 255 && v4[1] == 255 && v4[2] == 255 {
		return "broadcast address"
	}

	if !allowPrivate && ip.IsPrivate() {
		return "private address"
	}
	return ""
}

// HTTPClient builds an *http.Client that enforces policy at the transport
// layer. The dialer's Control hook re-checks the resolved IP at the moment of
// the connect() syscall, which means a DNS resolver that returns a safe IP
// during ValidateURL but a malicious one milliseconds later cannot bypass the
// check. Redirects are explicitly refused: Keycloak admin endpoints do not
// redirect, and a Location header is a textbook SSRF pivot.
func HTTPClient(policy Policy) *http.Client {
	timeout := policy.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	dialer := &net.Dialer{
		Timeout:   timeout,
		KeepAlive: 30 * time.Second,
		Control: func(network, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return fmt.Errorf("%w: malformed dial address", ErrBlockedAddress)
			}
			ip := net.ParseIP(host)
			if ip == nil {
				// Control runs after resolution: address must be an IP
				// literal here. If it isn't, fail closed.
				return fmt.Errorf("%w: dial host is not an IP literal", ErrBlockedAddress)
			}
			if reason := classify(ip, policy.AllowPrivateRanges); reason != "" {
				return fmt.Errorf("%w: %s", ErrBlockedAddress, reason)
			}
			if !strings.HasPrefix(network, "tcp") {
				return fmt.Errorf("%w: network %q not allowed", ErrBlockedAddress, network)
			}
			return nil
		},
	}

	transport := &http.Transport{
		DialContext:           dialer.DialContext,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ForceAttemptHTTP2:     true,
	}
	if policy.SkipTLSVerify {
		transport.TLSClientConfig = newSkipVerifyTLSConfig()
	}

	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return fmt.Errorf("%w: redirect to %s refused", ErrBlockedAddress, req.URL.Redacted())
		},
	}
}
