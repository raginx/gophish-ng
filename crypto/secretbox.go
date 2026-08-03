// Package crypto provides symmetric encryption for secrets that need to be
// stored at rest, such as OAuth2 refresh tokens. Everything else Gophish
// currently stores is plaintext,
// relying on DB-level access control
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
)

// KeySize is the required length, in bytes, of keys used by Encrypt,
// Decrypt, and ParseKey.
const KeySize = 32

// ErrInvalidKeySize is returned by ParseKey when the decoded key isn't
// exactly KeySize bytes long.
var ErrInvalidKeySize = errors.New("crypto: key must be 32 bytes")

// ErrCiphertextTooShort is returned by Decrypt when the input is too short
// to contain a nonce, i.e. it wasn't produced by Encrypt.
var ErrCiphertextTooShort = errors.New("crypto: ciphertext too short")

// ParseKey base64-decodes s (as produced by, e.g., `openssl rand -base64
// 32`) into a key suitable for Encrypt/Decrypt, validating its length.
func ParseKey(s string) ([KeySize]byte, error) {
	var key [KeySize]byte
	raw, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return key, err
	}
	if len(raw) != KeySize {
		return key, ErrInvalidKeySize
	}
	copy(key[:], raw)
	return key, nil
}

// Encrypt encrypts plaintext with AES-256-GCM using the given key, returning
// a base64-encoded string suitable for storing in a text column. A random
// nonce is generated per call and prepended to the ciphertext.
func Encrypt(plaintext []byte, key [KeySize]byte) (string, error) {
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, plaintext, nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

// Decrypt reverses Encrypt, returning an error if the key is wrong or the
// ciphertext has been truncated or tampered with.
func Decrypt(ciphertext string, key [KeySize]byte) ([]byte, error) {
	raw, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(raw) < gcm.NonceSize() {
		return nil, ErrCiphertextTooShort
	}
	nonce, sealed := raw[:gcm.NonceSize()], raw[gcm.NonceSize():]
	return gcm.Open(nil, nonce, sealed, nil)
}
