package crypto

import (
	"strings"
	"testing"
)

func TestDeriveKeyDeterministic(t *testing.T) {
	salt := []byte("0123456789abcdef0123456789abcdef")
	k1, _, err := DeriveKey("hunter2", salt)
	if err != nil {
		t.Fatalf("derive 1: %v", err)
	}
	k2, _, err := DeriveKey("hunter2", salt)
	if err != nil {
		t.Fatalf("derive 2: %v", err)
	}
	if string(k1) != string(k2) {
		t.Fatalf("PBKDF2 not deterministic with same salt")
	}
	if len(k1) != KeyLength {
		t.Fatalf("key length: got %d, want %d", len(k1), KeyLength)
	}
}

func TestDeriveKeyRandomSalt(t *testing.T) {
	_, s1, err := DeriveKey("hunter2", nil)
	if err != nil {
		t.Fatalf("derive 1: %v", err)
	}
	_, s2, err := DeriveKey("hunter2", nil)
	if err != nil {
		t.Fatalf("derive 2: %v", err)
	}
	if string(s1) == string(s2) {
		t.Fatalf("random salt repeated")
	}
	if len(s1) != SaltLength {
		t.Fatalf("salt length: got %d, want %d", len(s1), SaltLength)
	}
}

func TestEncryptDecryptRoundtrip(t *testing.T) {
	key, _, err := DeriveKey("hunter2", make([]byte, SaltLength))
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	svc, err := New(key)
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	cases := []string{
		"",
		"hello world",
		SentinelPlain,
		strings.Repeat("a", 4096),
	}
	for _, pt := range cases {
		env, err := svc.Encrypt(pt)
		if err != nil {
			t.Fatalf("encrypt %q: %v", pt, err)
		}
		if got := strings.Count(env, ":"); got != 2 {
			t.Fatalf("envelope shape %q: %d colons, want 2", env, got)
		}
		out, err := svc.Decrypt(env)
		if err != nil {
			t.Fatalf("decrypt %q: %v", pt, err)
		}
		if out != pt {
			t.Fatalf("roundtrip: got %q, want %q", out, pt)
		}
	}
}

func TestEncryptProducesFreshIV(t *testing.T) {
	key, _, _ := DeriveKey("hunter2", make([]byte, SaltLength))
	svc, _ := New(key)

	a, _ := svc.Encrypt("same plaintext")
	b, _ := svc.Encrypt("same plaintext")
	if a == b {
		t.Fatalf("two encrypts produced identical envelopes (IV reuse)")
	}
}

func TestDecryptWrongKeyFails(t *testing.T) {
	keyA, _, _ := DeriveKey("hunter2", make([]byte, SaltLength))
	keyB, _, _ := DeriveKey("hunter3", make([]byte, SaltLength))

	svcA, _ := New(keyA)
	svcB, _ := New(keyB)

	env, err := svcA.Encrypt("secret")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if _, err := svcB.Decrypt(env); err == nil {
		t.Fatalf("decrypt with wrong key should fail")
	}
}

func TestDecryptTamperedFails(t *testing.T) {
	key, _, _ := DeriveKey("hunter2", make([]byte, SaltLength))
	svc, _ := New(key)

	env, _ := svc.Encrypt("secret")
	// Flip a byte in the ciphertext portion.
	parts := strings.Split(env, ":")
	if len(parts[2]) < 2 {
		t.Fatalf("ciphertext too short to tamper")
	}
	tampered := parts[0] + ":" + parts[1] + ":" + flipFirstHex(parts[2])
	if _, err := svc.Decrypt(tampered); err == nil {
		t.Fatalf("decrypt with tampered ciphertext should fail")
	}
}

func TestDecryptMalformedEnvelope(t *testing.T) {
	key, _, _ := DeriveKey("hunter2", make([]byte, SaltLength))
	svc, _ := New(key)

	cases := []string{
		"",
		"not-an-envelope",
		"only:two",
		"zz:zz:zz",
	}
	for _, c := range cases {
		if _, err := svc.Decrypt(c); err == nil {
			t.Fatalf("decrypt %q: expected error, got nil", c)
		}
	}
}

func TestNewRejectsShortKey(t *testing.T) {
	if _, err := New([]byte("short")); err == nil {
		t.Fatalf("New should reject 5-byte key")
	}
}

func flipFirstHex(s string) string {
	if s == "" {
		return s
	}
	first := s[0]
	var swapped byte
	switch {
	case first == '0':
		swapped = '1'
	default:
		swapped = '0'
	}
	return string(swapped) + s[1:]
}
