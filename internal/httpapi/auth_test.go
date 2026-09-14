package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/20age1million/WaterlooStar-Backend/internal/apierror"
	"github.com/20age1million/WaterlooStar-Backend/internal/auth"
	"github.com/20age1million/WaterlooStar-Backend/internal/config"
	"github.com/20age1million/WaterlooStar-Backend/internal/email"
	"github.com/20age1million/WaterlooStar-Backend/internal/httpapi"
)

const (
	testEmail    = "meil@uwaterloo.ca"
	testUsername = "meil"
	testPassword = "correct horse battery"
)

// capturingMailer records what would have been sent, so a test can pull the
// verification link out of the email exactly as a user would.
type capturingMailer struct {
	mu   sync.Mutex
	sent []email.Message
}

func (m *capturingMailer) Send(_ context.Context, msg email.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = append(m.sent, msg)
	return nil
}

func (m *capturingMailer) last() (email.Message, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.sent) == 0 {
		return email.Message{}, false
	}
	return m.sent[len(m.sent)-1], true
}

var linkPattern = regexp.MustCompile(`https?://\S+`)

// token extracts the token from the last link sent by email.
func (m *capturingMailer) token(t *testing.T) string {
	t.Helper()
	msg, ok := m.last()
	if !ok {
		t.Fatal("no email was sent")
	}
	link := linkPattern.FindString(msg.Body)
	if link == "" {
		t.Fatalf("no link in email body: %q", msg.Body)
	}
	parts := strings.Split(strings.TrimSpace(link), "/")
	return parts[len(parts)-1]
}

type harness struct {
	server *httptest.Server
	db     *fakeQuerier
	mail   *capturingMailer
	client *http.Client
}

// newHarness builds a server over in-memory storage, with a cookie jar so the
// client behaves like a browser — which is the whole point, given the session
// travels as a cookie.
func newHarness(t *testing.T) *harness {
	t.Helper()
	gin.SetMode(gin.TestMode)

	cfg := config.Config{
		Port:        8080,
		CORSOrigin:  "http://localhost:3000",
		AppURL:      "http://localhost:3000",
		Environment: config.Development,
		JWTSecret:   "a-test-secret-that-is-long-enough-to-pass",
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	db := newFakeQuerier()
	mail := &capturingMailer{}
	srv := httptest.NewServer(httpapi.NewRouter(cfg, log,
		httpapi.NewServerWithQuerier(cfg, log, db, mail, "")))
	t.Cleanup(srv.Close)

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar: %v", err)
	}

	return &harness{server: srv, db: db, mail: mail, client: &http.Client{Jar: jar}}
}

// do sends a request, attaching the CSRF header from the jar when one is present
// — exactly what the real frontend has to do.
func (h *harness) do(t *testing.T, method, path string, body any) *http.Response {
	t.Helper()

	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(encoded)
	}

	req, err := http.NewRequest(method, h.server.URL+path, reader)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for _, c := range h.client.Jar.Cookies(req.URL) {
		if c.Name == auth.CSRFCookieName {
			req.Header.Set(auth.CSRFHeaderName, c.Value)
		}
	}

	res, err := h.client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	t.Cleanup(func() { res.Body.Close() })
	return res
}

func decode[T any](t *testing.T, res *http.Response) T {
	t.Helper()
	var v T
	if err := json.NewDecoder(res.Body).Decode(&v); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	return v
}

type userBody struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
	Role     string `json:"role"`
	Verified bool   `json:"verified"`
}

func registerBody(overrides map[string]string) map[string]string {
	body := map[string]string{"email": testEmail, "username": testUsername, "password": testPassword}
	for k, v := range overrides {
		body[k] = v
	}
	return body
}

// registerAndVerify takes an account all the way to verified, which most of the
// later tests need as a starting point.
func (h *harness) registerAndVerify(t *testing.T) {
	t.Helper()
	if res := h.do(t, http.MethodPost, "/auth/register", registerBody(nil)); res.StatusCode != http.StatusCreated {
		t.Fatalf("register: status %d", res.StatusCode)
	}
	if res := h.do(t, http.MethodPost, "/auth/verify",
		map[string]string{"token": h.mail.token(t)}); res.StatusCode != http.StatusOK {
		t.Fatalf("verify: status %d", res.StatusCode)
	}
}

