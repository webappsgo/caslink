package backup

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"golang.org/x/crypto/argon2"
)

// Argon2id parameters mirrored from src/server/service/password.go for
// consistency across the codebase (OWASP-recommended).
const (
	argon2Time    = 1
	argon2Memory  = 64 * 1024 // 64 MB
	argon2Threads = 4
	argon2KeyLen  = 32
	saltLen       = 16
)

// EncryptionMethod is the algorithm name recorded in manifest.json per
// AI.md PART 22.
const EncryptionMethod = "AES-256-GCM"

// deriveKey runs password through Argon2id with the given salt, producing a
// 256-bit AES key. The salt must be unique per backup and travels alongside
// the ciphertext — the password itself is NEVER stored.
func deriveKey(password string, salt []byte) []byte {
	return argon2.IDKey([]byte(password), salt, argon2Time, argon2Memory, argon2Threads, argon2KeyLen)
}

// encryptArchive encrypts plaintext with AES-256-GCM using an Argon2id key
// derived from password. Output framing: [salt(16)][nonce(12)][ciphertext+tag].
func encryptArchive(plaintext []byte, password string) ([]byte, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return nil, fmt.Errorf("backup encryption: salt generation failed: %w", err)
	}
	key := deriveKey(password, salt)

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("backup encryption: cipher init failed: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("backup encryption: gcm init failed: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("backup encryption: nonce generation failed: %w", err)
	}

	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)

	out := make([]byte, 0, saltLen+len(nonce)+len(ciphertext))
	out = append(out, salt...)
	out = append(out, nonce...)
	out = append(out, ciphertext...)
	return out, nil
}

// decryptArchive reverses encryptArchive. A wrong password surfaces as a GCM
// authentication failure rather than a distinguishable error, by AEAD design.
func decryptArchive(data []byte, password string) ([]byte, error) {
	if len(data) < saltLen+12 {
		return nil, fmt.Errorf("backup decryption: file too short to contain salt+nonce")
	}
	salt := data[:saltLen]
	key := deriveKey(password, salt)

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("backup decryption: cipher init failed: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("backup decryption: gcm init failed: %w", err)
	}
	nonceLen := gcm.NonceSize()
	if len(data) < saltLen+nonceLen {
		return nil, fmt.Errorf("backup decryption: file too short to contain nonce")
	}
	nonce := data[saltLen : saltLen+nonceLen]
	ciphertext := data[saltLen+nonceLen:]

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("backup decryption: authentication failed (wrong password or corrupt file): %w", err)
	}
	return plaintext, nil
}

// sha256Hex returns the lowercase hex SHA-256 digest of data.
func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
