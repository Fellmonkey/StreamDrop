package security

import (
	"bytes"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	s := New()
	plain := []byte("password:app-specific-token:я")
	enc, err := s.Encrypt(plain)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if bytes.Contains(enc, []byte("password")) {
		t.Fatal("ciphertext contains plaintext")
	}
	dec, err := s.Decrypt(enc)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if !bytes.Equal(dec, plain) {
		t.Fatalf("round trip mismatch: %q", dec)
	}
}

func TestEmpty(t *testing.T) {
	s := New()
	if enc, err := s.Encrypt(nil); err != nil || enc != nil {
		t.Fatalf("empty encrypt = %v, %v", enc, err)
	}
	if dec, err := s.Decrypt(nil); err != nil || dec != nil {
		t.Fatalf("empty decrypt = %v, %v", dec, err)
	}
}

func TestDecryptTampered(t *testing.T) {
	s := New()
	enc, err := s.Encrypt([]byte("secret"))
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	enc[len(enc)-1] ^= 0xff
	if _, err := s.Decrypt(enc); err == nil {
		t.Fatal("expected tampered ciphertext to fail")
	}
}
