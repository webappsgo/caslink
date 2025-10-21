package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/argon2"
)

const (
	// DefaultArgon2Time is the default time parameter for Argon2
	DefaultArgon2Time = 1
	// DefaultArgon2Memory is the default memory parameter for Argon2 (64MB)
	DefaultArgon2Memory = 64 * 1024
	// DefaultArgon2Threads is the default threads parameter for Argon2
	DefaultArgon2Threads = 4
	// DefaultArgon2KeyLength is the default key length for Argon2
	DefaultArgon2KeyLength = 32
	// DefaultSaltLength is the default salt length
	DefaultSaltLength = 16
)

// PasswordHasher handles password hashing and verification using Argon2id
type PasswordHasher struct {
	time    uint32
	memory  uint32
	threads uint8
	keyLen  uint32
	saltLen uint32
}

// NewPasswordHasher creates a new password hasher with default or custom parameters
func NewPasswordHasher(cost int) *PasswordHasher {
	// Map cost to Argon2 parameters
	var time, memory uint32
	var threads uint8

	switch {
	case cost <= 6:
		time = 1
		memory = 32 * 1024 // 32MB
		threads = 2
	case cost <= 8:
		time = 1
		memory = 64 * 1024 // 64MB
		threads = 4
	case cost <= 10:
		time = 2
		memory = 64 * 1024 // 64MB
		threads = 4
	case cost <= 12:
		time = 3
		memory = 128 * 1024 // 128MB
		threads = 4
	default:
		time = 4
		memory = 256 * 1024 // 256MB
		threads = 8
	}

	return &PasswordHasher{
		time:    time,
		memory:  memory,
		threads: threads,
		keyLen:  DefaultArgon2KeyLength,
		saltLen: DefaultSaltLength,
	}
}

// HashPassword hashes a password using Argon2id
func (ph *PasswordHasher) HashPassword(password string) (string, error) {
	// Generate a random salt
	salt := make([]byte, ph.saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("failed to generate salt: %w", err)
	}

	// Hash the password
	key := argon2.IDKey([]byte(password), salt, ph.time, ph.memory, ph.threads, ph.keyLen)

	// Encode the result
	encodedSalt := base64.RawStdEncoding.EncodeToString(salt)
	encodedKey := base64.RawStdEncoding.EncodeToString(key)

	// Format: $argon2id$v=19$m=memory,t=time,p=threads$salt$hash
	hash := fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		ph.memory, ph.time, ph.threads, encodedSalt, encodedKey)

	return hash, nil
}

// VerifyPassword verifies a password against a hash
func (ph *PasswordHasher) VerifyPassword(password, hash string) bool {
	// Parse the hash
	params, salt, key, err := ph.parseHash(hash)
	if err != nil {
		return false
	}

	// Hash the password with the same parameters
	computedKey := argon2.IDKey([]byte(password), salt, params.time, params.memory, params.threads, uint32(len(key)))

	// Compare using constant-time comparison
	return subtle.ConstantTimeCompare(key, computedKey) == 1
}

// hashParams represents the parameters extracted from a hash
type hashParams struct {
	memory  uint32
	time    uint32
	threads uint8
}

// parseHash parses an Argon2id hash string and extracts parameters, salt, and key
func (ph *PasswordHasher) parseHash(hash string) (*hashParams, []byte, []byte, error) {
	// Expected format: $argon2id$v=19$m=memory,t=time,p=threads$salt$hash
	parts := strings.Split(hash, "$")
	if len(parts) != 6 {
		return nil, nil, nil, fmt.Errorf("invalid hash format")
	}

	if parts[1] != "argon2id" {
		return nil, nil, nil, fmt.Errorf("unsupported hash algorithm: %s", parts[1])
	}

	if parts[2] != "v=19" {
		return nil, nil, nil, fmt.Errorf("unsupported version: %s", parts[2])
	}

	// Parse parameters
	params, err := ph.parseParams(parts[3])
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to parse parameters: %w", err)
	}

	// Decode salt
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to decode salt: %w", err)
	}

	// Decode key
	key, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to decode key: %w", err)
	}

	return params, salt, key, nil
}

