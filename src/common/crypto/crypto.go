// Package crypto provides AES-256-GCM at-rest encryption helpers built on
// server.security.encryption_key per AI.md PART 11 ("Cryptographic Keys").
// This is the single canonical at-rest AES key: every place the spec talks
// about "encrypt this sensitive data at rest" (2FA secrets, security report
// bodies as the PGP fallback, etc.) resolves to this key.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
)

// KeySize is the required length in bytes of server.security.encryption_key
// (32 bytes = AES-256).
const KeySize = 32

// GenerateKey returns a new random 32-byte AES-256 key, base64-encoded for
// storage in server.yml. Called on first run when encryption_key is empty.
func GenerateKey() (string, error) {
	key := make([]byte, KeySize)
	if _, err := rand.Read(key); err != nil {
		return "", fmt.Errorf("generate encryption key: %w", err)
	}
	return base64.StdEncoding.EncodeToString(key), nil
}

// DecodeKey base64-decodes a stored encryption_key and validates its length.
func DecodeKey(encoded string) ([]byte, error) {
	if encoded == "" {
		return nil, errors.New("encryption key is empty")
	}
	key, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("decode encryption key: %w", err)
	}
	if len(key) != KeySize {
		return nil, fmt.Errorf("encryption key must be %d bytes, got %d", KeySize, len(key))
	}
	return key, nil
}

// EncryptGCM encrypts plaintext with AES-256-GCM under key, returning a
// base64-encoded string of nonce||ciphertext||tag.
func EncryptGCM(key []byte, plaintext []byte) (string, error) {
	if len(key) != KeySize {
		return "", fmt.Errorf("encryption key must be %d bytes, got %d", KeySize, len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("aes cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("gcm: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}
	sealed := gcm.Seal(nonce, nonce, plaintext, nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

// DecryptGCM reverses EncryptGCM: decodes the base64 payload and decrypts it
// with AES-256-GCM under key. Returns an error on tampering or a wrong key.
func DecryptGCM(key []byte, encoded string) ([]byte, error) {
	if len(key) != KeySize {
		return nil, fmt.Errorf("encryption key must be %d bytes, got %d", KeySize, len(key))
	}
	sealed, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("decode ciphertext: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("aes cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("gcm: %w", err)
	}
	nonceSize := gcm.NonceSize()
	if len(sealed) < nonceSize {
		return nil, errors.New("ciphertext too short")
	}
	nonce, body := sealed[:nonceSize], sealed[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, body, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt: %w", err)
	}
	return plaintext, nil
}
