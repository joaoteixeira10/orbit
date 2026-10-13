// Package auth provides authentication and authorization: password hashing,
// JWT access tokens, revocable refresh tokens, RBAC middleware, admin
// bootstrap, and (scaffolded) OIDC SSO.
package auth

import "golang.org/x/crypto/bcrypt"

// bcryptCost balances security and login latency. 12 ≈ ~250ms on modern HW.
const bcryptCost = 12

// dummyHash is a real bcrypt hash used to keep the failed-login compare at
// constant cost for unknown usernames (defeats a timing/enumeration oracle).
// Generated once at startup so it always parses, unlike a hand-written literal.
var dummyHash = mustDummyHash()

func mustDummyHash() []byte {
	h, err := bcrypt.GenerateFromPassword([]byte("timing-equalization-placeholder"), bcryptCost)
	if err != nil {
		panic(err)
	}
	return h
}

// DummyCompare runs a constant-cost bcrypt comparison against a throwaway hash,
// so authentication takes the same time whether or not the username exists.
func DummyCompare(plain string) {
	_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(plain))
}

// HashPassword returns a bcrypt hash of the plaintext password.
func HashPassword(plain string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(plain), bcryptCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// CheckPassword reports whether plain matches the stored bcrypt hash.
func CheckPassword(hash, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}
