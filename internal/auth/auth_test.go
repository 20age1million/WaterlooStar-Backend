package auth_test

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/20age1million/waterloostar-api/internal/auth"
)

const secret = "test-secret-that-is-at-least-32-bytes-long"

func TestPasswordHashRoundTrip(t *testing.T) {
	const plain = "correct horse battery"

	hash, err := auth.HashPassword(plain)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if hash == plain {
		t.Fatal("the stored value must not be the password itself")
	}
	if err := auth.ComparePassword(hash, plain); err != nil {
		t.Errorf("ComparePassword with the right password: %v", err)
	}
	if err := auth.ComparePassword(hash, plain+"!"); err == nil {
		t.Error("ComparePassword accepted the wrong password")
	}
}

func TestPasswordHashIsSalted(t *testing.T) {
	const plain = "correct horse battery"

	first, err := auth.HashPassword(plain)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	second, err := auth.HashPassword(plain)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	// Equal hashes would mean no salt, and identical passwords across accounts
	// would be visible in a dump.
	if first == second {
		t.Error("two hashes of the same password should differ")
	}
}

func TestValidatePassword(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		wantErr error
	}{
		{"too short", "short", auth.ErrPasswordTooShort},
		{"exactly ten", "abcdefghij", nil},
		{"long but fine", "a correct horse battery staple phrase", nil},
		// bcrypt silently truncates past 72 bytes; accepting longer input would
		// mean the tail does nothing.
		{"past bcrypt limit", string(make([]byte, 73)), auth.ErrPasswordTooLong},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := auth.ValidatePassword(tc.input)
			if tc.wantErr == nil && err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if tc.wantErr != nil && err != tc.wantErr {
				t.Fatalf("expected %v, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestDummyHashIsUsable(t *testing.T) {
	// The constant-time login path compares against this when no user exists.
	// If it were not a valid bcrypt hash, the comparison would return early and
	// the timing defence would be worthless.
	if err := auth.ComparePassword(auth.DummyHash, "anything at all"); err == nil {
		t.Error("DummyHash should not match an arbitrary password")
	}
	auth.WasteComparison("anything at all") // must not panic
}

func TestAccessTokenRoundTrip(t *testing.T) {
	svc := auth.NewTokenService(secret)
	want := auth.Principal{UserID: uuid.New(), Role: "user", Verified: true}

	token, expires, err := svc.MintAccessToken(want)
	if err != nil {
		t.Fatalf("MintAccessToken: %v", err)
	}
	if !expires.After(time.Now()) {
		t.Error("token should expire in the future")
	}

	got, err := svc.VerifyAccessToken(token)
	if err != nil {
		t.Fatalf("VerifyAccessToken: %v", err)
	}
	if got != want {
		t.Errorf("principal = %+v, want %+v", got, want)
	}
}

func TestVerifyRejectsTamperedSignature(t *testing.T) {
	svc := auth.NewTokenService(secret)
	token, _, err := svc.MintAccessToken(auth.Principal{UserID: uuid.New(), Role: "user"})
	if err != nil {
		t.Fatalf("MintAccessToken: %v", err)
	}

	// Flip the final character of the signature.
	tampered := token[:len(token)-1]
	if token[len(token)-1] == 'A' {
		tampered += "B"
	} else {
		tampered += "A"
	}

	if _, err := svc.VerifyAccessToken(tampered); err == nil {
		t.Error("a tampered signature must be rejected")
	}
}

func TestVerifyRejectsForeignSecret(t *testing.T) {
	token, _, err := auth.NewTokenService(secret).MintAccessToken(
		auth.Principal{UserID: uuid.New(), Role: "admin", Verified: true})
	if err != nil {
		t.Fatalf("MintAccessToken: %v", err)
	}

	other := auth.NewTokenService("a completely different secret value!!")
	if _, err := other.VerifyAccessToken(token); err == nil {
		t.Error("a token signed with another secret must be rejected")
	}
}

func TestVerifyRejectsExpiredToken(t *testing.T) {
	svc := auth.NewTokenService(secret)
	token, _, err := svc.MintAccessToken(auth.Principal{UserID: uuid.New(), Role: "user"})
	if err != nil {
		t.Fatalf("MintAccessToken: %v", err)
	}

	// Verify from a point past the token's lifetime.
	future := auth.NewTokenServiceAt(secret, func() time.Time {
		return time.Now().Add(auth.AccessTokenTTL + time.Minute)
	})
	if _, err := future.VerifyAccessToken(token); err != auth.ErrTokenExpired {
		t.Errorf("expected ErrTokenExpired, got %v", err)
	}
}

func TestVerifyRejectsUnsignedToken(t *testing.T) {
	// The "alg: none" attack: a token with no signature at all.
	unsigned, err := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.RegisteredClaims{
		Subject:   uuid.NewString(),
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	}).SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("build unsigned token: %v", err)
	}

	if _, err := auth.NewTokenService(secret).VerifyAccessToken(unsigned); err == nil {
		t.Error("an unsigned token must be rejected")
	}
}

