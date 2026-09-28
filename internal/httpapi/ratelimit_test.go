package httpapi_test

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/20age1million/WaterlooStar-Backend/internal/apierror"
	"github.com/20age1million/WaterlooStar-Backend/internal/auth"
	"github.com/20age1million/WaterlooStar-Backend/internal/config"
	"github.com/20age1million/WaterlooStar-Backend/internal/httpapi"
	"github.com/20age1million/WaterlooStar-Backend/internal/ratelimit"
)

// Rate limiting, through the whole stack.
//
// The limiter's arithmetic is unit-tested in internal/ratelimit. What is tested
// here is what a client actually sees: the status, the envelope, the header, and
// the rule that ordinary use never meets a limit.

// testClock is the clock the harness below runs on, so a test can assert that an
// exhausted allowance refills without waiting fifteen real minutes for it.
type testClock struct {
	mu  sync.Mutex
	now time.Time
}

func newTestClock() *testClock {
	return &testClock{now: time.Date(2026, 9, 27, 9, 0, 0, 0, time.UTC)}
}

func (c *testClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *testClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// limitedHarness is newHarness with the clock in the test's hands.
func limitedHarness(t *testing.T) (*harness, *testClock) {
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

	clk := newTestClock()
	db := newFakeQuerier()
	mail := &capturingMailer{}
	srv := httptest.NewServer(httpapi.NewRouter(cfg, log,
		httpapi.NewServerWithLimiter(cfg, log, db, mail, "", ratelimit.New(0, clk.Now))))
	t.Cleanup(srv.Close)

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar: %v", err)
	}
	return &harness{server: srv, db: db, mail: mail, client: &http.Client{Jar: jar}}, clk
}

// doWithHeader is harness.do with one extra header, for the forwarded client
// address the limiter uses as a second dimension.
func (h *harness) doWithHeader(t *testing.T, method, path string, body any, header, value string) *http.Response {
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
	req.Header.Set(header, value)
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

// registered puts an account in the fake database and returns its address.
func registered(t *testing.T, h *harness) string {
	t.Helper()
	body := map[string]string{"email": testEmail, "username": testUsername, "password": testPassword}
	if res := h.do(t, http.MethodPost, "/auth/register", body); res.StatusCode != http.StatusCreated {
		t.Fatalf("register: %d", res.StatusCode)
	}
	return testEmail
}

// assertRefusal checks the one shape every refusal in this API has.
func assertRefusal(t *testing.T, res *http.Response) apierror.Envelope {
	t.Helper()
	if res.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("status %d, want 429", res.StatusCode)
	}

	retry := res.Header.Get("Retry-After")
	if retry == "" {
		t.Error("no Retry-After header, so a client has to guess when to try again")
	} else if seconds, err := strconv.Atoi(retry); err != nil || seconds < 1 {
		t.Errorf("Retry-After is %q, want a positive number of seconds", retry)
	}

	env := decode[apierror.Envelope](t, res)
	if env.Code != apierror.CodeRateLimited {
		t.Errorf("code %q, want %q", env.Code, apierror.CodeRateLimited)
	}
	if env.Message == "" {
		t.Error("no message, and this is one a user will read")
	}
	if env.RequestID == "" {
		t.Error("no request id, so the refusal cannot be found in the log")
	}
	return env
}

func TestFailedLoginsAreThrottledAndThenRefill(t *testing.T) {
	h, clk := limitedHarness(t)
	email := registered(t, h)
	wrong := map[string]string{"email": email, "password": "not the password"}

	burst := ratelimit.LoginPerEmail.Burst
	for i := range burst {
		res := h.do(t, http.MethodPost, "/auth/login", wrong)
		if res.StatusCode != http.StatusUnauthorized {
			t.Fatalf("attempt %d: status %d, want 401", i+1, res.StatusCode)
		}
	}

	// The burst is spent, so the next attempt is refused before the password is
	// even compared.
	assertRefusal(t, h.do(t, http.MethodPost, "/auth/login", wrong))

	// Refusal is not lockout: the correct password is refused too, while the
	// allowance is empty.
	right := map[string]string{"email": email, "password": testPassword}
	assertRefusal(t, h.do(t, http.MethodPost, "/auth/login", right))

	// A full window later, the account is reachable again.
	clk.advance(ratelimit.LoginPerEmail.Window + time.Second)
	if res := h.do(t, http.MethodPost, "/auth/login", right); res.StatusCode != http.StatusOK {
		t.Fatalf("after the window: status %d, want 200 — the allowance did not refill", res.StatusCode)
	}
}

