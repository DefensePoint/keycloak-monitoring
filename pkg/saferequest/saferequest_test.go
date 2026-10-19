package saferequest

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// stubLookup builds a LookupFunc that returns the given IPs for any hostname.
func stubLookup(ips ...string) LookupFunc {
	return func(_ context.Context, _ string) ([]net.IPAddr, error) {
		out := make([]net.IPAddr, 0, len(ips))
		for _, s := range ips {
			ip := net.ParseIP(s)
			if ip == nil {
				continue
			}
			out = append(out, net.IPAddr{IP: ip})
		}
		return out, nil
	}
}

func TestValidateURL_BlockedRanges(t *testing.T) {
	cases := []struct {
		name string
		url  string
	}{
		// Pentest report PoCs.
		{"cloud_metadata_v4", "http://169.254.169.254/"},
		{"loopback_v4", "http://127.0.0.1/"},
		{"loopback_v4_alt", "http://127.255.255.254/"},
		{"unspecified_v4", "http://0.0.0.0/"},

		// IPv6 variants.
		{"loopback_v6", "http://[::1]/"},
		{"unspecified_v6", "http://[::]/"},
		{"link_local_v6", "http://[fe80::1]/"},

		// IPv4-mapped IPv6 — common bypass.
		{"v4mapped_metadata", "http://[::ffff:169.254.169.254]/"},
		{"v4mapped_loopback", "http://[::ffff:127.0.0.1]/"},

		// Other reserved.
		{"multicast_v4", "http://224.0.0.1/"},
		{"broadcast_v4", "http://255.255.255.255/"},

		// Cloud metadata outside link-local, refused even with private
		// ranges allowed.
		{"alibaba_metadata", "http://100.100.100.200/latest/meta-data/"},
		{"azure_wireserver", "http://168.63.129.16/machine?comp=goalstate"},
		{"oracle_classic_metadata", "http://192.0.0.192/"},
		{"aws_metadata_v6", "http://[fd00:ec2::254]/latest/meta-data/"},
		{"aws_eks_pod_identity_v6", "http://[fd00:ec2::23]/"},
		{"gcp_metadata_v6", "http://[fd20:ce::254]/computeMetadata/v1/"},

		// IPv4 wrapped in IPv6 is judged by the IPv4 address.
		{"nat64_metadata", "http://[64:ff9b::a9fe:a9fe]/"},
		{"nat64_loopback", "http://[64:ff9b::7f00:1]/"},
		{"nat64_alibaba_metadata", "http://[64:ff9b::6464:64c8]/"},
		{"sixtofour_metadata", "http://[2002:a9fe:a9fe::1]/"},
		{"v4compatible_metadata", "http://[::a9fe:a9fe]/"},
	}

	policy := Policy{AllowPrivateRanges: true} // permissive: on-prem default
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateURL(tc.url, policy)
			if err == nil {
				t.Fatalf("expected ValidateURL to reject %s, got nil", tc.url)
			}
			if !errors.Is(err, ErrBlockedAddress) {
				t.Fatalf("expected ErrBlockedAddress for %s, got %v", tc.url, err)
			}
		})
	}
}

func TestValidateURL_PublicAlwaysAllowed(t *testing.T) {
	policy := Policy{
		AllowPrivateRanges: false,
		Lookup:             stubLookup("93.184.216.34"), // example.com
	}
	if err := ValidateURL("https://example.com/", policy); err != nil {
		t.Fatalf("public IP should be allowed, got %v", err)
	}
}

func TestValidateURL_PrivateRangesGatedByPolicy(t *testing.T) {
	const u = "http://10.0.0.5/"

	if err := ValidateURL(u, Policy{AllowPrivateRanges: true}); err != nil {
		t.Fatalf("RFC1918 should be allowed when AllowPrivateRanges=true, got %v", err)
	}
	if err := ValidateURL(u, Policy{AllowPrivateRanges: false}); err == nil {
		t.Fatalf("RFC1918 should be blocked when AllowPrivateRanges=false")
	}
}

