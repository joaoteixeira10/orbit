package keystore

import (
	"bytes"
	"testing"
)

func TestSealUnsealRoundTrip(t *testing.T) {
	s, err := New("master-passphrase", []byte("0123456789abcdef"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	plain := []byte("super secret material")
	blob, err := s.Seal(plain)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	if bytes.Contains(blob, plain) {
		t.Error("sealed blob must not contain plaintext")
	}
	got, err := s.Unseal(blob)
	if err != nil {
		t.Fatalf("Unseal: %v", err)
	}
	if !bytes.Equal(got, plain) {
		t.Errorf("round-trip mismatch: %q", got)
	}
}

func TestTamperDetected(t *testing.T) {
	s, _ := New("master-passphrase", []byte("0123456789abcdef"))
	blob, _ := s.Seal([]byte("data"))
	blob[len(blob)-1] ^= 0xff // flip a ciphertext bit
	if _, err := s.Unseal(blob); err != ErrCorrupt {
		t.Errorf("tampered blob: want ErrCorrupt, got %v", err)
	}
}

func TestWrongKeyFails(t *testing.T) {
	s1, _ := New("key-one", []byte("0123456789abcdef"))
	s2, _ := New("key-two", []byte("0123456789abcdef"))
	blob, _ := s1.Seal([]byte("data"))
	if _, err := s2.Unseal(blob); err != ErrCorrupt {
		t.Errorf("wrong master key: want ErrCorrupt, got %v", err)
	}
}

func TestShortBlob(t *testing.T) {
	s, _ := New("master", []byte("0123456789abcdef"))
	if _, err := s.Unseal([]byte{1, 2, 3}); err != ErrCorrupt {
		t.Errorf("short blob: want ErrCorrupt, got %v", err)
	}
}

func TestEmptyMasterKey(t *testing.T) {
	if _, err := New("", []byte("0123456789abcdef")); err == nil {
		t.Error("empty master key should error")
	}
}
