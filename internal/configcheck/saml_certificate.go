package configcheck

import (
	"context"
	"crypto/x509"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/pkg/saferequest"
)

// SAMLMetadata represents the structure of SAML metadata XML
type SAMLMetadata struct {
	XMLName    xml.Name         `xml:"EntityDescriptor"`
	EntityID   string           `xml:"entityID,attr"`
	IDPSSODesc IDPSSODescriptor `xml:"IDPSSODescriptor"`
}

// IDPSSODescriptor contains SAML IDP configuration
type IDPSSODescriptor struct {
	KeyDescriptors []KeyDescriptor `xml:"KeyDescriptor"`
}

// KeyDescriptor contains certificate information
type KeyDescriptor struct {
	Use     string  `xml:"use,attr"`
	KeyInfo KeyInfo `xml:"KeyInfo"`
}

// KeyInfo contains the X509 certificate data
type KeyInfo struct {
	X509Data X509Data `xml:"X509Data"`
}

// X509Data contains certificate data
type X509Data struct {
	X509Certificate string `xml:"X509Certificate"`
}

// CertificateInfo contains parsed certificate information
type CertificateInfo struct {
	Certificate *x509.Certificate
	Use         string // "signing" or "encryption"
	NotAfter    time.Time
	NotBefore   time.Time
	Subject     string
	Issuer      string
	SerialNum   string
}

// CertificateExpirationStatus represents the expiration status of a certificate
type CertificateExpirationStatus struct {
	IsExpired           bool
	IsExpiringSoon      bool
	DaysUntilExpiry     int
	Certificate         *CertificateInfo
	MetadataURL         string
	FetchedSuccessfully bool
	ErrorMessage        string
}

// maxSAMLMetadataBytes caps how much of a metadata document is read. One
// identity provider's metadata is a few kilobytes; without a cap a server could
// stream until memory ran out.
const maxSAMLMetadataBytes = 10 << 20

// maxSAMLMetadataRedirects bounds the redirects followed for one fetch.
const maxSAMLMetadataRedirects = 3

// SAMLMetadataFetcher provides functionality to fetch and parse SAML metadata
type SAMLMetadataFetcher struct {
	httpClient *http.Client
	policy     saferequest.Policy
	logger     *logger.Logger
}

// NewSAMLMetadataFetcher creates a new SAML metadata fetcher.
//
// The metadata URL is the identity provider's metadataDescriptorUrl, which
// anyone administering that realm in Keycloak can set, and the fetch runs from
// this server's network. policy is the same SSRF policy the Keycloak clients
// use: loopback, link-local (cloud metadata) and other never-legitimate ranges
// are refused at dial time, and private ranges follow AllowPrivateRanges.
func NewSAMLMetadataFetcher(policy saferequest.Policy, log *logger.Logger) *SAMLMetadataFetcher {
	client := saferequest.HTTPClient(policy)
	// saferequest refuses every redirect. Metadata URLs do redirect (http to
	// https, a CDN), and the dial-time check still runs on each hop, so follow
	// a few, never from https down to http.
	client.CheckRedirect = checkSAMLMetadataRedirect
	return &SAMLMetadataFetcher{
		httpClient: client,
		policy:     policy,
		logger:     log.WithComponent("saml_metadata_fetcher"),
	}
}

func checkSAMLMetadataRedirect(req *http.Request, via []*http.Request) error {
	// via holds the requests made so far, so this redirect is number len(via).
	if len(via) > maxSAMLMetadataRedirects {
		return fmt.Errorf("stopped after %d redirects", maxSAMLMetadataRedirects)
	}
	if req.URL.Scheme != "https" && req.URL.Scheme != "http" {
		return fmt.Errorf("redirect to %q scheme refused", req.URL.Scheme)
	}
	if via[len(via)-1].URL.Scheme == "https" && req.URL.Scheme == "http" {
		return fmt.Errorf("redirect from https to http refused")
	}
	return nil
}

// FetchAndParseCertificates fetches SAML metadata from a URL and extracts certificates
func (f *SAMLMetadataFetcher) FetchAndParseCertificates(ctx context.Context, metadataURL string) ([]*CertificateInfo, error) {
	// Fetch metadata
	metadata, err := f.fetchMetadata(ctx, metadataURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch metadata: %w", err)
	}

	// Parse certificates
	certificates, err := f.parseCertificates(metadata)
	if err != nil {
		return nil, fmt.Errorf("failed to parse certificates: %w", err)
	}

	return certificates, nil
}