// ---------------------------------------------------------------- registration

func TestRegisterAcceptsUWaterlooAddress(t *testing.T) {
	h := newHarness(t)

	res := h.do(t, http.MethodPost, "/auth/register", registerBody(nil))
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201", res.StatusCode)
	}

	body := decode[struct {
		User                 userBody `json:"user"`
		VerificationRequired bool     `json:"verification_required"`
	}](t, res)

	if !body.VerificationRequired {
		t.Error("verification_required should be true")
	}
	if body.User.Verified {
		t.Error("a new account must start unverified")
	}
	if body.User.Email != testEmail {
		t.Errorf("email = %q, want %q", body.User.Email, testEmail)
	}
	if _, ok := h.mail.last(); !ok {
		t.Error("registration should send a confirmation email")
	}
}

func TestRegisterRejectsNonUWaterlooAddresses(t *testing.T) {
	// The last two matter most: a domain that merely ends in the same letters,
	// and one that puts uwaterloo.ca in the wrong position, must both fail.
	for _, addr := range []string{
		"someone@gmail.com",
		"someone@evil-uwaterloo.ca",
		"someone@uwaterloo.ca.attacker.com",
	} {
		t.Run(addr, func(t *testing.T) {
			h := newHarness(t)
			res := h.do(t, http.MethodPost, "/auth/register", registerBody(map[string]string{"email": addr}))

			if res.StatusCode != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400 for %q", res.StatusCode, addr)
			}
			env := decode[apierror.Envelope](t, res)
			if env.Details["email"] == "" {
				t.Errorf("expected a field-level email problem, got %+v", env)
			}
		})
	}
}

func TestRegisterRejectsMalformedEmailWithTheStandardEnvelope(t *testing.T) {
	h := newHarness(t)
	// A syntactically invalid address is caught by the generated request binding
	// before any handler runs. That path must still answer with the documented
	// envelope rather than oapi-codegen's default {"msg": ...}.
	res := h.do(t, http.MethodPost, "/auth/register", registerBody(map[string]string{"email": "not-an-email"}))

	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", res.StatusCode)
	}
	env := decode[apierror.Envelope](t, res)
	if env.Code == "" || env.Message == "" {
		t.Errorf("binding failures must use the standard envelope, got %+v", env)
	}
}

func TestRegisterAcceptsUWaterlooSubdomain(t *testing.T) {
	h := newHarness(t)
	res := h.do(t, http.MethodPost, "/auth/register",
		registerBody(map[string]string{"email": "someone@edu.uwaterloo.ca"}))
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201 for a uwaterloo.ca subdomain", res.StatusCode)
	}
}

func TestRegisterRejectsWeakPassword(t *testing.T) {
	h := newHarness(t)
	res := h.do(t, http.MethodPost, "/auth/register", registerBody(map[string]string{"password": "short"}))

	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", res.StatusCode)
	}
	if decode[apierror.Envelope](t, res).Details["password"] == "" {
		t.Error("expected a field-level password problem")
	}
}

func TestRegisterRejectsDuplicateWithoutRevealingWhich(t *testing.T) {
	h := newHarness(t)
	if res := h.do(t, http.MethodPost, "/auth/register", registerBody(nil)); res.StatusCode != http.StatusCreated {
		t.Fatalf("first register: status %d", res.StatusCode)
	}

	// The property that matters is that the two collisions are indistinguishable:
	// if they differed, this endpoint would become a way to discover which
	// addresses and usernames are taken.
	duplicateEmail := h.do(t, http.MethodPost, "/auth/register",
		registerBody(map[string]string{"username": "someoneelse"}))
	duplicateUsername := h.do(t, http.MethodPost, "/auth/register",
		registerBody(map[string]string{"email": "other@uwaterloo.ca"}))

	if duplicateEmail.StatusCode != http.StatusConflict {
		t.Fatalf("duplicate email: status = %d, want 409", duplicateEmail.StatusCode)
	}
	if duplicateUsername.StatusCode != http.StatusConflict {
		t.Fatalf("duplicate username: status = %d, want 409", duplicateUsername.StatusCode)
	}

	a := decode[apierror.Envelope](t, duplicateEmail)
	b := decode[apierror.Envelope](t, duplicateUsername)
	if a.Code != b.Code || a.Message != b.Message {
		t.Errorf("the two collisions are distinguishable and leak which field is taken:\n  %+v\n  %+v", a, b)
	}
	if len(a.Details) != 0 {
		t.Errorf("field-level details would reveal which field collided: %+v", a.Details)
	}
}

