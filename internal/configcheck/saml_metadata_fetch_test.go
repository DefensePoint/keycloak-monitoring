package configcheck

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/pkg/saferequest"
)

// The metadata URL comes from an identity provider's metadataDescriptorUrl,
// which a realm administrator sets, and the fetch runs from this server.
func TestSAMLMetadataFetch_RefusesBlockedAddresses(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
	}))
	defer srv.Close()

	// Private ranges allowed, as on-prem deployments run: loopback and
	// link-local must still be refused.
	f := NewSAMLMetadataFetcher(saferequest.Policy{AllowPrivateRanges: true, Timeout: 2 * time.Second}, logger.NewNoop())
	for _, u := range []string{
		srv.URL + "/metadata",
		"http://169.254.169.254/latest/meta-data/",
		"http://[::1]:9/metadata",
	} {
		_, err := f.FetchAndParseCertificates(context.Background(), u)
		if !errors.Is(err, saferequest.ErrBlockedAddress) {
			t.Errorf("%s: err = %v, want ErrBlockedAddress", u, err)
		}
	}
	if n := hits.Load(); n != 0 {
		t.Fatalf("the loopback server was reached %d times", n)
	}
}

func TestSAMLMetadataFetch_PrivateRangesFollowPolicy(t *testing.T) {
	f := NewSAMLMetadataFetcher(saferequest.Policy{AllowPrivateRanges: false, Timeout: 2 * time.Second}, logger.NewNoop())
	_, err := f.FetchAndParseCertificates(context.Background(), "http://10.0.0.1/metadata")
	if !errors.Is(err, saferequest.ErrBlockedAddress) {
		t.Fatalf("private address with AllowPrivateRanges=false: err = %v, want ErrBlockedAddress", err)
	}
}

// fetcherAgainst returns a fetcher whose metadata host resolves to a public
// address for the pre-check, and whose client connects to srv. The guarded
// client refuses loopback, so the size and redirect rules are tested with the
// fetcher's own redirect policy on a plain transport.
func fetcherAgainst(srv *httptest.Server) *SAMLMetadataFetcher {
	addr := srv.Listener.Addr().String()
	return &SAMLMetadataFetcher{
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
					return (&net.Dialer{}).DialContext(ctx, network, addr)
				},
			},
			CheckRedirect: checkSAMLMetadataRedirect,
		},
		policy: saferequest.Policy{Lookup: func(context.Context, string) ([]net.IPAddr, error) {
			return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
		}},
		logger: logger.NewNoop(),
	}
}

func testMetadataXML(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "idp.example"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(90 * 24 * time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf(`<EntityDescriptor entityID="https://idp.example"><IDPSSODescriptor>`+
		`<KeyDescriptor use="signing"><KeyInfo><X509Data><X509Certificate>%s</X509Certificate></X509Data></KeyInfo></KeyDescriptor>`+
		`</IDPSSODescriptor></EntityDescriptor>`, base64.StdEncoding.EncodeToString(der))
}

func TestSAMLMetadataFetch_ReadsNormalMetadata(t *testing.T) {
	doc := testMetadataXML(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(doc))
	}))
	defer srv.Close()

	certs, err := fetcherAgainst(srv).FetchAndParseCertificates(context.Background(), "http://metadata.example/idp")
	if err != nil {
		t.Fatalf("FetchAndParseCertificates: %v", err)
	}
	if len(certs) != 1 || certs[0].Use != "signing" {
		t.Fatalf("certs = %+v, want one signing certificate", certs)
	}
}

func TestSAMLMetadataFetch_StopsAtTheSizeCap(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		chunk := []byte(strings.Repeat("A", 64<<10))
		for written := 0; written <= maxSAMLMetadataBytes; written += len(chunk) {
			if _, err := w.Write(chunk); err != nil {
				return
			}
		}
	}))
	defer srv.Close()

	_, err := fetcherAgainst(srv).FetchAndParseCertificates(context.Background(), "http://metadata.example/huge")
	if err == nil || !strings.Contains(err.Error(), "larger than") {
		t.Fatalf("oversized metadata: err = %v, want a size error", err)
	}
}

func TestSAMLMetadataFetch_FollowsAFewRedirects(t *testing.T) {
	doc := testMetadataXML(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/final" {
			_, _ = w.Write([]byte(doc))
			return
		}
		http.Redirect(w, r, "/final", http.StatusFound)
	}))
	defer srv.Close()

	if _, err := fetcherAgainst(srv).FetchAndParseCertificates(context.Background(), "http://metadata.example/start"); err != nil {
		t.Fatalf("one redirect: %v", err)
	}
}

func TestSAMLMetadataFetch_StopsARedirectLoop(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		http.Redirect(w, r, fmt.Sprintf("/hop%d", hits.Load()), http.StatusFound)
	}))
	defer srv.Close()

	_, err := fetcherAgainst(srv).FetchAndParseCertificates(context.Background(), "http://metadata.example/loop")
	if err == nil {
		t.Fatal("a redirect loop was followed to the end")
	}
	if n := hits.Load(); n != maxSAMLMetadataRedirects+1 {
		t.Fatalf("server saw %d requests, want %d", n, maxSAMLMetadataRedirects+1)
	}
}

func TestCheckSAMLMetadataRedirect(t *testing.T) {
	req := func(raw string) *http.Request {
		u, _ := url.Parse(raw)
		return &http.Request{URL: u}
	}
	via := func(urls ...string) []*http.Request {
		out := make([]*http.Request, len(urls))
		for i, u := range urls {
			out[i] = req(u)
		}
		return out
	}
	for _, tc := range []struct {
		name string
		to   string
		via  []*http.Request
		ok   bool
	}{
		{"http to https", "https://idp.example/m", via("http://idp.example/m"), true},
		{"https to https", "https://cdn.example/m", via("https://idp.example/m"), true},
		{"https down to http", "http://idp.example/m", via("https://idp.example/m"), false},
		{"other scheme", "file:///etc/passwd", via("https://idp.example/m"), false},
		{"third redirect", "https://idp.example/4", via("https://idp.example/1", "https://idp.example/2", "https://idp.example/3"), true},
		{"over the limit", "https://idp.example/5", via("https://idp.example/1", "https://idp.example/2", "https://idp.example/3", "https://idp.example/4"), false},
	} {
		err := checkSAMLMetadataRedirect(req(tc.to), tc.via)
		if (err == nil) != tc.ok {
			t.Errorf("%s: err = %v, want ok=%v", tc.name, err, tc.ok)
		}
	}
}
