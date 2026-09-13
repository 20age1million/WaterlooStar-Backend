package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
)

// tokenBytes is the entropy in an opaque token — refresh tokens, verification
// links, reset links. 32 bytes is well beyond guessing range and still produces
// a URL-safe string short enough to sit in an email.
const tokenBytes = 32

// NewOpaqueToken returns a URL-safe random token and the SHA-256 hash to store.
//
// The raw value is returned to be handed to exactly one place — a cookie or an
// email — and is never persisted. Only the hash reaches the database, so a
// database dump yields nothing that can be presented as a credential.
//
// SHA-256 rather than bcrypt is correct here: these tokens carry full random
// entropy, so there is no dictionary to slow down, and lookup happens on every
// refresh where a deliberately slow hash would hurt.
func NewOpaqueToken() (raw string, hash []byte, err error) {
	buf := make([]byte, tokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", nil, fmt.Errorf("generate token: %w", err)
	}
	raw = base64.RawURLEncoding.EncodeToString(buf)
	return raw, HashToken(raw), nil
}

// HashToken returns the stored form of an opaque token.
func HashToken(raw string) []byte {
	sum := sha256.Sum256([]byte(raw))
	return sum[:]
}

// ConstantTimeEqual compares two secrets without leaking their contents through
// timing. Used for the CSRF double-submit check.
func ConstantTimeEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
