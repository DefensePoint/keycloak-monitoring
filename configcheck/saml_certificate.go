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

// SAMLMetadataFetcher provides functionality to fetch and parse SAML metadata
type SAMLMetadataFetcher struct {
	httpClient *http.Client
	logger     *logger.Logger
}

// NewSAMLMetadataFetcher creates a new SAML metadata fetcher
func NewSAMLMetadataFetcher(timeout time.Duration, log *logger.Logger) *SAMLMetadataFetcher {
	return &SAMLMetadataFetcher{
		httpClient: &http.Client{
			Timeout: timeout,
		},
		logger: log.WithComponent("saml_metadata_fetcher"),
	}
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

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
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
			logger.Str("subject", certInfo.Subject),
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