func TestSuccessfulLoginsAreNeverThrottled(t *testing.T) {
	h, _ := limitedHarness(t)
	email := registered(t, h)
	right := map[string]string{"email": email, "password": testPassword}

	// Far past the burst. Only guessing is charged for, which is what keeps the
	// end-to-end suite — and anyone who signs in daily — clear of the limiter.
	for i := range ratelimit.LoginPerEmail.Burst * 4 {
		res := h.do(t, http.MethodPost, "/auth/login", right)
		if res.StatusCode != http.StatusOK {
			t.Fatalf("login %d: status %d, want 200", i+1, res.StatusCode)
		}
	}
}

func TestAWrongPasswordAfterSuccessesStillCosts(t *testing.T) {
	h, _ := limitedHarness(t)
	email := registered(t, h)
	right := map[string]string{"email": email, "password": testPassword}
	wrong := map[string]string{"email": email, "password": "not the password"}

	// Successes must not top the bucket up either: an attacker who knows one
	// password on the site should not be able to buy attempts against another
	// account, and a success is simply not counted in any direction.
	for range 3 {
		h.do(t, http.MethodPost, "/auth/login", right)
	}
	for i := range ratelimit.LoginPerEmail.Burst {
		if res := h.do(t, http.MethodPost, "/auth/login", wrong); res.StatusCode != http.StatusUnauthorized {
			t.Fatalf("failure %d: status %d, want 401", i+1, res.StatusCode)
		}
	}
	assertRefusal(t, h.do(t, http.MethodPost, "/auth/login", wrong))
}

func TestUnknownAddressesAreThrottledIdenticallyToKnownOnes(t *testing.T) {
	h, _ := limitedHarness(t)
	registered(t, h)

	unknown := map[string]string{"email": "nobody@uwaterloo.ca", "password": testPassword}
	for range ratelimit.LoginPerEmail.Burst {
		if res := h.do(t, http.MethodPost, "/auth/login", unknown); res.StatusCode != http.StatusUnauthorized {
			t.Fatal("an unknown address answered something other than 401")
		}
	}
	unknownRefusal := assertRefusal(t, h.do(t, http.MethodPost, "/auth/login", unknown))

	known := map[string]string{"email": testEmail, "password": "not the password"}
	for range ratelimit.LoginPerEmail.Burst {
		h.do(t, http.MethodPost, "/auth/login", known)
	}
	knownRefusal := assertRefusal(t, h.do(t, http.MethodPost, "/auth/login", known))

	// If the two differed, 429 would be an account-enumeration oracle: ask once
	// too often and the answer tells you whether the address is registered.
	if unknownRefusal.Message != knownRefusal.Message {
		t.Errorf("refusals differ:\n unknown: %q\n known:   %q",
			unknownRefusal.Message, knownRefusal.Message)
	}
}

func TestRegistrationIsThrottledPerAddress(t *testing.T) {
	h, _ := limitedHarness(t)
	body := map[string]string{"email": "repeat@uwaterloo.ca", "username": "repeat", "password": testPassword}

	// The first succeeds; the rest collide on the address. Either way each one
	// is an attempt and each one is charged.
	for range ratelimit.RegisterPerEmail.Burst {
		res := h.do(t, http.MethodPost, "/auth/register", body)
		if res.StatusCode != http.StatusCreated && res.StatusCode != http.StatusConflict {
			t.Fatalf("status %d, want 201 or 409", res.StatusCode)
		}
	}
	assertRefusal(t, h.do(t, http.MethodPost, "/auth/register", body))
}

func TestPasswordResetRequestsAreThrottledPerAddress(t *testing.T) {
	h, _ := limitedHarness(t)
	email := registered(t, h)
	body := map[string]string{"email": email}

	for range ratelimit.ResetRequestPerEmail.Burst {
		if res := h.do(t, http.MethodPost, "/auth/password-reset", body); res.StatusCode != http.StatusAccepted {
			t.Fatalf("status %d, want 202", res.StatusCode)
		}
	}
	// Uncapped, this endpoint sends mail to someone else's inbox on demand.
	assertRefusal(t, h.do(t, http.MethodPost, "/auth/password-reset", body))
}

func TestTokenRedemptionIsThrottledPerToken(t *testing.T) {
	h, _ := limitedHarness(t)
	registered(t, h)

	body := map[string]string{"token": "a-token-that-does-not-exist"}
	for range ratelimit.TokenPerToken.Burst {
		if res := h.do(t, http.MethodPost, "/auth/verify", body); res.StatusCode != http.StatusBadRequest {
			t.Fatalf("status %d, want 400", res.StatusCode)
		}
	}
	assertRefusal(t, h.do(t, http.MethodPost, "/auth/verify", body))
}

