package notifications

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/config"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
)

// The checkers and the Keycloak monitor notify inline, so an email that
// never finishes stops their alerting. These tests run the notifier against a
// small SMTP server that misbehaves on purpose.

type smtpMode int

const (
	smtpOK            smtpMode = iota
	smtpSilent                 // accepts the connection, never greets
	smtpStallAtData            // stops answering once the message is sent
	smtpRejectMessage          // refuses the message after it is sent
)

type fakeSMTP struct {
	ln       net.Listener
	mode     smtpMode
	mu       sync.Mutex
	messages []string
}

func startFakeSMTP(t *testing.T, mode smtpMode, useTLS bool) *fakeSMTP {
	t.Helper()
	var ln net.Listener
	var err error
	if useTLS {
		ln, err = tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{selfSigned(t)}})
	} else {
		ln, err = net.Listen("tcp", "127.0.0.1:0")
	}
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	f := &fakeSMTP{ln: ln, mode: mode}
	done := make(chan struct{})
	t.Cleanup(func() { close(done); _ = ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go f.serve(conn, done)
		}
	}()
	return f
}

func (f *fakeSMTP) serve(conn net.Conn, done chan struct{}) {
	defer func() { _ = conn.Close() }()
	if f.mode == smtpSilent {
		<-done
		return
	}
	r := bufio.NewReader(conn)
	reply := func(s string) { _, _ = conn.Write([]byte(s + "\r\n")) }
	reply("220 fake ESMTP")
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		cmd := strings.ToUpper(strings.TrimSpace(line))
		switch {
		case strings.HasPrefix(cmd, "EHLO"), strings.HasPrefix(cmd, "HELO"):
			reply("250 fake")
		case strings.HasPrefix(cmd, "MAIL"), strings.HasPrefix(cmd, "RCPT"):
			reply("250 ok")
		case cmd == "DATA":
			reply("354 go ahead")
			var msg strings.Builder
			for {
				l, err := r.ReadString('\n')
				if err != nil {
					return
				}
				if l == ".\r\n" {
					break
				}
				msg.WriteString(l)
			}
			switch f.mode {
			case smtpStallAtData:
				<-done
				return
			case smtpRejectMessage:
				reply("554 message refused")
			default:
				f.mu.Lock()
				f.messages = append(f.messages, msg.String())
				f.mu.Unlock()
				reply("250 queued")
			}
		case cmd == "QUIT":
			reply("221 bye")
			return
		default:
			reply("250 ok")
		}
	}
}

func (f *fakeSMTP) received() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.messages...)
}

func selfSigned(t *testing.T) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "127.0.0.1"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("certificate: %v", err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

func notifierFor(t *testing.T, f *fakeSMTP, useTLS bool, timeout time.Duration) *EmailNotifier {
	t.Helper()
	host, port, _ := net.SplitHostPort(f.ln.Addr().String())
	p, _ := strconv.Atoi(port)
	n := NewEmailNotifier(&config.EmailConfig{
		Enabled: true, SMTPHost: host, SMTPPort: p, From: "kmt@example.com",
		To: []string{"ops@example.com"}, UseTLS: useTLS, SkipVerify: true,
	}, logger.NewNoop())
	n.timeout = timeout
	return n
}

func testAlert() *domain.Alert {
	return &domain.Alert{AlertID: "a1", Title: "SSL Not Required", Severity: domain.AlertSeverityCritical,
		ResourceName: "blue", Status: domain.AlertStatusActive}
}

func TestEmail_AServerThatNeverAnswersTimesOut(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mode   smtpMode
		useTLS bool
	}{
		{"silent after connect", smtpSilent, false},
		{"stalls after the message", smtpStallAtData, false},
		{"stalls after the message, TLS", smtpStallAtData, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := startFakeSMTP(t, tc.mode, tc.useTLS)
			n := notifierFor(t, f, tc.useTLS, 300*time.Millisecond)

			start := time.Now()
			_, err := n.Notify(context.Background(), testAlert())
			if err == nil {
				t.Fatal("the send reported success against a server that never answered")
			}
			if took := time.Since(start); took > 3*time.Second {
				t.Errorf("the send took %s, want about the 300ms timeout", took)
			}
			if !strings.Contains(err.Error(), "did not finish within") {
				t.Errorf("error does not say it timed out: %v", err)
			}
		})
	}
}

func TestEmail_ACancelledContextStopsTheSend(t *testing.T) {
	f := startFakeSMTP(t, smtpSilent, false)
	n := notifierFor(t, f, false, 30*time.Second)

	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(200*time.Millisecond, cancel)
	start := time.Now()
	_, err := n.Notify(ctx, testAlert())
	if err == nil || !strings.Contains(err.Error(), "cancelled") {
		t.Fatalf("want a cancellation error, got %v", err)
	}
	if took := time.Since(start); took > 3*time.Second {
		t.Errorf("the send took %s after cancellation", took)
	}
}

// The server's answer to the end of the message is its verdict on it. The
// TLS path used to discard it, so a refused email was reported as sent.
func TestEmail_ARefusedMessageIsAnError(t *testing.T) {
	for _, useTLS := range []bool{false, true} {
		f := startFakeSMTP(t, smtpRejectMessage, useTLS)
		n := notifierFor(t, f, useTLS, 5*time.Second)
		_, err := n.Notify(context.Background(), testAlert())
		if err == nil || !strings.Contains(err.Error(), "rejected the message") {
			t.Errorf("use_tls=%v: want a rejection error, got %v", useTLS, err)
		}
	}
}

func TestEmail_ANormalServerReceivesTheMessage(t *testing.T) {
	for _, useTLS := range []bool{false, true} {
		f := startFakeSMTP(t, smtpOK, useTLS)
		n := notifierFor(t, f, useTLS, 5*time.Second)
		if _, err := n.Notify(context.Background(), testAlert()); err != nil {
			t.Fatalf("use_tls=%v: send: %v", useTLS, err)
		}
		got := f.received()
		if len(got) != 1 || !strings.Contains(got[0], "Subject: [CRITICAL] Keycloak Monitoring Tool Alert: SSL Not Required - blue") {
			t.Errorf("use_tls=%v: server received %d message(s): %.200q", useTLS, len(got), got)
		}
	}
}