// parseParams parses the parameter string (e.g., "m=65536,t=1,p=4")
func (ph *PasswordHasher) parseParams(paramStr string) (*hashParams, error) {
	params := &hashParams{}

	for _, param := range strings.Split(paramStr, ",") {
		kv := strings.Split(param, "=")
		if len(kv) != 2 {
			return nil, fmt.Errorf("invalid parameter format: %s", param)
		}

		key, value := kv[0], kv[1]

		switch key {
		case "m":
			if _, err := fmt.Sscanf(value, "%d", &params.memory); err != nil {
				return nil, fmt.Errorf("invalid memory parameter: %s", value)
			}
		case "t":
			if _, err := fmt.Sscanf(value, "%d", &params.time); err != nil {
				return nil, fmt.Errorf("invalid time parameter: %s", value)
			}
		case "p":
			var threads int
			if _, err := fmt.Sscanf(value, "%d", &threads); err != nil {
				return nil, fmt.Errorf("invalid threads parameter: %s", value)
			}
			params.threads = uint8(threads)
		default:
			return nil, fmt.Errorf("unknown parameter: %s", key)
		}
	}

	return params, nil
}

// ValidatePasswordStrength validates password strength according to configured rules
func (ph *PasswordHasher) ValidatePasswordStrength(password string, minLength int, requireSpecial bool) error {
	if len(password) < minLength {
		return fmt.Errorf("password must be at least %d characters long", minLength)
	}

	if requireSpecial {
		hasUpper := false
		hasLower := false
		hasDigit := false
		hasSpecial := false

		for _, char := range password {
			switch {
			case char >= 'A' && char <= 'Z':
				hasUpper = true
			case char >= 'a' && char <= 'z':
				hasLower = true
			case char >= '0' && char <= '9':
				hasDigit = true
			case isSpecialChar(char):
				hasSpecial = true
			}
		}

		if !hasUpper {
			return fmt.Errorf("password must contain at least one uppercase letter")
		}
		if !hasLower {
			return fmt.Errorf("password must contain at least one lowercase letter")
		}
		if !hasDigit {
			return fmt.Errorf("password must contain at least one digit")
		}
		if !hasSpecial {
			return fmt.Errorf("password must contain at least one special character")
		}
	}

	return nil
}

// isSpecialChar checks if a character is a special character
func isSpecialChar(char rune) bool {
	specialChars := "!@#$%^&*()_+-=[]{}|;:,.<>?"
	for _, special := range specialChars {
		if char == special {
			return true
		}
	}
	return false
}

// GenerateRandomPassword generates a cryptographically secure random password
func GenerateRandomPassword(length int) (string, error) {
	if length < 4 {
		return "", fmt.Errorf("password length must be at least 4 characters")
	}

	const (
		lowerChars   = "abcdefghijklmnopqrstuvwxyz"
		upperChars   = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
		digitChars   = "0123456789"
		specialChars = "!@#$%^&*()_+-=[]{}|;:,.<>?"
		allChars     = lowerChars + upperChars + digitChars + specialChars
	)

	password := make([]byte, length)

	// Ensure at least one character from each category
	categories := []string{lowerChars, upperChars, digitChars, specialChars}
	for i, category := range categories {
		if i < length {
			randomIndex, err := secureRandomInt(len(category))
			if err != nil {
				return "", err
			}
			password[i] = category[randomIndex]
		}
	}

	// Fill the rest with random characters from all categories
	for i := len(categories); i < length; i++ {
		randomIndex, err := secureRandomInt(len(allChars))
		if err != nil {
			return "", err
		}
		password[i] = allChars[randomIndex]
	}

	// Shuffle the password to avoid predictable patterns
	for i := length - 1; i > 0; i-- {
		j, err := secureRandomInt(i + 1)
		if err != nil {
			return "", err
		}
		password[i], password[j] = password[j], password[i]
	}

	return string(password), nil
}

// secureRandomInt generates a cryptographically secure random integer in [0, max)
func secureRandomInt(max int) (int, error) {
	if max <= 0 {
		return 0, fmt.Errorf("max must be positive")
	}

	// Calculate the number of bytes needed
	bytes := make([]byte, 4)
	if _, err := rand.Read(bytes); err != nil {
		return 0, err
	}

	// Convert bytes to int and apply modulo
	num := int(bytes[0])<<24 | int(bytes[1])<<16 | int(bytes[2])<<8 | int(bytes[3])
	if num < 0 {
		num = -num
	}

	return num % max, nil
}

// EstimateHashTime estimates the time it takes to hash a password with current parameters
func (ph *PasswordHasher) EstimateHashTime(password string) (time.Duration, error) {
	start := time.Now()
	_, err := ph.HashPassword(password)
	if err != nil {
		return 0, err
	}
	return time.Since(start), nil
}

// GetHashParams returns the current hashing parameters
func (ph *PasswordHasher) GetHashParams() map[string]interface{} {
	return map[string]interface{}{
		"algorithm": "argon2id",
		"version":   19,
		"time":      ph.time,
		"memory":    ph.memory,
		"threads":   ph.threads,
		"keyLength": ph.keyLen,
		"saltLength": ph.saltLen,
	}
}