func TestValidateURL_HostnameResolvesToBlockedIP(t *testing.T) {
	// Simulates a malicious DNS entry that maps a friendly name to AWS metadata.
	policy := Policy{
		AllowPrivateRanges: true, // even with private allowed, link-local must fail
		Lookup:             stubLookup("169.254.169.254"),
	}
	err := ValidateURL("http://innocent.example.com/", policy)
	if err == nil || !errors.Is(err, ErrBlockedAddress) {
		t.Fatalf("expected ErrBlockedAddress for hostname → metadata IP, got %v", err)
	}
}

func TestValidateURL_MultiAnswerRejectedIfAnyBlocked(t *testing.T) {
	// Resolver returns one public + one blocked. Go's resolver picks one at
	// dial time, so any blocked answer must reject the whole URL.
	policy := Policy{
		AllowPrivateRanges: true,
		Lookup:             stubLookup("93.184.216.34", "127.0.0.1"),
	}
	err := ValidateURL("http://multi.example.com/", policy)
	if err == nil || !errors.Is(err, ErrBlockedAddress) {
		t.Fatalf("multi-A with blocked entry should reject, got %v", err)
	}
}

func TestValidateURL_InvalidScheme(t *testing.T) {
	for _, raw := range []string{
		"file:///etc/passwd",
		"gopher://example.com/",
		"ftp://example.com/",
		"javascript:alert(1)",
	} {
		if err := ValidateURL(raw, Policy{}); err == nil {
			t.Errorf("scheme rejection failed for %s", raw)
		}
	}
}

func TestValidateURL_MalformedURL(t *testing.T) {
	for _, raw := range []string{
		"http://[invalid",
		"://no-scheme",
		"http://",
	} {
		if err := ValidateURL(raw, Policy{}); err == nil {
			t.Errorf("malformed URL should reject: %s", raw)
		}
	}
}

// TestHTTPClient_BlocksRedirectToInternal verifies that the CheckRedirect hook
// refuses to follow a Location header pointing at a blocked target. We don't
// need the redirect target to actually accept the connection — refusing to
// follow at all is the correct behavior.
func TestHTTPClient_BlocksRedirectToInternal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://127.0.0.1:1/internal", http.StatusFound)
	}))
	defer srv.Close()

	client := HTTPClient(Policy{AllowPrivateRanges: true})
	// httptest binds to 127.0.0.1, which our DialContext blocks. Override
	// the dialer so we can actually reach the test server, but keep the
	// CheckRedirect behavior under test.
	client.Transport = http.DefaultTransport

	resp, err := client.Get(srv.URL)
	if err == nil {
		_ = resp.Body.Close()
		t.Fatalf("expected redirect refusal, got status %d", resp.StatusCode)
	}
	if !strings.Contains(err.Error(), "redirect") {
		t.Fatalf("expected redirect-related error, got %v", err)
	}
}

// TestHTTPClient_DialContextBlocksLoopback verifies the syscall-level guard:
// even if validation is bypassed (e.g. caller forgot to call ValidateURL),
// the dialer refuses to connect to a loopback IP. This is the anti-DNS-
// rebinding defense in concrete form.
func TestHTTPClient_DialContextBlocksLoopback(t *testing.T) {
	client := HTTPClient(Policy{AllowPrivateRanges: true})
	resp, err := client.Get("http://127.0.0.1:1/")
	if err == nil {
		_ = resp.Body.Close()
		t.Fatalf("expected dial to be refused for loopback")
	}
	if !strings.Contains(err.Error(), "loopback") {
		t.Fatalf("expected loopback-related error, got %v", err)
	}
}

func TestHTTPClient_DialContextBlocksMetadataIP(t *testing.T) {
	client := HTTPClient(Policy{AllowPrivateRanges: true})
	// 169.254.169.254 should never be reachable through this client.
	_, err := client.Get("http://169.254.169.254/latest/meta-data/")
	if err == nil {
		t.Fatalf("expected dial to be refused for cloud metadata IP")
	}
	if !strings.Contains(err.Error(), "link-local") {
		t.Fatalf("expected link-local error, got %v", err)
	}
}

