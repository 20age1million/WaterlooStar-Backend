package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// Lifetimes. The access token is short because nothing can revoke it once
// issued — a JWT is valid until it expires. Revocation is the refresh token's
// job, and that one lives in the database where it can be struck out.
const (
	AccessTokenTTL       = 15 * time.Minute
	RefreshTokenTTL      = 30 * 24 * time.Hour
	VerificationTokenTTL = 24 * time.Hour
	PasswordResetTTL     = 1 * time.Hour
)

const (
	issuer   = "waterloostar-backend"
	audience = "waterloostar-frontend"
)

var (
	ErrTokenInvalid = errors.New("token is not valid")
	ErrTokenExpired = errors.New("token has expired")
)

// Principal is who the request is from. It is what middleware puts on the
// request context and what handlers read.
type Principal struct {
	UserID   uuid.UUID
	Role     string
	Verified bool
}

// IsAdmin reports whether the principal holds the admin role.
func (p Principal) IsAdmin() bool { return p.Role == "admin" }

// Claims is the access token payload. `verified` rides in the token so the
// common "is this a verified student?" check needs no database round-trip; the
// cost is that verifying flips only at the next refresh, within AccessTokenTTL.
type Claims struct {
	Role     string `json:"role"`
	Verified bool   `json:"verified"`
	jwt.RegisteredClaims
}

// TokenService mints and verifies access tokens.
type TokenService struct {
	secret []byte
	now    func() time.Time
}

// NewTokenService builds a service over the signing secret.
func NewTokenService(secret string) *TokenService {
	return &TokenService{secret: []byte(secret), now: time.Now}
}

// NewTokenServiceAt builds a service with a supplied clock, so expiry behaviour
// can be tested without sleeping through a token's lifetime.
func NewTokenServiceAt(secret string, now func() time.Time) *TokenService {
	return &TokenService{secret: []byte(secret), now: now}
}

// MintAccessToken issues a signed access token for the principal.
func (s *TokenService) MintAccessToken(p Principal) (string, time.Time, error) {
	now := s.now()
	expires := now.Add(AccessTokenTTL)

	claims := Claims{
		Role:     p.Role,
		Verified: p.Verified,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   p.UserID.String(),
			Issuer:    issuer,
			Audience:  jwt.ClaimStrings{audience},
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expires),
			ID:        uuid.NewString(),
		},
	}

	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.secret)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("sign access token: %w", err)
	}
	return signed, expires, nil
}

// VerifyAccessToken checks the signature, algorithm, expiry, issuer and audience,
// returning the principal it describes.
func (s *TokenService) VerifyAccessToken(raw string) (Principal, error) {
	var claims Claims

	_, err := jwt.ParseWithClaims(raw, &claims, func(t *jwt.Token) (any, error) {
		// Pinning the method is what stops the "alg: none" and
		// RS256-verified-as-HS256 confusion attacks.
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method %v", t.Header["alg"])
		}
		return s.secret, nil
	},
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(issuer),
		jwt.WithAudience(audience),
		jwt.WithExpirationRequired(),
		jwt.WithTimeFunc(s.now),
	)
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return Principal{}, ErrTokenExpired
		}
		return Principal{}, ErrTokenInvalid
	}

	userID, err := uuid.Parse(claims.Subject)
	if err != nil {
		return Principal{}, ErrTokenInvalid
	}

	return Principal{UserID: userID, Role: claims.Role, Verified: claims.Verified}, nil
}