// ---------------------------------------------------------------- verification

func TestVerifyMarksAccountVerified(t *testing.T) {
	h := newHarness(t)
	if res := h.do(t, http.MethodPost, "/auth/register", registerBody(nil)); res.StatusCode != http.StatusCreated {
		t.Fatalf("register: status %d", res.StatusCode)
	}

	token := h.mail.token(t)
	res := h.do(t, http.MethodPost, "/auth/verify", map[string]string{"token": token})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	if !decode[userBody](t, res).Verified {
		t.Error("account should be verified after redeeming the token")
	}

	// Single use: replaying the same link must not work.
	if replay := h.do(t, http.MethodPost, "/auth/verify",
		map[string]string{"token": token}); replay.StatusCode != http.StatusBadRequest {
		t.Errorf("replayed token: status = %d, want 400", replay.StatusCode)
	}
}

func TestVerifyRejectsUnknownToken(t *testing.T) {
	h := newHarness(t)
	res := h.do(t, http.MethodPost, "/auth/verify", map[string]string{"token": "not-a-real-token"})
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", res.StatusCode)
	}
}

// ----------------------------------------------------------------------- login

func TestLoginSetsSessionCookies(t *testing.T) {
	h := newHarness(t)
	h.registerAndVerify(t)

	res := h.do(t, http.MethodPost, "/auth/login",
		map[string]string{"email": testEmail, "password": testPassword})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}

	got := map[string]*http.Cookie{}
	for _, c := range res.Cookies() {
		got[c.Name] = c
	}

	session, ok := got[auth.SessionCookieName]
	if !ok {
		t.Fatal("login must set the session cookie")
	}
	if !session.HttpOnly {
		t.Error("the session cookie must be httpOnly")
	}
	if _, ok := got[auth.RefreshCookieName]; !ok {
		t.Error("login must set the refresh cookie")
	}
	csrf, ok := got[auth.CSRFCookieName]
	if !ok {
		t.Fatal("login must set the CSRF cookie")
	}
	if csrf.HttpOnly {
		t.Error("the CSRF cookie must be readable by script so it can be echoed in a header")
	}
}

func TestLoginRejectsWrongPasswordAndUnknownAddressIdentically(t *testing.T) {
	h := newHarness(t)
	h.registerAndVerify(t)

	wrongPassword := h.do(t, http.MethodPost, "/auth/login",
		map[string]string{"email": testEmail, "password": "not the password"})
	unknownAddress := h.do(t, http.MethodPost, "/auth/login",
		map[string]string{"email": "nobody@uwaterloo.ca", "password": testPassword})

	if wrongPassword.StatusCode != http.StatusUnauthorized {
		t.Errorf("wrong password: status = %d, want 401", wrongPassword.StatusCode)
	}
	if unknownAddress.StatusCode != http.StatusUnauthorized {
		t.Errorf("unknown address: status = %d, want 401", unknownAddress.StatusCode)
	}

	// Identical wording, so the response cannot be used to discover which
	// addresses have accounts.
	a := decode[apierror.Envelope](t, wrongPassword)
	b := decode[apierror.Envelope](t, unknownAddress)
	if a.Message != b.Message || a.Code != b.Code {
		t.Errorf("responses differ and leak account existence:\n  %+v\n  %+v", a, b)
	}
}

func TestUnverifiedAccountCanStillLogIn(t *testing.T) {
	h := newHarness(t)
	if res := h.do(t, http.MethodPost, "/auth/register", registerBody(nil)); res.StatusCode != http.StatusCreated {
		t.Fatalf("register: status %d", res.StatusCode)
	}

	// Logging in is allowed; it is acting as a verified student that is not.
	res := h.do(t, http.MethodPost, "/auth/login",
		map[string]string{"email": testEmail, "password": testPassword})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	if decode[userBody](t, res).Verified {
		t.Error("the user should still be unverified")
	}
}