// The dial-time check refuses the IPv6 metadata endpoint too, so a hostname
// that resolves to it at connect time (DNS rebinding) gets nowhere.
func TestHTTPClient_DialContextBlocksMetadataIPv6(t *testing.T) {
	client := HTTPClient(Policy{AllowPrivateRanges: true})
	_, err := client.Get("http://[fd00:ec2::254]/latest/meta-data/")
	if err == nil {
		t.Fatalf("expected dial to be refused for the IPv6 metadata endpoint")
	}
	if !errors.Is(err, ErrBlockedAddress) || !strings.Contains(err.Error(), "cloud metadata") {
		t.Fatalf("expected a cloud metadata refusal, got %v", err)
	}
}

func TestValidateURL_HostnameResolvesToIPv6Metadata(t *testing.T) {
	policy := Policy{AllowPrivateRanges: true, Lookup: stubLookup("93.184.216.34", "fd00:ec2::254")}
	if err := ValidateURL("https://keycloak.example.com/", policy); !errors.Is(err, ErrBlockedAddress) {
		t.Fatalf("expected a name resolving to the IPv6 metadata endpoint to be refused, got %v", err)
	}
}

// TestHTTPClient_AllowsPublic confirms the client is not so paranoid that it
// breaks legitimate traffic. We use httptest and bypass the dialer (which
// blocks loopback by design) — what we're checking is that nothing OTHER
// than the dial restriction interferes with a normal request/response.
func TestHTTPClient_AllowsPublic(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := HTTPClient(Policy{AllowPrivateRanges: true})
	client.Transport = http.DefaultTransport // bypass dial guard for this assertion

	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("public request failed: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

// TestClassify is a small table-driven sanity check for the IP predicate. The
// higher-level tests above already exercise the same code paths through
// ValidateURL, but having the predicate covered directly makes regressions
// easier to localize.
func TestClassify(t *testing.T) {
	cases := []struct {
		ip           string
		allowPrivate bool
		blocked      bool
	}{
		{"127.0.0.1", true, true},
		{"::1", true, true},
		{"169.254.169.254", true, true},
		{"fe80::1", true, true},
		{"0.0.0.0", true, true},
		{"::", true, true},
		{"224.0.0.1", true, true},
		{"10.0.0.5", true, false},
		{"10.0.0.5", false, true},
		{"192.168.1.1", false, true},
		{"172.16.0.1", false, true},
		{"8.8.8.8", false, false},
		{"8.8.8.8", true, false},

		// Metadata endpoints, whatever the private-range policy.
		{"100.100.100.200", true, true},
		{"168.63.129.16", true, true},
		{"fd00:ec2::254", true, true},
		{"fd20:ce::254", true, true},
		// Their neighbours keep their usual treatment: shared address space
		// (Tailscale and similar), a public address, and ULA gated by policy.
		{"100.100.100.201", true, false},
		{"100.100.100.201", false, false},
		{"168.63.129.17", false, false},
		{"fd00:ec2::1", true, false},
		{"fd00:ec2::1", false, true},

		// Embedded IPv4 follows the policy of the address it carries.
		{"64:ff9b::5db8:d822", false, false}, // NAT64 of public 93.184.216.34
		{"2002:5db8:d822::1", false, false},  // 6to4 of the same
		{"64:ff9b::a00:5", true, false},      // NAT64 of 10.0.0.5, private allowed
		{"64:ff9b::a00:5", false, true},      // and refused when it is not
		{"2001:db8::1", false, false},        // an ordinary IPv6 address is untouched
	}
	for _, tc := range cases {
		ip := net.ParseIP(tc.ip)
		if ip == nil {
			t.Fatalf("bad test data: %s", tc.ip)
		}
		got := classify(ip, tc.allowPrivate)
		if (got != "") != tc.blocked {
			t.Errorf("classify(%s, allowPrivate=%v): blocked=%v, reason=%q", tc.ip, tc.allowPrivate, got != "", got)
		}
	}
}
