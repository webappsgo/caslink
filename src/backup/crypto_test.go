package backup

import (
	"bytes"
	"testing"
)

// TestEncryptDecryptRoundTrip covers the core AES-256-GCM/Argon2id round
// trip: what encryptArchive produces, decryptArchive must reverse exactly.
func TestEncryptDecryptRoundTrip(t *testing.T) {
	plaintext := []byte("the quick brown fox jumps over the lazy dog")
	password := "s3cr3t-passphrase"

	ciphertext, err := encryptArchive(plaintext, password)
	if err != nil {
		t.Fatalf("encryptArchive failed: %v", err)
	}
	if bytes.Equal(ciphertext, plaintext) {
		t.Fatal("ciphertext must not equal plaintext")
	}

	got, err := decryptArchive(ciphertext, password)
	if err != nil {
		t.Fatalf("decryptArchive failed: %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatalf("round trip mismatch: got %q, want %q", got, plaintext)
	}
}

// TestEncryptArchiveNondeterministic asserts salt/nonce are generated fresh
// per call: encrypting the same plaintext+password twice must not produce
// identical ciphertext (otherwise the salt/nonce generation would be broken
// and backups would be vulnerable to key/nonce reuse).
func TestEncryptArchiveNondeterministic(t *testing.T) {
	plaintext := []byte("identical input, must not repeat framing")
	password := "same-password"

	a, err := encryptArchive(plaintext, password)
	if err != nil {
		t.Fatalf("encryptArchive #1 failed: %v", err)
	}
	b, err := encryptArchive(plaintext, password)
	if err != nil {
		t.Fatalf("encryptArchive #2 failed: %v", err)
	}
	if bytes.Equal(a, b) {
		t.Fatal("expected two encryptions of the same plaintext to differ (fresh salt/nonce)")
	}

	// Both must still independently decrypt back to the same plaintext.
	for i, ct := range [][]byte{a, b} {
		got, err := decryptArchive(ct, password)
		if err != nil {
			t.Fatalf("decryptArchive #%d failed: %v", i, err)
		}
		if !bytes.Equal(got, plaintext) {
			t.Fatalf("decryptArchive #%d mismatch: got %q", i, got)
		}
	}
}

// TestDecryptArchiveWrongPasswordFails covers the AEAD-auth-failure path: a
// wrong password must surface as an error, never as silently wrong output.
func TestDecryptArchiveWrongPasswordFails(t *testing.T) {
	ciphertext, err := encryptArchive([]byte("payload"), "right-password")
	if err != nil {
		t.Fatalf("encryptArchive failed: %v", err)
	}
	if _, err := decryptArchive(ciphertext, "wrong-password"); err == nil {
		t.Fatal("expected decryptArchive to fail with a wrong password")
	}
}

// TestDecryptArchiveTooShort covers both length guards in decryptArchive:
// data shorter than salt+nonce, and data shorter than salt+nonce+ciphertext.
func TestDecryptArchiveTooShort(t *testing.T) {
	cases := map[string][]byte{
		"empty":               {},
		"shorter than salt":   make([]byte, saltLen-1),
		"salt only, no nonce": make([]byte, saltLen),
		"salt+partial nonce":  make([]byte, saltLen+5),
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := decryptArchive(data, "any-password"); err == nil {
				t.Fatalf("expected decryptArchive to fail for %s (%d bytes)", name, len(data))
			}
		})
	}
}

// TestDeriveKeyDeterministic covers deriveKey's contract: same password+salt
// must always yield the same key (required for decrypt to ever succeed).
func TestDeriveKeyDeterministic(t *testing.T) {
	salt := bytes.Repeat([]byte{0x42}, saltLen)
	k1 := deriveKey("password", salt)
	k2 := deriveKey("password", salt)
	if !bytes.Equal(k1, k2) {
		t.Fatal("expected deriveKey to be deterministic for the same password+salt")
	}
	if len(k1) != argon2KeyLen {
		t.Fatalf("expected a %d-byte key, got %d", argon2KeyLen, len(k1))
	}
}

// TestDeriveKeyDiffersBySaltAndPassword covers deriveKey's uniqueness
// contract: changing either input must change the derived key.
func TestDeriveKeyDiffersBySaltAndPassword(t *testing.T) {
	saltA := bytes.Repeat([]byte{0x01}, saltLen)
	saltB := bytes.Repeat([]byte{0x02}, saltLen)

	base := deriveKey("password-a", saltA)
	diffSalt := deriveKey("password-a", saltB)
	diffPassword := deriveKey("password-b", saltA)

	if bytes.Equal(base, diffSalt) {
		t.Fatal("expected different salts to produce different keys")
	}
	if bytes.Equal(base, diffPassword) {
		t.Fatal("expected different passwords to produce different keys")
	}
}

// TestSha256HexKnownVector pins sha256Hex against a known SHA-256 digest so
// a future refactor cannot silently swap in the wrong hash algorithm.
func TestSha256HexKnownVector(t *testing.T) {
	got := sha256Hex([]byte("abc"))
	// The well-known SHA-256("abc") test vector.
	want := "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
	if got != want {
		t.Fatalf("sha256Hex(%q) = %q, want %q", "abc", got, want)
	}
}
