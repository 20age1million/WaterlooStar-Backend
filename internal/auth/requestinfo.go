package auth

import "context"

// The strict handlers generated from the OpenAPI contract receive only a
// context.Context — by design, since the contract cannot describe cookies or
// transport headers. Middleware therefore lifts the few transport details the
// auth handlers genuinely need onto the context, and they read them from here.

type refreshTokenKey struct{}
type userAgentKey struct{}

// WithRefreshToken carries the raw refresh-cookie value for the handlers that
// rotate or revoke it.
func WithRefreshToken(ctx context.Context, raw string) context.Context {
	return context.WithValue(ctx, refreshTokenKey{}, raw)
}

// RefreshTokenFrom returns the raw refresh token, or "" when none was presented.
func RefreshTokenFrom(ctx context.Context) string {
	raw, _ := ctx.Value(refreshTokenKey{}).(string)
	return raw
}

// WithUserAgent carries the User-Agent header, recorded against each refresh
// token so a user can later be shown where their sessions are.
func WithUserAgent(ctx context.Context, ua string) context.Context {
	return context.WithValue(ctx, userAgentKey{}, ua)
}

// UserAgentFrom returns the User-Agent header, or "" when absent.
func UserAgentFrom(ctx context.Context) string {
	ua, _ := ctx.Value(userAgentKey{}).(string)
	return ua
}
