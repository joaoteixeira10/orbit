// Package keystore seals private keys at rest with AES-256-GCM under a master
// key (KEK). The 32-byte AES key is derived from ORBIT_MASTER_KEY with Argon2id
// over a stored per-install salt, so a weaker/human master key still resists
// offline brute force. Legacy installs used a bare SHA-256; New() is migrated to
// Argon2id on first run (see db.MigrateKEK), and NewLegacy reads pre-Argon2 data.
//
// Sealed blob layout: [12-byte nonce][GCM ciphertext+tag]. GCM authenticates
// the ciphertext, so tampering is detected on Unseal.
package keystore

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"io"

	"golang.org/x/crypto/argon2"
)

// Argon2id parameters (one pass, 64 MiB, 4 lanes → ~tens of ms at startup).
const (
	argonTime    = 1
	argonMemory  = 64 * 1024
	argonThreads = 4
	argonKeyLen  = 32
)

// ErrCorrupt is returned when a sealed blob fails authentication.
var ErrCorrupt = errors.New("keystore: sealed data is corrupt or wrong key")

// Sealer seals and unseals secrets.
type Sealer struct {
	aead cipher.AEAD
}

// New derives the AES-256 key from masterKey + salt via Argon2id.
func New(masterKey string, salt []byte) (*Sealer, error) {
	if masterKey == "" {
		return nil, errors.New("keystore: empty master key")
	}
	if len(salt) == 0 {
		return nil, errors.New("keystore: empty salt")
	}
	key := argon2.IDKey([]byte(masterKey), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return newFromKey(key)
}

// NewLegacy derives the AES-256 key from a bare SHA-256 of masterKey — the
// original scheme, retained only so MigrateKEK can read pre-Argon2 data.
func NewLegacy(masterKey string) (*Sealer, error) {
	if masterKey == "" {
		return nil, errors.New("keystore: empty master key")
	}
	sum := sha256.Sum256([]byte(masterKey))
	return newFromKey(sum[:])
}

// RandomSalt returns n bytes of CSPRNG, for use as an Argon2id salt.
func RandomSalt(n int) ([]byte, error) {
	salt := make([]byte, n)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, err
	}
	return salt, nil
}

func newFromKey(key []byte) (*Sealer, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Sealer{aead: aead}, nil
}

// Seal encrypts plaintext, returning nonce||ciphertext.
func (s *Sealer) Seal(plaintext []byte) ([]byte, error) {
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return s.aead.Seal(nonce, nonce, plaintext, nil), nil
}

// Unseal reverses Seal.
func (s *Sealer) Unseal(blob []byte) ([]byte, error) {
	n := s.aead.NonceSize()
	if len(blob) < n {
		return nil, ErrCorrupt
	}
	nonce, ct := blob[:n], blob[n:]
	pt, err := s.aead.Open(nil, nonce, ct, nil)
	if err != nil {
		return nil, ErrCorrupt
	}
	return pt, nil
}
