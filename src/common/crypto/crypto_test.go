package crypto

import "testing"

func TestGenerateKeyDecodeKeyRoundTrip(t *testing.T) {
	encoded, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	key, err := DecodeKey(encoded)
	if err != nil {
		t.Fatalf("DecodeKey: %v", err)
	}
	if len(key) != KeySize {
		t.Fatalf("expected key length %d, got %d", KeySize, len(key))
	}
}

func TestDecodeKeyRejectsBadInput(t *testing.T) {
	if _, err := DecodeKey(""); err == nil {
		t.Fatal("expected error for empty key")
	}
	if _, err := DecodeKey("not-base64!!"); err == nil {
		t.Fatal("expected error for invalid base64")
	}
	shortKey, _ := GenerateKey()
	shortKey = shortKey[:10]
	if _, err := DecodeKey(shortKey); err == nil {
		t.Fatal("expected error for wrong-length key")
	}
}

func TestEncryptDecryptGCMRoundTrip(t *testing.T) {
	encoded, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	key, err := DecodeKey(encoded)
	if err != nil {
		t.Fatalf("DecodeKey: %v", err)
	}

	plaintext := []byte("JBSWY3DPEHPK3PXP")
	ciphertext, err := EncryptGCM(key, plaintext)
	if err != nil {
		t.Fatalf("EncryptGCM: %v", err)
	}
	if ciphertext == string(plaintext) {
		t.Fatal("ciphertext must not equal plaintext")
	}

	decrypted, err := DecryptGCM(key, ciphertext)
	if err != nil {
		t.Fatalf("DecryptGCM: %v", err)
	}
	if string(decrypted) != string(plaintext) {
		t.Fatalf("decrypted mismatch: got %q, want %q", decrypted, plaintext)
	}
}

func TestDecryptGCMWrongKeyFails(t *testing.T) {
	encodedA, _ := GenerateKey()
	keyA, _ := DecodeKey(encodedA)
	encodedB, _ := GenerateKey()
	keyB, _ := DecodeKey(encodedB)

	ciphertext, err := EncryptGCM(keyA, []byte("secret"))
	if err != nil {
		t.Fatalf("EncryptGCM: %v", err)
	}
	if _, err := DecryptGCM(keyB, ciphertext); err == nil {
		t.Fatal("expected decryption to fail with wrong key")
	}
}

func TestEncryptGCMRejectsBadKeyLength(t *testing.T) {
	if _, err := EncryptGCM([]byte("short"), []byte("data")); err == nil {
		t.Fatal("expected error for short key")
	}
}

func TestDecryptGCMRejectsTamperedCiphertext(t *testing.T) {
	encoded, _ := GenerateKey()
	key, _ := DecodeKey(encoded)
	ciphertext, err := EncryptGCM(key, []byte("secret"))
	if err != nil {
		t.Fatalf("EncryptGCM: %v", err)
	}
	tampered := ciphertext[:len(ciphertext)-4] + "abcd"
	if _, err := DecryptGCM(key, tampered); err == nil {
		t.Fatal("expected error for tampered ciphertext")
	}
}
