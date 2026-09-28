package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/20age1million/WaterlooStar-Backend/internal/apierror"
	"github.com/20age1million/WaterlooStar-Backend/internal/auth"
	"github.com/20age1million/WaterlooStar-Backend/internal/ratelimit"
)

// Rate limiting, at the edge of the API.
//
// It is middleware rather than handler code because the point is to answer
// *before* the expensive part runs — a bcrypt comparison, a database round trip,
// an email. The generated handlers are all registered in one call, so the
// middleware matches on the route itself rather than being attached per route.
//
// The route table below is an allowlist: a new endpoint is unlimited until
// somebody decides otherwise. The alternative — limiting everything by default —
// would eventually throttle something that should not be, in a way nobody
// noticed until a user did.

// ClientIPHeader is an optional address the frontend may forward.
//
// This service has no public URL: the Next.js server is its only client, so
// every request arrives from one address on the compose network and per-IP
// counting here would treat the whole internet as one client. When the frontend
// forwards the real address it becomes an extra dimension of the key, which
// separates two people behind one frontend. It is trusted only because the
// service is unreachable from the internet.
const ClientIPHeader = "X-Client-IP"

// limitKind says where a limited route's key comes from.
type limitKind int

const (
	// keyByEmail reads the email out of the JSON body. Registration, login and
	// the reset request all identify the account that way.
	keyByEmail limitKind = iota
	// keyByToken reads a one-shot token out of the body, and hashes it so the
	// limiter's memory never holds the secret itself.
	keyByToken
	// keyByUser uses the authenticated principal. Anonymous callers are not
	// limited here, because they cannot reach these routes at all.
	keyByUser
)

// limitedRoute is one entry in the allowlist.
type limitedRoute struct {
	method string
	// path is gin's route pattern, as c.FullPath() reports it.
	path string
	kind limitKind
	// perKey applies to the identity; global is the service-wide backstop that
	// catches an attacker varying the identity to stay under perKey.
	perKey ratelimit.Rule
	global ratelimit.Rule
	// chargeOnFailureOnly leaves the spending to the handler. Login sets it: a
	// correct password costs nothing, so ordinary use never meets the limit.
	chargeOnFailureOnly bool
	// message is the plain-English refusal, with a %s for the wait.
	message string
}

// limitedRoutes is every rate-limited endpoint in the service.
var limitedRoutes = []limitedRoute{
	{
		method: http.MethodPost, path: "/auth/login", kind: keyByEmail,
		perKey: ratelimit.LoginPerEmail, global: ratelimit.LoginGlobal,
		chargeOnFailureOnly: true,
		message:             "Too many sign-in attempts for that address. Try again in %s.",
	},
	{
		method: http.MethodPost, path: "/auth/register", kind: keyByEmail,
		perKey:  ratelimit.RegisterPerEmail,
		global:  ratelimit.RegisterGlobal,
		message: "Too many sign-up attempts. Try again in %s.",
	},
	{
		method: http.MethodPost, path: "/auth/password-reset", kind: keyByEmail,
		perKey:  ratelimit.ResetRequestPerEmail,
		global:  ratelimit.ResetRequestGlobal,
		message: "A reset link has already been requested for that address. Try again in %s.",
	},
	{
		method: http.MethodPost, path: "/auth/password-reset/confirm", kind: keyByToken,
		perKey:  ratelimit.TokenPerToken,
		global:  ratelimit.TokenGlobal,
		message: "Too many attempts with that link. Try again in %s.",
	},
	{
		method: http.MethodPost, path: "/auth/verify", kind: keyByToken,
		perKey:  ratelimit.TokenPerToken,
		global:  ratelimit.TokenGlobal,
		message: "Too many attempts with that link. Try again in %s.",
	},
	{
		method: http.MethodPost, path: "/listings", kind: keyByUser,
		perKey:  ratelimit.WritePerUser,
		message: "You have posted a lot in a short time. Try again in %s.",
	},
	{
		method: http.MethodPost, path: "/requests", kind: keyByUser,
		perKey:  ratelimit.WritePerUser,
		message: "You have posted a lot in a short time. Try again in %s.",
	},
	{
		method: http.MethodPost, path: "/requests/:id/offers", kind: keyByUser,
		perKey:  ratelimit.WritePerUser,
		message: "You have sent a lot of offers in a short time. Try again in %s.",
	},
}

// loginRouteKey is the context key under which the middleware leaves the login
// bucket key, so the handler can charge a failed attempt against the same
// bucket the middleware checked.
const loginRouteKey = "ratelimit_login_key"

// RateLimit rejects a request whose identity has spent its allowance.
func RateLimit(limiter *ratelimit.Limiter, log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		route, ok := matchRoute(c)
		if !ok {
			c.Next()
			return
		}

		identity, ok := routeIdentity(c, route)
		if !ok {
			// Nothing to key on — an anonymous write, or a body the handler is
			// about to reject anyway. The handler's own refusal is the better
			// answer, and the global backstop below still applies.
			identity = ""
		}

		if route.global.Burst > 0 {
			if d := limiter.Allow(route.global, ratelimit.GlobalKey(route.global)); !d.OK {
				refuse(c, log, route, "global", d.RetryAfter)
				return
			}
		}

		if identity == "" {
			c.Next()
			return
		}

		key := route.perKey.Name + ":" + identity
		if ip := strings.TrimSpace(c.GetHeader(ClientIPHeader)); ip != "" {
			key += "|" + ip
		}

		if route.chargeOnFailureOnly {
			// Peek, do not spend: the handler charges only a failed attempt.
			if d := limiter.Peek(route.perKey, key); !d.OK {
				refuse(c, log, route, identity, d.RetryAfter)
				return
			}
			c.Set(loginRouteKey, key)
			c.Next()
			return
		}

		if d := limiter.Allow(route.perKey, key); !d.OK {
			refuse(c, log, route, identity, d.RetryAfter)
			return
		}
		c.Next()
	}
}