// ------------------------------------------------------------------------- me

func TestGetMeRequiresASession(t *testing.T) {
	h := newHarness(t)

	res := h.do(t, http.MethodGet, "/me", nil)
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", res.StatusCode)
	}
	if decode[apierror.Envelope](t, res).Code != apierror.CodeUnauthorized {
		t.Error("expected the unauthorized code in the standard envelope")
	}
}

func TestGetMeReturnsTheSignedInUser(t *testing.T) {
	h := newHarness(t)
	h.registerAndVerify(t)
	if res := h.do(t, http.MethodPost, "/auth/login",
		map[string]string{"email": testEmail, "password": testPassword}); res.StatusCode != http.StatusOK {
		t.Fatalf("login: status %d", res.StatusCode)
	}

	res := h.do(t, http.MethodGet, "/me", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}

	user := decode[userBody](t, res)
	if user.Username != testUsername {
		t.Errorf("username = %q, want %q", user.Username, testUsername)
	}
	if !user.Verified {
		t.Error("the account was verified before logging in")
	}
}

func TestGetMeRejectsATamperedSessionCookie(t *testing.T) {
	h := newHarness(t)
	h.registerAndVerify(t)
	if res := h.do(t, http.MethodPost, "/auth/login",
		map[string]string{"email": testEmail, "password": testPassword}); res.StatusCode != http.StatusOK {
		t.Fatalf("login: status %d", res.StatusCode)
	}

	req, err := http.NewRequest(http.MethodGet, h.server.URL+"/me", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: "forged.token.value"})

	res, err := (&http.Client{}).Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 for a forged cookie", res.StatusCode)
	}
}

// -------------------------------------------------------------- refresh/logout

func TestRefreshRotatesTheRefreshToken(t *testing.T) {
	h := newHarness(t)
	h.registerAndVerify(t)
	login := h.do(t, http.MethodPost, "/auth/login",
		map[string]string{"email": testEmail, "password": testPassword})
	if login.StatusCode != http.StatusOK {
		t.Fatalf("login: status %d", login.StatusCode)
	}

	var original string
	for _, c := range login.Cookies() {
		if c.Name == auth.RefreshCookieName {
			original = c.Value
		}
	}
	if original == "" {
		t.Fatal("no refresh cookie from login")
	}

	res := h.do(t, http.MethodPost, "/auth/refresh", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("refresh: status = %d, want 200", res.StatusCode)
	}

	var rotated string
	for _, c := range res.Cookies() {
		if c.Name == auth.RefreshCookieName {
			rotated = c.Value
		}
	}
	if rotated == "" || rotated == original {
		t.Fatal("refresh must issue a different refresh token")
	}

	// The old token must be dead: that is what limits the damage of a stolen one.
	replay, err := http.NewRequest(http.MethodPost, h.server.URL+"/auth/refresh", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	replay.AddCookie(&http.Cookie{Name: auth.RefreshCookieName, Value: original})
	replayRes, err := (&http.Client{}).Do(replay)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	defer replayRes.Body.Close()

	if replayRes.StatusCode != http.StatusUnauthorized {
		t.Errorf("replaying the rotated token: status = %d, want 401", replayRes.StatusCode)
	}
}

func TestRefreshWithoutACookieIsUnauthorized(t *testing.T) {
	h := newHarness(t)
	res := h.do(t, http.MethodPost, "/auth/refresh", nil)
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", res.StatusCode)
	}
}

func TestLogoutRevokesTheSession(t *testing.T) {
	h := newHarness(t)
	h.registerAndVerify(t)
	if res := h.do(t, http.MethodPost, "/auth/login",
		map[string]string{"email": testEmail, "password": testPassword}); res.StatusCode != http.StatusOK {
		t.Fatalf("login: status %d", res.StatusCode)
	}

	user, err := h.db.GetUserByEmail(context.Background(), testEmail)
	if err != nil {
		t.Fatalf("load user: %v", err)
	}
	if got := h.db.liveRefreshTokenCount(user.ID); got != 1 {
		t.Fatalf("live refresh tokens before logout = %d, want 1", got)
	}

	res := h.do(t, http.MethodPost, "/auth/logout", nil)
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", res.StatusCode)
	}
	if got := h.db.liveRefreshTokenCount(user.ID); got != 0 {
		t.Errorf("live refresh tokens after logout = %d, want 0", got)
	}
}

