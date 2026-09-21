// Package auth holds the primitives behind authentication: password hashing,
// access-token minting and verification, opaque token generation, and the
// shaping of the cookies that carry them.
package auth

import (
	"errors"
	"fmt"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

// BcryptCost is stated explicitly rather than left to bcrypt.DefaultCost so that
// raising it is a visible, reviewable change.
const BcryptCost = 12

const (
	// bcrypt silently truncates at 72 bytes. Rejecting longer input is honest;
	// accepting it would mean the tail of a long passphrase does nothing.
	maxPasswordBytes = 72
	minPasswordRunes = 10
)

var (
	ErrPasswordTooShort = fmt.Errorf("password must be at least %d characters", minPasswordRunes)
	ErrPasswordTooLong  = fmt.Errorf("password must be at most %d bytes", maxPasswordBytes)
	// ErrPasswordMismatch is deliberately opaque — callers must not distinguish
	// "no such user" from "wrong password" in what they return to a client.
	ErrPasswordMismatch = errors.New("password does not match")
)

// ValidatePassword applies the policy: long enough to be worth having, short
// enough that bcrypt will actually hash all of it.
func ValidatePassword(plain string) error {
	if utf8.RuneCountInString(plain) < minPasswordRunes {
		return ErrPasswordTooShort
	}
	if len(plain) > maxPasswordBytes {
		return ErrPasswordTooLong
	}
	return nil
}

// HashPassword returns a bcrypt hash. The cost and salt travel inside the hash,
// so an existing hash stays verifiable after BcryptCost is raised.
func HashPassword(plain string) (string, error) {
	if err := ValidatePassword(plain); err != nil {
		return "", err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(plain), BcryptCost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return string(hash), nil
}

// ComparePassword reports whether the plaintext matches the stored hash.
// bcrypt's comparison is constant-time with respect to the hash contents.
func ComparePassword(hash, plain string) error {
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)); err != nil {
		return ErrPasswordMismatch
	}
	return nil
}

// DummyHash is a valid bcrypt hash of a value nobody knows. Comparing against it
// when no user was found makes a login attempt for an unknown address cost the
// same as one for a known address, so response timing does not reveal which
// addresses are registered.
const DummyHash = "$2a$12$eImiTXuWVxfM37uY4JANjQ.SgN3BFC7EbUcKBwYaHiv/ux8OqK8By"

// WasteComparison performs a throwaway bcrypt comparison purely for its timing.
func WasteComparison(plain string) {
	_ = bcrypt.CompareHashAndPassword([]byte(DummyHash), []byte(plain))
}