func TestGlobalBackstopCatchesAVariedIdentity(t *testing.T) {
	h, _ := limitedHarness(t)

	// Every attempt here uses a different address, so no per-address bucket is
	// ever spent twice. Without the service-wide backstop this loop would run
	// forever at full speed.
	refused := false
	for i := range ratelimit.TokenGlobal.Burst + 5 {
		res := h.do(t, http.MethodPost, "/auth/verify",
			map[string]string{"token": "guess-" + strconv.Itoa(i)})
		if res.StatusCode == http.StatusTooManyRequests {
			refused = true
			break
		}
	}
	if !refused {
		t.Fatalf("varying the token stayed unlimited past the backstop of %d",
			ratelimit.TokenGlobal.Burst)
	}
}

func TestRefreshIsNeverThrottled(t *testing.T) {
	h, _ := limitedHarness(t)
	email := registered(t, h)
	if res := h.do(t, http.MethodPost, "/auth/login",
		map[string]string{"email": email, "password": testPassword}); res.StatusCode != http.StatusOK {
		t.Fatalf("login: %d", res.StatusCode)
	}

	// The frontend refreshes on navigation, so a browsing session calls this at a
	// rate no human types at. Throttling it would sign people out for using the
	// site — deliberately unlimited, and asserted so nobody adds it by reflex.
	for i := range 80 {
		res := h.do(t, http.MethodPost, "/auth/refresh", nil)
		if res.StatusCode == http.StatusTooManyRequests {
			t.Fatalf("refresh %d was throttled", i+1)
		}
	}
}

func TestPostingIsThrottledPerUser(t *testing.T) {
	h, clk := limitedHarness(t)

	// Own registration and verification, because this harness owns the clock.
	email := "poster@uwaterloo.ca"
	if res := h.do(t, http.MethodPost, "/auth/register",
		map[string]string{"email": email, "username": "poster", "password": testPassword}); res.StatusCode != http.StatusCreated {
		t.Fatalf("register: %d", res.StatusCode)
	}
	if res := h.do(t, http.MethodPost, "/auth/verify",
		map[string]string{"token": h.mail.token(t)}); res.StatusCode != http.StatusOK {
		t.Fatalf("verify: %d", res.StatusCode)
	}
	if res := h.do(t, http.MethodPost, "/auth/login",
		map[string]string{"email": email, "password": testPassword}); res.StatusCode != http.StatusOK {
		t.Fatalf("login: %d", res.StatusCode)
	}

	for i := range ratelimit.WritePerUser.Burst {
		res, _ := createListing(t, h, validListing())
		if res.StatusCode != http.StatusCreated {
			t.Fatalf("listing %d: status %d, want 201", i+1, res.StatusCode)
		}
	}

	// Sent with do rather than createListing, which reads the body itself and
	// would leave nothing for the envelope assertion.
	assertRefusal(t, h.do(t, http.MethodPost, "/listings", validListing()))

	// And it refills, because a prolific poster is not a permanent offender.
	clk.advance(ratelimit.WritePerUser.Window + time.Second)
	res, _ := createListing(t, h, validListing())
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("after the window: status %d, want 201", res.StatusCode)
	}
}

func TestBrowsingAndReadingAreNeverThrottled(t *testing.T) {
	h, _ := limitedHarness(t)

	// Reads are not in the allowlist, and this is the test that fails if someone
	// adds them without meaning to.
	for i := range 120 {
		if res := h.do(t, http.MethodGet, "/listings", nil); res.StatusCode == http.StatusTooManyRequests {
			t.Fatalf("GET /listings was throttled on request %d", i+1)
		}
		if res := h.do(t, http.MethodGet, "/requests", nil); res.StatusCode == http.StatusTooManyRequests {
			t.Fatalf("GET /requests was throttled on request %d", i+1)
		}
	}
}

func TestClientIPSeparatesCallersBehindOneFrontend(t *testing.T) {
	h, _ := limitedHarness(t)
	registered(t, h)
	wrong := map[string]any{"email": testEmail, "password": "not the password"}

	// Every request to this API arrives from the frontend's address, so the
	// forwarded client address is the only thing that can tell two people apart.
	spend := func(ip string) *http.Response {
		return h.doWithHeader(t, http.MethodPost, "/auth/login", wrong, httpapi.ClientIPHeader, ip)
	}

	for range ratelimit.LoginPerEmail.Burst {
		if res := spend("203.0.113.7"); res.StatusCode != http.StatusUnauthorized {
			t.Fatal("expected 401 while the allowance lasts")
		}
	}
	assertRefusal(t, spend("203.0.113.7"))

	// A different person, guessing at the same account, has their own allowance.
	// The global backstop is what bounds them together.
	if res := spend("198.51.100.4"); res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("a second client address was refused for the first one's spending: %d", res.StatusCode)
	}
}
