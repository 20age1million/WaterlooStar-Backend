package httpapi_test

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/20age1million/WaterlooStar-Backend/internal/apierror"
	"github.com/20age1million/WaterlooStar-Backend/internal/config"
	"github.com/20age1million/WaterlooStar-Backend/internal/httpapi"
	"github.com/20age1million/WaterlooStar-Backend/internal/middleware"
)

// newTestServer builds a server over the in-memory fake defined in
// fake_querier_test.go, so the health handler's failure path can be exercised
// without standing up PostgreSQL.
func newTestServer(t *testing.T, q *fakeQuerier) *httptest.Server {
	t.Helper()
	gin.SetMode(gin.TestMode)

	cfg := config.Config{
		Port:        8080,
		CORSOrigin:  "http://localhost:3000",
		Environment: config.Development,
		JWTSecret:   "a-test-secret-that-is-long-enough-to-pass",
	}
	// Discard log output so a deliberately failing case does not litter test output.
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	srv := httptest.NewServer(httpapi.NewRouter(cfg, log,
		httpapi.NewServerWithQuerier(cfg, log, q, &capturingMailer{}, "")))
	t.Cleanup(srv.Close)
	return srv
}

type healthBody struct {
	Status   string  `json:"status"`
	Database string  `json:"database"`
	Version  *string `json:"version"`
}

func TestHealthReportsOKWhenDatabaseResponds(t *testing.T) {
	srv := newTestServer(t, newFakeQuerier())

	res, err := http.Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}

	var body healthBody
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Status != "ok" {
		t.Errorf("status = %q, want \"ok\"", body.Status)
	}
	if body.Database != "ok" {
		t.Errorf("database = %q, want \"ok\"", body.Database)
	}
	if body.Version != nil {
		t.Errorf("version should be omitted when the binary is unstamped, got %q", *body.Version)
	}
}

func TestHealthReports503WhenDatabaseUnreachable(t *testing.T) {
	srv := newTestServer(t, &fakeQuerier{pingErr: errors.New("connection refused")})

	res, err := http.Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}
	defer res.Body.Close()

	// 503 rather than 500: the service is running, its dependency is not, and a
	// load balancer should take this instance out of rotation.
	if res.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", res.StatusCode)
	}

	var body healthBody
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Status != "degraded" {
		t.Errorf("status = %q, want \"degraded\"", body.Status)
	}
	if body.Database != "unreachable" {
		t.Errorf("database = %q, want \"unreachable\"", body.Database)
	}
}

func TestUnknownPathReturnsStandardErrorEnvelope(t *testing.T) {
	srv := newTestServer(t, newFakeQuerier())

	res, err := http.Get(srv.URL + "/no-such-endpoint")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", res.StatusCode)
	}

	var env apierror.Envelope
	if err := json.NewDecoder(res.Body).Decode(&env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// gin's default 404 is a bare string. Every failure in this service must use
	// the documented envelope instead, or the frontend has two contracts.
	if env.Code != apierror.CodeNotFound {
		t.Errorf("code = %q, want %q", env.Code, apierror.CodeNotFound)
	}
	if env.Message == "" {
		t.Error("message should not be empty")
	}
	if env.RequestID == "" {
		t.Error("request_id should be populated so a user can quote it")
	}
}

func TestResponseCarriesRequestID(t *testing.T) {
	srv := newTestServer(t, newFakeQuerier())

	t.Run("generated when absent", func(t *testing.T) {
		res, err := http.Get(srv.URL + "/healthz")
		if err != nil {
			t.Fatalf("GET: %v", err)
		}
		defer res.Body.Close()

		if res.Header.Get(middleware.RequestIDHeader) == "" {
			t.Error("response should carry a generated request id")
		}
	})

	t.Run("inbound id is reused", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, srv.URL+"/healthz", nil)
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		const given = "11111111-2222-3333-4444-555555555555"
		req.Header.Set(middleware.RequestIDHeader, given)

		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("do: %v", err)
		}
		defer res.Body.Close()

		if got := res.Header.Get(middleware.RequestIDHeader); got != given {
			t.Errorf("request id = %q, want the inbound %q", got, given)
		}
	})
}

func TestCORSAllowsConfiguredOriginWithCredentials(t *testing.T) {
	srv := newTestServer(t, newFakeQuerier())

	req, err := http.NewRequest(http.MethodOptions, srv.URL+"/healthz", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Origin", "http://localhost:3000")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("preflight: %v", err)
	}
	defer res.Body.Close()

	if got := res.Header.Get("Access-Control-Allow-Origin"); got != "http://localhost:3000" {
		t.Errorf("allow-origin = %q, want the configured origin echoed exactly", got)
	}
	// The session JWT travels as a cookie, so credentials must be allowed — and
	// a wildcard origin is invalid alongside them.
	if got := res.Header.Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Errorf("allow-credentials = %q, want \"true\"", got)
	}
}

func TestCORSIgnoresUnknownOrigin(t *testing.T) {
	srv := newTestServer(t, newFakeQuerier())

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/healthz", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Origin", "https://evil.example")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer res.Body.Close()

	if got := res.Header.Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("allow-origin = %q, want empty for an unconfigured origin", got)
	}
}