func TestLogoutWithoutASessionStillSucceeds(t *testing.T) {
	h := newHarness(t)
	// A client must always be able to reach a signed-out state.
	if res := h.do(t, http.MethodPost, "/auth/logout", nil); res.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", res.StatusCode)
	}
}

// -------------------------------------------------------------- password reset

func TestPasswordResetChangesPasswordAndRevokesSessions(t *testing.T) {
	h := newHarness(t)
	h.registerAndVerify(t)
	if res := h.do(t, http.MethodPost, "/auth/login",
		map[string]string{"email": testEmail, "password": testPassword}); res.StatusCode != http.StatusOK {
		t.Fatalf("login: status %d", res.StatusCode)
	}

	user, err := h.db.GetUserByEmail(context.Background(), testEmail)
	if err != nil {
		t.Fatalf("load user: %v", err)
	}

	if res := h.do(t, http.MethodPost, "/auth/password-reset",
		map[string]string{"email": testEmail}); res.StatusCode != http.StatusAccepted {
		t.Fatalf("request reset: status = %d, want 202", res.StatusCode)
	}

	const newPassword = "an entirely different passphrase"
	res := h.do(t, http.MethodPost, "/auth/password-reset/confirm",
		map[string]string{"token": h.mail.token(t), "password": newPassword})
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("confirm reset: status = %d, want 204", res.StatusCode)
	}

	// A reset is usually prompted by a compromise, so it must evict live sessions.
	if got := h.db.liveRefreshTokenCount(user.ID); got != 0 {
		t.Errorf("live refresh tokens after reset = %d, want 0", got)
	}

	if old := h.do(t, http.MethodPost, "/auth/login",
		map[string]string{"email": testEmail, "password": testPassword}); old.StatusCode != http.StatusUnauthorized {
		t.Errorf("old password: status = %d, want 401", old.StatusCode)
	}
	if fresh := h.do(t, http.MethodPost, "/auth/login",
		map[string]string{"email": testEmail, "password": newPassword}); fresh.StatusCode != http.StatusOK {
		t.Errorf("new password: status = %d, want 200", fresh.StatusCode)
	}
}

func TestPasswordResetHidesWhetherTheAddressExists(t *testing.T) {
	h := newHarness(t)
	res := h.do(t, http.MethodPost, "/auth/password-reset",
		map[string]string{"email": "nobody@uwaterloo.ca"})

	// Same 202 as the success path: anything else would reveal who has an account.
	if res.StatusCode != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", res.StatusCode)
	}
	if _, sent := h.mail.last(); sent {
		t.Error("no email should be sent for an unknown address")
	}
}

// ------------------------------------------------------------------------ csrf

func TestUnsafeRequestWithSessionRequiresCSRFHeader(t *testing.T) {
	h := newHarness(t)
	h.registerAndVerify(t)
	if res := h.do(t, http.MethodPost, "/auth/login",
		map[string]string{"email": testEmail, "password": testPassword}); res.StatusCode != http.StatusOK {
		t.Fatalf("login: status %d", res.StatusCode)
	}

	// Same cookies, no CSRF header — what a cross-site form post would look like.
	req, err := http.NewRequest(http.MethodPost, h.server.URL+"/auth/logout", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	for _, c := range h.client.Jar.Cookies(req.URL) {
		req.AddCookie(c)
	}

	res, err := (&http.Client{}).Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 without the CSRF header", res.StatusCode)
	}
	if decode[apierror.Envelope](t, res).Code != apierror.CodeForbidden {
		t.Error("expected the forbidden code in the standard envelope")
	}
}

func TestUnsafeRequestWithoutSessionSkipsCSRF(t *testing.T) {
	h := newHarness(t)
	// Registration carries no ambient authority, so there is nothing to ride and
	// no CSRF token to demand.
	if res := h.do(t, http.MethodPost, "/auth/register", registerBody(nil)); res.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201", res.StatusCode)
	}
}
