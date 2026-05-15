package service

import (
	"testing"
	"time"
)

func TestArgon2idRoundTrip(t *testing.T) {
	const password = "correct-horse-battery-staple"

	hash, err := hashPasswordArgon2id(password)
	if err != nil {
		t.Fatalf("hashPasswordArgon2id returned error: %v", err)
	}

	if hash == "" {
		t.Fatal("expected non-empty hash")
	}

	// Correct password verifies
	if !verifyPasswordArgon2id(password, hash) {
		t.Error("verifyPasswordArgon2id returned false for correct password")
	}

	// Wrong password fails
	if verifyPasswordArgon2id("wrong-password", hash) {
		t.Error("verifyPasswordArgon2id returned true for wrong password")
	}

	// Empty password fails
	if verifyPasswordArgon2id("", hash) {
		t.Error("verifyPasswordArgon2id returned true for empty password")
	}
}

func TestPasswordTiming(t *testing.T) {
	const password = "timing-test-password-123"

	start := time.Now()
	hash, err := hashPasswordArgon2id(password)
	if err != nil {
		t.Fatalf("hashPasswordArgon2id returned error: %v", err)
	}
	hashDuration := time.Since(start)

	start = time.Now()
	verifyPasswordArgon2id(password, hash)
	verifyDuration := time.Since(start)

	// Argon2id with 64 MB memory must take meaningfully longer than a fast
	// hash. 20 ms is a conservative floor that passes even on minimal CI
	// containers while still ruling out MD5/SHA timing.
	const minDuration = 20 * time.Millisecond

	if hashDuration < minDuration {
		t.Errorf("hash took %v, want >= %v (Argon2id must be slow)", hashDuration, minDuration)
	}

	if verifyDuration < minDuration {
		t.Errorf("verify took %v, want >= %v (Argon2id must be slow)", verifyDuration, minDuration)
	}
}
