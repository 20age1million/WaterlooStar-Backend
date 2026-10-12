package email

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/mail"
	"strings"
	"testing"
	"time"
)

// No test here reaches Resend: every sender points at an httptest server.

const testKey = "re_never_a_real_key"

var testFrom = mail.Address{Name: "WaterlooStar", Address: "no-reply@waterloostar.com"}

// testMessage carries a link, as every real message does, so the tests can check
// that it never leaks into an error.
func testMessage() Message {
	return VerificationMessage("verify-0123abcd", "student@uwaterloo.ca", "https://waterloostar.com/verify/raw-token-secret")
}

type capturedRequest struct {
	method, path, auth, idempotencyKey, contentType string
	body                                            map[string]any
}

func fakeResend(t *testing.T, status int, contentType, response string) (*httptest.Server, *capturedRequest) {
	t.Helper()
	got := &capturedRequest{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.method, got.path = r.Method, r.URL.Path
		got.auth = r.Header.Get("Authorization")
		got.idempotencyKey = r.Header.Get("Idempotency-Key")
		got.contentType = r.Header.Get("Content-Type")
		raw, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(raw, &got.body); err != nil {
			t.Errorf("request body is not JSON: %v", err)
		}
		w.Header().Set("Content-Type", contentType)
		w.WriteHeader(status)
		_, _ = io.WriteString(w, response)
	}))
	t.Cleanup(srv.Close)
	return srv, got
}

func TestResendSenderSends(t *testing.T) {
	srv, got := fakeResend(t, http.StatusOK, "application/json", `{"id":"49a3999c-0ce1-4ea6-ab68-afcd6dc2e794"}`)
	sender := newResendSender(testKey, testFrom, srv.URL, time.Second)

	if err := sender.Send(context.Background(), testMessage()); err != nil {
		t.Fatalf("Send: %v", err)
	}

	if got.method != http.MethodPost || got.path != "/emails" {
		t.Errorf("request = %s %s, want POST /emails", got.method, got.path)
	}
	if got.auth != "Bearer "+testKey {
		t.Errorf("Authorization = %q, want the key as a bearer token", got.auth)
	}
	if got.idempotencyKey != "verify-0123abcd" {
		t.Errorf("Idempotency-Key = %q, want the message ID", got.idempotencyKey)
	}
	if got.contentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got.contentType)
	}

	want := map[string]any{
		"from":    `"WaterlooStar" <no-reply@waterloostar.com>`,
		"to":      []any{"student@uwaterloo.ca"},
		"subject": "Confirm your WaterlooStar account",
	}
	for k, v := range want {
		gotJSON, _ := json.Marshal(got.body[k])
		wantJSON, _ := json.Marshal(v)
		if string(gotJSON) != string(wantJSON) {
			t.Errorf("body[%q] = %s, want %s", k, gotJSON, wantJSON)
		}
	}
	text, _ := got.body["text"].(string)
	if !strings.Contains(text, "https://waterloostar.com/verify/raw-token-secret") {
		t.Errorf("text should carry the link, got %q", text)
	}
	if _, ok := got.body["html"]; ok {
		t.Error("body has an html part; messages are plain text")
	}
}

func TestResendSenderSendsABareAddress(t *testing.T) {
	srv, got := fakeResend(t, http.StatusOK, "application/json", `{"id":"x"}`)
	sender := newResendSender(testKey, mail.Address{Address: "no-reply@waterloostar.com"}, srv.URL, time.Second)

	if err := sender.Send(context.Background(), testMessage()); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if got.body["from"] != "<no-reply@waterloostar.com>" {
		t.Errorf("from = %v, want the address alone", got.body["from"])
	}
}

