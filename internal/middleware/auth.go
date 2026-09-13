package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/20age1million/waterloostar-api/internal/apierror"
	"github.com/20age1million/waterloostar-api/internal/auth"
)

// Authenticate reads the session cookie and, when it holds a valid access token,
// puts the principal on the request context. It never rejects a request.
//
// Populate-and-continue rather than reject-here because oapi-codegen registers
// every generated route in one call, so gin middleware cannot be attached
// per-route without restating the contract's security rules in Go. Enforcement
// therefore lives one layer in, in the handlers, via RequireAuth/RequireVerified
// below — which read the context this middleware fills.
func Authenticate(tokens *auth.TokenService) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()

		// The strict handlers see only a context, so the transport details the
		// auth endpoints need are lifted onto it here.
		ctx = auth.WithUserAgent(ctx, c.Request.UserAgent())
		if refresh, err := c.Request.Cookie(auth.RefreshCookieName); err == nil {
			ctx = auth.WithRefreshToken(ctx, refresh.Value)
		}
		c.Request = c.Request.WithContext(ctx)

		cookie, err := c.Request.Cookie(auth.SessionCookieName)
		if err != nil || cookie.Value == "" {
			c.Next()
			return
		}

		principal, err := tokens.VerifyAccessToken(cookie.Value)
		if err != nil {
			// An expired or tampered token is treated as no token. The client
			// discovers this as a 401 from whatever it was trying to do, and
			// refreshes.
			c.Next()
			return
		}

		c.Request = c.Request.WithContext(auth.WithPrincipal(c.Request.Context(), principal))
		c.Next()
	}
}

// CSRF enforces the double-submit check on unsafe methods.
//
// The session cookie is sent by the browser on any cross-site request, so a
// cookie alone cannot prove intent. A matching header can: another origin's
// script cannot read this site's cookie, so it cannot produce the header.
func CSRF() gin.HandlerFunc {
	return func(c *gin.Context) {
		switch c.Request.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			c.Next()
			return
		}

		// Requests with no session cookie carry no ambient authority, so there is
		// nothing for an attacker to ride. Registering and logging in land here.
		if _, err := c.Request.Cookie(auth.SessionCookieName); err != nil {
			c.Next()
			return
		}

		cookie, err := c.Request.Cookie(auth.CSRFCookieName)
		if err != nil || cookie.Value == "" {
			apierror.Write(c, http.StatusForbidden, apierror.CodeForbidden,
				"Missing CSRF token. Send the "+auth.CSRFCookieName+" cookie value in the "+
					auth.CSRFHeaderName+" header.")
			return
		}

		if !auth.ConstantTimeEqual(cookie.Value, c.GetHeader(auth.CSRFHeaderName)) {
			apierror.Write(c, http.StatusForbidden, apierror.CodeForbidden,
				"CSRF token does not match.")
			return
		}

		c.Next()
	}
}
