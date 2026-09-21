package auth

import "context"

// principalKey is unexported so nothing outside this package can plant a
// Principal on a context — the only way one gets there is through the
// authentication middleware.
type principalKey struct{}

// WithPrincipal returns a context carrying the authenticated principal.
func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, p)
}

// PrincipalFrom returns the principal on the context, if the request was
// authenticated. Handlers use this rather than reaching for the gin context, so
// they stay testable as plain functions.
func PrincipalFrom(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(principalKey{}).(Principal)
	return p, ok
}
