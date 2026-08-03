package crypto

import (
	"encoding/base64"
	"testing"
)

func testKey(t *testing.T, seed byte) [KeySize]byte {
	t.Helper()
	var key [KeySize]byte
	for i := range key {
		key[i] = seed + byte(i)
	}
	return key
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	key := testKey(t, 0)
	plaintext := []byte("a very secret refresh token")

	ciphertext, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("Encrypt returned an error: %v", err)
	}
	if ciphertext == string(plaintext) {
		t.Fatalf("ciphertext matches plaintext - not actually encrypted")
	}

	got, err := Decrypt(ciphertext, key)
	if err != nil {
		t.Fatalf("Decrypt returned an error: %v", err)
	}
	if string(got) != string(plaintext) {
		t.Fatalf("round-tripped plaintext mismatch: got %q, want %q", got, plaintext)
	}
}

func TestEncryptIsRandomized(t *testing.T) {
	key := testKey(t, 0)
	plaintext := []byte("same input, twice")

	first, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("Encrypt returned an error: %v", err)
	}
	second, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("Encrypt returned an error: %v", err)
	}
	if first == second {
		t.Fatalf("two encryptions of the same plaintext produced identical ciphertext - nonce isn't being randomized")
	}
}

func TestDecryptWrongKeyFails(t *testing.T) {
	ciphertext, err := Encrypt([]byte("secret"), testKey(t, 0))
	if err != nil {
		t.Fatalf("Encrypt returned an error: %v", err)
	}
	if _, err := Decrypt(ciphertext, testKey(t, 1)); err == nil {
		t.Fatalf("expected Decrypt with the wrong key to fail, got nil error")
	}
}

func TestDecryptTamperedCiphertextFails(t *testing.T) {
	key := testKey(t, 0)
	ciphertext, err := Encrypt([]byte("secret"), key)
	if err != nil {
		t.Fatalf("Encrypt returned an error: %v", err)
	}
	raw, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		t.Fatalf("error decoding ciphertext: %v", err)
	}
	// Flip a bit somewhere in the middle of the ciphertext (past the nonce).
	raw[len(raw)-1] ^= 0xFF
	tampered := base64.StdEncoding.EncodeToString(raw)

	if _, err := Decrypt(tampered, key); err == nil {
		t.Fatalf("expected Decrypt to reject tampered ciphertext, got nil error")
	}
}

func TestDecryptTooShortFails(t *testing.T) {
	key := testKey(t, 0)
	tooShort := base64.StdEncoding.EncodeToString([]byte("short"))
	if _, err := Decrypt(tooShort, key); err != ErrCiphertextTooShort {
		t.Fatalf("expected ErrCiphertextTooShort, got %v", err)
	}
}

func TestParseKey(t *testing.T) {
	valid := base64.StdEncoding.EncodeToString(make([]byte, KeySize))
	key, err := ParseKey(valid)
	if err != nil {
		t.Fatalf("ParseKey returned an error for a valid key: %v", err)
	}
	if len(key) != KeySize {
		t.Fatalf("unexpected key length: got %d, want %d", len(key), KeySize)
	}
}

func TestParseKeyWrongSize(t *testing.T) {
	tooShort := base64.StdEncoding.EncodeToString(make([]byte, 16))
	if _, err := ParseKey(tooShort); err != ErrInvalidKeySize {
		t.Fatalf("expected ErrInvalidKeySize, got %v", err)
	}
}

func TestParseKeyInvalidBase64(t *testing.T) {
	if _, err := ParseKey("not valid base64!!!"); err == nil {
		t.Fatalf("expected an error for invalid base64 input")
	}
}