func TestVerifyRejectsGarbage(t *testing.T) {
	svc := auth.NewTokenService(secret)
	for _, input := range []string{"", "not-a-token", "a.b.c"} {
		if _, err := svc.VerifyAccessToken(input); err == nil {
			t.Errorf("VerifyAccessToken(%q) should have failed", input)
		}
	}
}

func TestOpaqueTokenIsRandomAndHashed(t *testing.T) {
	raw, hash, err := auth.NewOpaqueToken()
	if err != nil {
		t.Fatalf("NewOpaqueToken: %v", err)
	}
	if raw == "" || len(hash) != 32 {
		t.Fatalf("raw=%q hashLen=%d; want a token and a 32-byte hash", raw, len(hash))
	}
	if string(hash) == raw {
		t.Error("the stored hash must not equal the raw token")
	}

	// Hashing is deterministic, which is what makes lookup by hash work.
	if string(auth.HashToken(raw)) != string(hash) {
		t.Error("HashToken should reproduce the hash for the same input")
	}

	other, _, err := auth.NewOpaqueToken()
	if err != nil {
		t.Fatalf("NewOpaqueToken: %v", err)
	}
	if other == raw {
		t.Error("two generated tokens must differ")
	}
}

func TestConstantTimeEqual(t *testing.T) {
	if !auth.ConstantTimeEqual("abc123", "abc123") {
		t.Error("identical values should compare equal")
	}
	if auth.ConstantTimeEqual("abc123", "abc124") {
		t.Error("different values should not compare equal")
	}
	if auth.ConstantTimeEqual("abc123", "") {
		t.Error("an empty candidate should never match")
	}
}

func TestCookieSecurityAttributes(t *testing.T) {
	t.Run("development allows plain http", func(t *testing.T) {
		w := auth.NewCookieWriter(true)
		if w.Session("token", time.Now().Add(time.Hour)).Secure {
			t.Error("Secure must be off in development, where localhost is plain http")
		}
	})

	t.Run("production requires https", func(t *testing.T) {
		w := auth.NewCookieWriter(false)
		c := w.Session("token", time.Now().Add(time.Hour))
		if !c.Secure {
			t.Error("Secure must be on outside development")
		}
		if !c.HttpOnly {
			t.Error("the session cookie must be httpOnly so script cannot read it")
		}
	})

	t.Run("csrf cookie is readable by script", func(t *testing.T) {
		// The frontend has to read it to echo it in a header; that is the whole
		// basis of the double-submit pattern.
		if auth.NewCookieWriter(false).CSRF("token", time.Now().Add(time.Hour)).HttpOnly {
			t.Error("the CSRF cookie must not be httpOnly")
		}
	})

	t.Run("clear expires every auth cookie", func(t *testing.T) {
		cleared := auth.NewCookieWriter(true).Clear()
		if len(cleared) != 3 {
			t.Fatalf("Clear() returned %d cookies, want 3", len(cleared))
		}
		for _, c := range cleared {
			if c.Value != "" || c.MaxAge >= 0 {
				t.Errorf("cookie %q should be emptied and expired, got value=%q maxAge=%d",
					c.Name, c.Value, c.MaxAge)
			}
		}
	})
}
