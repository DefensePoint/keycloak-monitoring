package configcheck

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/pkg/keycloakadmin"
	"github.com/DefensePoint/keycloak-monitoring/pkg/saferequest"
)

type recordingNotifier struct {
	mu       sync.Mutex
	sent     []domain.Alert
	resolved []domain.Alert
}

func (n *recordingNotifier) NotifyAlertResolution(_ context.Context, alert *domain.Alert) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.resolved = append(n.resolved, *alert)
	return nil
}

func (n *recordingNotifier) NotifyAlert(_ context.Context, alert *domain.Alert) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.sent = append(n.sent, *alert)
	return nil
}

// metadataServer serves SAML metadata holding one certificate whose expiry
// the test moves.
type metadataServer struct {
	mu       sync.Mutex
	serial   int64
	notAfter time.Time
}

func (s *metadataServer) set(notAfter time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.notAfter = notAfter
}

func (s *metadataServer) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	serial, notAfter := s.serial, s.notAfter
	s.mu.Unlock()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(serial),
		Subject:      pkix.Name{CommonName: "corp-idp"},
		NotBefore:    notAfter.Add(-365 * 24 * time.Hour),
		NotAfter:     notAfter,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_, _ = fmt.Fprintf(w, `<EntityDescriptor entityID="corp"><IDPSSODescriptor><KeyDescriptor use="signing"><KeyInfo><X509Data><X509Certificate>%s</X509Certificate></X509Data></KeyInfo></KeyDescriptor></IDPSSODescriptor></EntityDescriptor>`,
		base64.StdEncoding.EncodeToString(der))
}

type certTransition struct {
	svc      *service
	check    *IdentityProviderMetadataCheck
	store    *mockAlertStore
	notifier *recordingNotifier
	metadata *metadataServer
}

func newCertTransition(t *testing.T) *certTransition {
	t.Helper()

	metadata := &metadataServer{serial: 4242}
	srv := httptest.NewServer(metadata)
	t.Cleanup(srv.Close)

	kc := &testKeycloakClient{identityProviders: map[string][]*keycloakadmin.IdentityProviderRepresentation{
		"r1": {{Alias: "corp", ProviderID: "saml", InternalID: "idp-1", Config: map[string]string{"metadataDescriptorUrl": "http://metadata.example/idp"}}},
	}}
	log := logger.NewNoop()
	check := NewIdentityProviderMetadataCheck(kc, log, []string{"r1"}, "t1", 30, 5*time.Second, saferequest.Policy{})
	check.metadataFetcher = fetcherAgainst(srv)
	store := newMockAlertStore()
	notifier := &recordingNotifier{}
	svc := NewService([]Check{check}, store, notifier, log, time.Hour).(*service)
	return &certTransition{svc: svc, check: check, store: store, notifier: notifier, metadata: metadata}
}

func (c *certTransition) poll(t *testing.T) {
	t.Helper()
	if err := c.svc.executeCheck(context.Background(), c.check); err != nil {
		t.Fatalf("executeCheck: %v", err)
	}
}

func (c *certTransition) warningID() string {
	return c.check.generateCertAlertID("r1", "corp", "4242")
}

func (c *certTransition) expiredID() string {
	return c.check.generateExpiredCertAlertID("r1", "corp", "4242")
}

func TestCertificateExpiry_RaisesItsOwnAlert(t *testing.T) {
	for _, tc := range []struct {
		name        string
		warningLeft func(*domain.Alert)
	}{
		{name: "warning still open", warningLeft: func(*domain.Alert) {}},
		{name: "warning acknowledged", warningLeft: func(a *domain.Alert) {
			now := time.Now()
			a.Status, a.AcknowledgedAt, a.AcknowledgedBy = domain.AlertStatusAcknowled, &now, "operator"
		}},
		{name: "warning resolved", warningLeft: func(a *domain.Alert) {
			now := time.Now()
			a.Status, a.ResolvedAt = domain.AlertStatusResolved, &now
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := newCertTransition(t)

			c.metadata.set(time.Now().Add(10 * 24 * time.Hour))
			c.poll(t)
			warning := c.store.alerts[c.warningID()]
			if warning == nil || warning.Severity != domain.AlertSeverityWarning || len(c.notifier.sent) != 1 {
				t.Fatalf("expiring soon: alert %+v, notifications %d; want one warning, notified once", warning, len(c.notifier.sent))
			}
			tc.warningLeft(warning)

			c.metadata.set(time.Now().Add(-24 * time.Hour))
			c.poll(t)

			expired := c.store.alerts[c.expiredID()]
			if expired == nil {
				t.Fatal("no alert for the expired certificate")
			}
			if expired.Status != domain.AlertStatusActive || expired.Severity != domain.AlertSeverityCritical {
				t.Fatalf("expired alert status %s severity %s, want active critical", expired.Status, expired.Severity)
			}
			if !strings.Contains(expired.Title, "has expired") {
				t.Fatalf("expired alert title %q", expired.Title)
			}
			if len(c.notifier.sent) != 2 || c.notifier.sent[1].AlertID != c.expiredID() ||
				c.notifier.sent[1].Severity != domain.AlertSeverityCritical {
				t.Fatalf("notifications %+v, want a second, critical one for the expired alert", c.notifier.sent)
			}
			if got := c.store.alerts[c.warningID()].Status; got != domain.AlertStatusResolved {
				t.Fatalf("superseded warning status %s, want resolved", got)
			}

			c.poll(t)
			if len(c.notifier.sent) != 2 {
				t.Fatalf("a repeat poll within the hour notified again: %d notifications", len(c.notifier.sent))
			}
		})
	}
}

func TestCertificateExpiringSoon_DoesNotRepeat(t *testing.T) {
	c := newCertTransition(t)
	c.metadata.set(time.Now().Add(10 * 24 * time.Hour))
	for range 3 {
		c.poll(t)
	}
	if len(c.store.alerts) != 1 || len(c.notifier.sent) != 1 {
		t.Fatalf("after three polls: %d alerts, %d notifications; want 1 and 1", len(c.store.alerts), len(c.notifier.sent))
	}
	if c.store.alerts[c.warningID()].Status != domain.AlertStatusActive {
		t.Fatal("the expiring-soon warning is no longer active")
	}
}