// fetchMetadata fetches SAML metadata from the given URL
func (f *SAMLMetadataFetcher) fetchMetadata(ctx context.Context, metadataURL string) (*SAMLMetadata, error) {
	f.logger.Debug("Fetching SAML metadata", logger.Str("url", metadataURL))

	// Refuse a blocked address before connecting, for a clear error. The
	// dialer checks again at connect time, which also covers redirects and a
	// DNS answer that changes between the two.
	if err := saferequest.ValidateURLContext(ctx, metadataURL, f.policy); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, metadataURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch metadata: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			f.logger.Warn("Failed to close response body", logger.Err(closeErr))
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxSAMLMetadataBytes+1))
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	if len(body) > maxSAMLMetadataBytes {
		return nil, fmt.Errorf("metadata is larger than %d bytes", maxSAMLMetadataBytes)
	}

	var metadata SAMLMetadata
	if err := xml.Unmarshal(body, &metadata); err != nil {
		return nil, fmt.Errorf("failed to parse XML: %w", err)
	}

	f.logger.Debug("Successfully fetched and parsed SAML metadata",
		logger.Str("entity_id", metadata.EntityID),
		logger.Int("key_descriptors", len(metadata.IDPSSODesc.KeyDescriptors)))

	return &metadata, nil
}

// parseCertificates extracts and parses X.509 certificates from SAML metadata
func (f *SAMLMetadataFetcher) parseCertificates(metadata *SAMLMetadata) ([]*CertificateInfo, error) {
	var certificates []*CertificateInfo

	for _, keyDesc := range metadata.IDPSSODesc.KeyDescriptors {
		certData := strings.TrimSpace(keyDesc.KeyInfo.X509Data.X509Certificate)
		if certData == "" {
			continue
		}

		// Decode base64 certificate
		certBytes, err := base64.StdEncoding.DecodeString(certData)
		if err != nil {
			f.logger.Warn("Failed to decode certificate",
				logger.Str("use", keyDesc.Use),
				logger.Err(err))
			continue
		}

		// Parse X.509 certificate
		cert, err := x509.ParseCertificate(certBytes)
		if err != nil {
			f.logger.Warn("Failed to parse X.509 certificate",
				logger.Str("use", keyDesc.Use),
				logger.Err(err))
			continue
		}

		certInfo := &CertificateInfo{
			Certificate: cert,
			Use:         keyDesc.Use,
			NotAfter:    cert.NotAfter,
			NotBefore:   cert.NotBefore,
			Subject:     cert.Subject.String(),
			Issuer:      cert.Issuer.String(),
			SerialNum:   cert.SerialNumber.String(),
		}

		certificates = append(certificates, certInfo)

		f.logger.Debug("Parsed certificate",
			logger.Str("use", keyDesc.Use),
			logger.Str("certificate_subject", certInfo.Subject),
			logger.Time("not_after", certInfo.NotAfter),
			logger.Time("not_before", certInfo.NotBefore))
	}

	if len(certificates) == 0 {
		return nil, fmt.Errorf("no valid certificates found in metadata")
	}

	return certificates, nil
}

// CheckCertificateExpiration checks if a certificate is expired or expiring soon
func CheckCertificateExpiration(cert *CertificateInfo, warningDays int, now time.Time) CertificateExpirationStatus {
	status := CertificateExpirationStatus{
		Certificate:         cert,
		FetchedSuccessfully: true,
	}

	// Check if expired
	if now.After(cert.NotAfter) {
		status.IsExpired = true
		status.DaysUntilExpiry = int(now.Sub(cert.NotAfter).Hours() / 24)
		return status
	}

	// Calculate days until expiry
	daysUntilExpiry := int(cert.NotAfter.Sub(now).Hours() / 24)
	status.DaysUntilExpiry = daysUntilExpiry

	// Check if expiring soon
	if daysUntilExpiry <= warningDays {
		status.IsExpiringSoon = true
	}

	return status
}
