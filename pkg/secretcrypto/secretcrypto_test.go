package secretcrypto

import (
	"crypto/rand"
	"encoding/base64"
	"strings"
	"testing"
)

func testKey(t *testing.T) string {
	t.Helper()
	key := make([]byte, keySize)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("failed to generate test key: %v", err)
	}
	return base64.StdEncoding.EncodeToString(key)
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	enc, err := New(testKey(t))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	const plaintext = "gIUckLL9ZzhQnd2uC7yODpU5uGYU0ce5"

	ciphertext, err := enc.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}
	if !strings.HasPrefix(ciphertext, EncryptedPrefix) {
		t.Fatalf("Encrypt() result missing prefix %q: %q", EncryptedPrefix, ciphertext)
	}
	if ciphertext == plaintext {
		t.Fatal("Encrypt() returned the plaintext unchanged")
	}

	decrypted, err := enc.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("Decrypt() error = %v", err)
	}
	if decrypted != plaintext {
		t.Fatalf("Decrypt() = %q, want %q", decrypted, plaintext)
	}
}

func TestEncryptIsNonDeterministic(t *testing.T) {
	enc, err := New(testKey(t))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	a, err := enc.Encrypt("same-secret")
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}
	b, err := enc.Encrypt("same-secret")
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}

	if a == b {
		t.Fatal("Encrypt() produced identical ciphertext for two calls, nonce reuse suspected")
	}
}

func TestEncryptEmptyString(t *testing.T) {
	enc, err := New(testKey(t))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	got, err := enc.Encrypt("")
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}
	if got != "" {
		t.Fatalf("Encrypt(\"\") = %q, want empty string", got)
	}
}

func TestDecryptPassesThroughPlaintext(t *testing.T) {
	enc, err := New(testKey(t))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	cases := []string{
		"gIUckLL9ZzhQnd2uC7yODpU5uGYU0ce5", // legacy plaintext secret
		"${KC_STAGING_CLIENT_SECRET}",      // unresolved config.yaml placeholder
		"",
	}

	for _, in := range cases {
		got, err := enc.Decrypt(in)
		if err != nil {
			t.Fatalf("Decrypt(%q) error = %v", in, err)
		}
		if got != in {
			t.Fatalf("Decrypt(%q) = %q, want unchanged", in, got)
		}
	}
}

func TestDecryptWrongKeyFails(t *testing.T) {
	encA, err := New(testKey(t))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	encB, err := New(testKey(t))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ciphertext, err := encA.Encrypt("secret-value")
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}

	if _, err := encB.Decrypt(ciphertext); err == nil {
		t.Fatal("Decrypt() with wrong key succeeded, want error")
	}
}

func TestDecryptMalformedCiphertext(t *testing.T) {
	enc, err := New(testKey(t))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	cases := []string{
		EncryptedPrefix + "not-valid-base64!!!",
		EncryptedPrefix + base64.StdEncoding.EncodeToString([]byte("short")),
	}

	for _, in := range cases {
		if _, err := enc.Decrypt(in); err == nil {
			t.Fatalf("Decrypt(%q) succeeded, want error", in)
		}
	}
}

func TestNewRejectsInvalidKeys(t *testing.T) {
	cases := map[string]string{
		"not base64":   "not-valid-base64!!!",
		"wrong length": base64.StdEncoding.EncodeToString([]byte("too-short")),
	}

	for name, key := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := New(key); err == nil {
				t.Fatalf("New(%q) succeeded, want error", key)
			}
		})
	}
}

func TestIsEncrypted(t *testing.T) {
	if IsEncrypted("plaintext") {
		t.Fatal("IsEncrypted(plaintext) = true, want false")
	}
	if !IsEncrypted(EncryptedPrefix + "abc") {
		t.Fatal("IsEncrypted(prefixed) = false, want true")
	}
}