func TestResendSenderErrors(t *testing.T) {
	cases := []struct {
		name        string
		status      int
		contentType string
		response    string
		want        []string
	}{
		{
			name:        "a rejection names Resend's error and its message",
			status:      http.StatusForbidden,
			contentType: "application/json",
			response:    `{"statusCode":403,"name":"validation_error","message":"The waterloostar.com domain is not verified. Please, add and verify your domain on https://resend.com/domains"}`,
			want:        []string{"403", "validation_error", "domain is not verified", "[link]"},
		},
		{
			// Resend's message can quote the request.
			name:        "a quoted recipient, link or key is redacted",
			status:      http.StatusUnprocessableEntity,
			contentType: "application/json",
			response:    `{"statusCode":422,"name":"validation_error","message":"Invalid to: student@uwaterloo.ca near https://waterloostar.com/verify/raw-token-secret with re_never_a_real_key"}`,
			want:        []string{"422", "validation_error", "[redacted]", "[link]"},
		},
		{
			name:        "an outage names the status",
			status:      http.StatusInternalServerError,
			contentType: "application/json",
			response:    `{"statusCode":500,"name":"internal_server_error","message":"An unexpected error occurred."}`,
			want:        []string{"500", "internal_server_error"},
		},
		{
			// It could echo anything, so only the status is kept.
			name:        "an unparseable answer is not passed through",
			status:      http.StatusBadGateway,
			contentType: "text/html",
			response:    `<html>raw-token-secret student@uwaterloo.ca ` + testKey + `</html>`,
			want:        []string{"status 502"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, _ := fakeResend(t, tc.status, tc.contentType, tc.response)
			sender := newResendSender(testKey, testFrom, srv.URL, time.Second)

			err := sender.Send(context.Background(), testMessage())
			if err == nil {
				t.Fatal("expected an error, got nil")
			}
			for _, w := range tc.want {
				if !strings.Contains(err.Error(), w) {
					t.Errorf("error should contain %q, got: %v", w, err)
				}
			}
			assertNothingSecret(t, err)
		})
	}
}

func TestResendSenderTimesOut(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(func() { close(release); srv.Close() })
	sender := newResendSender(testKey, testFrom, srv.URL, 50*time.Millisecond)

	start := time.Now()
	err := sender.Send(context.Background(), testMessage())
	if err == nil {
		t.Fatal("expected a timeout, got nil")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("Send took %s; the timeout is 50ms", elapsed)
	}
	if !strings.Contains(err.Error(), "no answer within") {
		t.Errorf("error should say Resend did not answer, got: %v", err)
	}
	assertNothingSecret(t, err)
}

func TestResendSenderUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()
	sender := newResendSender(testKey, testFrom, url, time.Second)

	err := sender.Send(context.Background(), testMessage())
	if err == nil || !strings.Contains(err.Error(), "request failed") {
		t.Fatalf("error = %v, want a failed request", err)
	}
	assertNothingSecret(t, err)
}

func TestMessageID(t *testing.T) {
	hash := []byte("stored-token-hash")
	id := MessageID("verify", hash)

	if !strings.HasPrefix(id, "verify-") || len(id) != len("verify-")+32 {
		t.Errorf("MessageID = %q, want verify- and 32 hex characters", id)
	}
	if id != MessageID("verify", hash) {
		t.Error("MessageID is not stable for the same token")
	}
	if id == MessageID("verify", []byte("another-token-hash")) {
		t.Error("two tokens share a message ID")
	}
	if strings.Contains(id, "73746f726564") { // hex of "stored"
		t.Error("MessageID exposes the stored hash")
	}
	for _, r := range id {
		if !(r == '-' || r == '_' || (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')) {
			t.Fatalf("MessageID %q has %q, which is not safe in an idempotency key", id, r)
		}
	}
}

func assertNothingSecret(t *testing.T, err error) {
	t.Helper()
	for _, secret := range []string{testKey, "raw-token-secret", "student@uwaterloo.ca"} {
		if strings.Contains(err.Error(), secret) {
			t.Errorf("error leaks %q: %v", secret, err)
		}
	}
}
