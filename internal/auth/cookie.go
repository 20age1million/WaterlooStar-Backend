package auth

import (
	"net/http"
	"time"
)

// Cookie names. The session and refresh cookies are httpOnly so no script can
// read them; the CSRF cookie deliberately is not, because the frontend has to
// read it to echo it back in a header.
const (
	SessionCookieName = "waterloostar_session"
	RefreshCookieName = "waterloostar_refresh"
	CSRFCookieName    = "waterloostar_csrf"
	CSRFHeaderName    = "X-CSRF-Token"
)

// refreshPath scopes the refresh cookie to the auth endpoints, so it is not
// attached to every ordinary API call.
//
// It covers /auth rather than /auth/refresh alone because logout also needs it:
// a cookie scoped to the refresh path is not sent to /auth/logout, which would
// leave logout unable to revoke the very token it is meant to kill.
const refreshPath = "/auth"

// CookieWriter shapes auth cookies consistently. Secure is off only in
// development, where the frontend is served over plain http on localhost.
type CookieWriter struct {
	Secure bool
}

// NewCookieWriter builds a writer for the environment.
func NewCookieWriter(isDevelopment bool) CookieWriter {
	return CookieWriter{Secure: !isDevelopment}
}

// SameSite=Lax rather than Strict: Strict would drop the cookie on a top-level
// navigation into the site from an email link, which is exactly how a
// verification or reset link is followed.
const sameSite = http.SameSiteLaxMode

func (w CookieWriter) base(name, value, path string, expires time.Time, httpOnly bool) *http.Cookie {
	return &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     path,
		Expires:  expires,
		MaxAge:   int(time.Until(expires).Seconds()),
		HttpOnly: httpOnly,
		Secure:   w.Secure,
		SameSite: sameSite,
	}
}

// Session builds the access-token cookie.
func (w CookieWriter) Session(token string, expires time.Time) *http.Cookie {
	return w.base(SessionCookieName, token, "/", expires, true)
}

// Refresh builds the refresh-token cookie, scoped to the refresh endpoint.
func (w CookieWriter) Refresh(token string, expires time.Time) *http.Cookie {
	return w.base(RefreshCookieName, token, refreshPath, expires, true)
}

// CSRF builds the double-submit cookie. Readable by script on purpose — the
// frontend copies its value into the X-CSRF-Token header. That is safe because
// the same-origin policy stops another site from reading it, which is the whole
// basis of the double-submit pattern.
func (w CookieWriter) CSRF(token string, expires time.Time) *http.Cookie {
	return w.base(CSRFCookieName, token, "/", expires, false)
}

// expired returns a cookie that instructs the browser to delete the named one.
// The attributes must match those it was set with or the browser keeps it.
func (w CookieWriter) expired(name, path string, httpOnly bool) *http.Cookie {
	c := w.base(name, "", path, time.Unix(0, 0), httpOnly)
	c.MaxAge = -1
	return c
}

// Clear returns the cookies that end a session.
func (w CookieWriter) Clear() []*http.Cookie {
	return []*http.Cookie{
		w.expired(SessionCookieName, "/", true),
		w.expired(RefreshCookieName, refreshPath, true),
		w.expired(CSRFCookieName, "/", false),
	}
}
