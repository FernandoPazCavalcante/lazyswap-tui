// Package crypto implements AES-256-GCM encryption with PBKDF2-derived keys.
//
// Wire-compatible with the Bun/TS reference (src/common/crypto-service.ts):
//   - PBKDF2-HMAC-SHA256, 100_000 iterations, 32-byte key, 32-byte salt
//   - AES-256-GCM with 16-byte IV (non-standard nonce size) and 16-byte auth tag
//   - Ciphertext format: "iv_hex:authTag_hex:ciphertext_hex"
//
// IV length is 16 bytes (not the standard 12) to match the JS reference.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

const (
	KeyLength       = 32      // 256-bit AES key
	IVLength        = 16      // 128-bit nonce — matches JS implementation
	SaltLength      = 32      // 256-bit PBKDF2 salt
	AuthTagLength   = 16      // 128-bit GCM auth tag (Go default)
	PBKDF2Iter      = 100_000 // matches JS implementation
	SentinelPlain   = "lazyswap-v1-ok"
)

// Service holds the derived encryption key.
type Service struct {
	key []byte
}

// New creates a Service from an already-derived 32-byte key.
func New(key []byte) (*Service, error) {
	if len(key) != KeyLength {
		return nil, fmt.Errorf("key length: got %d, want %d", len(key), KeyLength)
	}
	return &Service{key: key}, nil
}

// DeriveKey runs PBKDF2-HMAC-SHA256 over the password. If salt is nil, a fresh
// random 32-byte salt is generated and returned alongside the derived key.
func DeriveKey(password string, salt []byte) (key, usedSalt []byte, err error) {
	if salt == nil {
		salt = make([]byte, SaltLength)
		if _, err := rand.Read(salt); err != nil {
			return nil, nil, err
		}
	}
	k, err := pbkdf2.Key(sha256.New, password, salt, PBKDF2Iter, KeyLength)
	if err != nil {
		return nil, nil, err
	}
	return k, salt, nil
}

// Encrypt seals plaintext under AES-256-GCM and returns the
// "iv_hex:authTag_hex:ciphertext_hex" envelope.
func (s *Service) Encrypt(plaintext string) (string, error) {
	iv := make([]byte, IVLength)
	if _, err := rand.Read(iv); err != nil {
		return "", err
	}

	block, err := aes.NewCipher(s.key)
	if err != nil {
		return "", err
	}
	aead, err := cipher.NewGCMWithNonceSize(block, IVLength)
	if err != nil {
		return "", err
	}

	sealed := aead.Seal(nil, iv, []byte(plaintext), nil)
	if len(sealed) < AuthTagLength {
		return "", errors.New("sealed output shorter than auth tag")
	}
	ciphertext := sealed[:len(sealed)-AuthTagLength]
	authTag := sealed[len(sealed)-AuthTagLength:]

	return fmt.Sprintf("%s:%s:%s",
		hex.EncodeToString(iv),
		hex.EncodeToString(authTag),
		hex.EncodeToString(ciphertext)), nil
}

// Decrypt opens an envelope produced by Encrypt. Returns an error if the
// auth tag fails to verify (wrong key or tampered ciphertext).
func (s *Service) Decrypt(envelope string) (string, error) {
	parts := strings.Split(envelope, ":")
	if len(parts) != 3 {
		return "", errors.New("invalid encrypted data format")
	}

	iv, err := hex.DecodeString(parts[0])
	if err != nil {
		return "", fmt.Errorf("decode iv: %w", err)
	}
	if len(iv) != IVLength {
		return "", fmt.Errorf("iv length: got %d, want %d", len(iv), IVLength)
	}
	authTag, err := hex.DecodeString(parts[1])
	if err != nil {
		return "", fmt.Errorf("decode authTag: %w", err)
	}
	if len(authTag) != AuthTagLength {
		return "", fmt.Errorf("authTag length: got %d, want %d", len(authTag), AuthTagLength)
	}
	ciphertext, err := hex.DecodeString(parts[2])
	if err != nil {
		return "", fmt.Errorf("decode ciphertext: %w", err)
	}

	block, err := aes.NewCipher(s.key)
	if err != nil {
		return "", err
	}
	aead, err := cipher.NewGCMWithNonceSize(block, IVLength)
	if err != nil {
		return "", err
	}

	// Go's GCM expects ciphertext||authTag.
	combined := make([]byte, 0, len(ciphertext)+len(authTag))
	combined = append(combined, ciphertext...)
	combined = append(combined, authTag...)

	plaintext, err := aead.Open(nil, iv, combined, nil)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}
