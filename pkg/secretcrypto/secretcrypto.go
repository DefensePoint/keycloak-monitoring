// Package secretcrypto encrypts secrets (e.g. tenant client_secret) at rest
// using AES-256-GCM, so a database dump alone doesn't hand over live
// Keycloak credentials. Values are still needed in plaintext to actually
// authenticate with Keycloak, so this is encryption (reversible), not
// hashing (one-way).
package secretcrypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"
)

// EncryptedPrefix marks a stored value as ciphertext produced by this
// package, so Decrypt can tell it apart from legacy plaintext (existing rows
// written before encryption was introduced, or a config.yaml value that was
// never a real secret to begin with) without needing to guess.
const EncryptedPrefix = "enc:v1:"

const keySize = 32 // AES-256

// Encryptor encrypts and decrypts secrets with a single AES-256-GCM key.
type Encryptor struct {
	gcm cipher.AEAD
}

// New builds an Encryptor from a base64-encoded 32-byte key, as produced by
// e.g. `openssl rand -base64 32`.
func New(base64Key string) (*Encryptor, error) {
	key, err := base64.StdEncoding.DecodeString(base64Key)
	if err != nil {
		return nil, fmt.Errorf("encryption key is not valid base64: %w", err)
	}
	if len(key) != keySize {
		return nil, fmt.Errorf("encryption key must decode to %d bytes, got %d", keySize, len(key))
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM mode: %w", err)
	}

	return &Encryptor{gcm: gcm}, nil
}

// Encrypt returns plaintext encrypted and encoded as "enc:v1:<base64>".
// Empty input is returned unchanged (nothing to protect, and round-tripping
// "" through AES-GCM would just add a meaningless nonce+tag).
func (e *Encryptor) Encrypt(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}

	nonce := make([]byte, e.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}

	ciphertext := e.gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return EncryptedPrefix + base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt reverses Encrypt. A value without the EncryptedPrefix is assumed to
// be legacy plaintext (or a config.yaml literal that was never encrypted)
// and is returned unchanged rather than treated as an error, so the app
// keeps working during the window before existing rows are migrated.
func (e *Encryptor) Decrypt(stored string) (string, error) {
	if !strings.HasPrefix(stored, EncryptedPrefix) {
		return stored, nil
	}

	encoded := strings.TrimPrefix(stored, EncryptedPrefix)
	ciphertext, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("stored secret is not valid base64: %w", err)
	}

	nonceSize := e.gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return "", errors.New("stored secret is too short to contain a nonce")
	}

	nonce, sealed := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := e.gcm.Open(nil, nonce, sealed, nil)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt secret: %w", err)
	}

	return string(plaintext), nil
}

// IsEncrypted reports whether stored was produced by Encrypt.
func IsEncrypted(stored string) bool {
	return strings.HasPrefix(stored, EncryptedPrefix)
}
