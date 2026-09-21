package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/20age1million/WaterlooStar-Backend/internal/httpapi/gen"
)

// The generated response objects write a status and a body, but the auth
// endpoints also have to set cookies. Each type below satisfies the generated
// response interface for one operation and writes the cookies first.
//
// This is the intended extension point for oapi-codegen's strict server: the
// response object owns the write, so anything the contract cannot express —
// Set-Cookie, in this case — belongs here rather than leaking into middleware.

func writeJSONWithCookies(w http.ResponseWriter, status int, cookies []*http.Cookie, body any) error {
	for _, c := range cookies {
		http.SetCookie(w, c)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	return json.NewEncoder(w).Encode(body)
}

func writeStatusWithCookies(w http.ResponseWriter, status int, cookies []*http.Cookie) error {
	for _, c := range cookies {
		http.SetCookie(w, c)
	}
	w.WriteHeader(status)
	return nil
}

// loginWithCookies is a 200 carrying the user plus the session cookie set.
type loginWithCookies struct {
	user    gen.User
	cookies []*http.Cookie
}

func (r loginWithCookies) VisitLoginResponse(w http.ResponseWriter) error {
	return writeJSONWithCookies(w, http.StatusOK, r.cookies, r.user)
}

// refreshWithCookies is a 200 carrying the user plus rotated session cookies.
type refreshWithCookies struct {
	user    gen.User
	cookies []*http.Cookie
}

func (r refreshWithCookies) VisitRefreshSessionResponse(w http.ResponseWriter) error {
	return writeJSONWithCookies(w, http.StatusOK, r.cookies, r.user)
}

// logoutWithCookies is a 204 that clears the session cookies.
type logoutWithCookies struct {
	cookies []*http.Cookie
}

func (r logoutWithCookies) VisitLogoutResponse(w http.ResponseWriter) error {
	return writeStatusWithCookies(w, http.StatusNoContent, r.cookies)
}

// Compile-time proof that each type satisfies the operation it is written for.
var (
	_ gen.LoginResponseObject          = loginWithCookies{}
	_ gen.RefreshSessionResponseObject = refreshWithCookies{}
	_ gen.LogoutResponseObject         = logoutWithCookies{}
)