// chargeFailedLogin spends one token against the bucket the middleware checked.
//
// Login is the one route that pays after the fact rather than in advance: a
// correct password costs nothing, so a person who signs in every morning — and
// the end-to-end suite, which signs in repeatedly on purpose — never meets the
// limit. Only guessing does.
func (s *Server) chargeFailedLogin(ctx context.Context) {
	if s.limiter == nil {
		return
	}
	key, _ := ctx.Value(loginRouteKey).(string)
	if key == "" {
		// No key means the request did not come through the middleware — a unit
		// test calling the handler directly. Nothing to charge.
		return
	}
	s.limiter.Spend(ratelimit.LoginPerEmail, key)
}

// matchRoute finds the allowlist entry for this request, if any.
func matchRoute(c *gin.Context) (limitedRoute, bool) {
	path := c.FullPath()
	for _, r := range limitedRoutes {
		if r.method == c.Request.Method && r.path == path {
			return r, true
		}
	}
	return limitedRoute{}, false
}

// routeIdentity derives the identity this route is keyed on.
func routeIdentity(c *gin.Context, route limitedRoute) (string, bool) {
	switch route.kind {
	case keyByEmail:
		addr, ok := bodyField(c, "email")
		if !ok {
			return "", false
		}
		// Lowercased so Alice@ and alice@ cannot each get a full allowance;
		// the database compares addresses the same way.
		return strings.ToLower(strings.TrimSpace(addr)), true
	case keyByToken:
		token, ok := bodyField(c, "token")
		if !ok {
			return "", false
		}
		token = strings.TrimSpace(token)
		if token == "" {
			return "", false
		}
		// Hashed and truncated: the limiter keeps a fingerprint, never the
		// secret. It only ever needs to tell two tokens apart.
		sum := sha256.Sum256([]byte(token))
		return hex.EncodeToString(sum[:8]), true
	case keyByUser:
		principal, ok := auth.PrincipalFrom(c.Request.Context())
		if !ok {
			return "", false
		}
		return principal.UserID.String(), true
	}
	return "", false
}

// bodyField reads one string field from the JSON body and puts the body back,
// because the generated handler is about to read it too.
func bodyField(c *gin.Context, field string) (string, bool) {
	if c.Request.Body == nil {
		return "", false
	}
	raw, err := io.ReadAll(c.Request.Body)
	// Restored unconditionally, including on a read error: leaving the handler
	// with a drained body would turn a rate-limit concern into a broken request.
	c.Request.Body = io.NopCloser(bytes.NewReader(raw))
	if err != nil || len(raw) == 0 {
		return "", false
	}

	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		// Malformed JSON is the handler's to refuse, with a message about the
		// body rather than about limits.
		return "", false
	}
	value, ok := fields[field].(string)
	if !ok {
		return "", false
	}
	return value, true
}

// refuse writes the 429 and logs it, so an attack in progress is visible in the
// API log rather than only in the attacker's terminal.
func refuse(c *gin.Context, log *slog.Logger, route limitedRoute, identity string, retryAfter time.Duration) {
	if log != nil {
		log.Warn("rate limit reached",
			slog.String("rule", route.perKey.Name),
			slog.String("path", c.Request.URL.Path),
			// The identity is an email on some routes, so it is logged as a
			// fingerprint. "Which account is under attack" is answerable by
			// matching it; the log itself stays free of addresses.
			slog.String("identity", fingerprint(identity)),
			slog.Duration("retry_after", retryAfter),
			slog.String("request_id", c.GetString(apierror.RequestIDKey)),
		)
	}
	apierror.TooManyRequests(c, fmt.Sprintf(route.message, humanWait(retryAfter)), retryAfter)
}

// fingerprint keeps identities out of the log in readable form.
func fingerprint(identity string) string {
	if identity == "" || identity == "global" {
		return identity
	}
	sum := sha256.Sum256([]byte(identity))
	return hex.EncodeToString(sum[:6])
}

// humanWait renders a duration the way a person would say it.
func humanWait(d time.Duration) string {
	switch {
	case d < time.Minute:
		seconds := int(d.Round(time.Second).Seconds())
		if seconds <= 1 {
			return "a moment"
		}
		return fmt.Sprintf("%d seconds", seconds)
	case d < time.Hour:
		minutes := int(d.Round(time.Minute).Minutes())
		if minutes <= 1 {
			return "a minute"
		}
		return fmt.Sprintf("%d minutes", minutes)
	default:
		hours := int(d.Round(time.Hour).Hours())
		if hours <= 1 {
			return "an hour"
		}
		return fmt.Sprintf("%d hours", hours)
	}
}
