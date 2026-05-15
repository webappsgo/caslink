package service

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// hashPasswordArgon2id hashes a password using Argon2id (SPEC requirement line 129)
// NEVER use bcrypt - always use Argon2id.
// Returns an error if the CSPRNG salt cannot be generated; callers must
// refuse to persist a hash in that case rather than silently substituting
// a weak one.
func hashPasswordArgon2id(password string) (string, error) {
	// Argon2id parameters (OWASP recommendations)
	const (
		time    = 1
		memory  = 64 * 1024 // 64 MB
		threads = 4
		keyLen  = 32
	)

	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("argon2id: salt generation failed: %w", err)
	}

	// Hash password
	hash := argon2.IDKey([]byte(password), salt, time, memory, threads, keyLen)

	// Encode as base64
	b64Salt := base64.RawStdEncoding.EncodeToString(salt)
	b64Hash := base64.RawStdEncoding.EncodeToString(hash)

	// Format: $argon2id$v=19$m=65536,t=1,p=4$salt$hash
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		memory, time, threads, b64Salt, b64Hash), nil
}

// verifyPasswordArgon2id verifies a password against an Argon2id hash
func verifyPasswordArgon2id(password, hash string) bool {
	// Parse hash format
	parts := strings.Split(hash, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false
	}

	// Parse parameters
	var memory, time uint32
	var threads uint8
	fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &time, &threads)

	// Decode salt and hash
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false
	}

	expectedHash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false
	}

	// Hash provided password with same parameters
	actualHash := argon2.IDKey([]byte(password), salt, time, memory, threads, uint32(len(expectedHash)))

	// Constant-time comparison via the stdlib (ConstantTimeCompare returns
	// 0 immediately on length mismatch, so we don't leak length either).
	return subtle.ConstantTimeCompare(actualHash, expectedHash) == 1
}
