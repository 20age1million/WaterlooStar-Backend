package httpapi_test

import (
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/20age1million/WaterlooStar-Backend/internal/db/sqlcgen"
	"github.com/20age1million/WaterlooStar-Backend/internal/ratelimit"
)

// POST /auth/verify/resend: a new confirmation link for an unverified account,
// and the same answer for every address.

func resend(t *testing.T, h *harness, address string) (int, string) {
	t.Helper()
	res := h.do(t, http.MethodPost, "/auth/verify/resend", map[string]string{"email": address})
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return res.StatusCode, string(body)
}

func TestResendSendsANewLinkAndSpendsTheOld(t *testing.T) {
	h := newHarness(t)
	if res := h.do(t, http.MethodPost, "/auth/register", registerBody(nil)); res.StatusCode != http.StatusCreated {
		t.Fatalf("register: status %d", res.StatusCode)
	}
	oldToken := h.mail.token(t)

	if status, _ := resend(t, h, testEmail); status != http.StatusAccepted {
		t.Fatalf("resend: status %d, want 202", status)
	}
	if n := h.mail.count(); n != 2 {
		t.Fatalf("%d emails sent, want the confirmation and one new link", n)
	}
	msg, _ := h.mail.last()
	if msg.To != testEmail || msg.Subject != "Confirm your WaterlooStar account" {
		t.Errorf("sent %q to %q, want a confirmation to %q", msg.Subject, msg.To, testEmail)
	}
	newToken := h.mail.token(t)
	if newToken == oldToken {
		t.Fatal("the new link carries the old token")
	}

	// The old link is spent; the new one verifies.
	if res := h.do(t, http.MethodPost, "/auth/verify", map[string]string{"token": oldToken}); res.StatusCode != http.StatusBadRequest {
		t.Errorf("old link: status %d, want 400", res.StatusCode)
	}
	if res := h.do(t, http.MethodPost, "/auth/verify", map[string]string{"token": newToken}); res.StatusCode != http.StatusOK {
		t.Errorf("new link: status %d, want 200", res.StatusCode)
	}
}

func TestResendAddressMatchesCaseInsensitively(t *testing.T) {
	h := newHarness(t)
	if res := h.do(t, http.MethodPost, "/auth/register", registerBody(nil)); res.StatusCode != http.StatusCreated {
		t.Fatalf("register: status %d", res.StatusCode)
	}

	if status, _ := resend(t, h, "  MEIL@UWaterloo.ca "); status != http.StatusAccepted {
		t.Fatalf("status %d, want 202", status)
	}
	if n := h.mail.count(); n != 2 {
		t.Errorf("%d emails sent, want a new link for the same account", n)
	}
}

func TestResendSendsNothingOutsideAnUnverifiedAccount(t *testing.T) {
	cases := []struct {
		name  string
		setup func(t *testing.T, h *harness) string
	}{
		{"an unknown address", func(t *testing.T, h *harness) string {
			return "nobody@uwaterloo.ca"
		}},
		{"a verified account", func(t *testing.T, h *harness) string {
			h.registerAndVerify(t)
			return testEmail
		}},
		{"a suspended account", func(t *testing.T, h *harness) string {
			if res := h.do(t, http.MethodPost, "/auth/register", registerBody(nil)); res.StatusCode != http.StatusCreated {
				t.Fatalf("register: status %d", res.StatusCode)
			}
			user, err := h.db.GetUserByEmail(context.Background(), testEmail)
			if err != nil {
				t.Fatalf("load user: %v", err)
			}
			if _, err := h.db.SuspendUser(context.Background(), sqlcgen.SuspendUserParams{ID: user.ID}); err != nil {
				t.Fatalf("suspend: %v", err)
			}
			return testEmail
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			address := tc.setup(t, h)
			before := h.mail.count()

			if status, _ := resend(t, h, address); status != http.StatusAccepted {
				t.Fatalf("status %d, want 202", status)
			}
			if n := h.mail.count(); n != before {
				t.Errorf("%d emails sent, want none", n-before)
			}
		})
	}
}

// Every case answers alike, or the endpoint says who has an account and who is
// verified.
func TestResendAnswersEveryAddressTheSame(t *testing.T) {
	h := newHarness(t)
	if res := h.do(t, http.MethodPost, "/auth/register", registerBody(nil)); res.StatusCode != http.StatusCreated {
		t.Fatalf("register: status %d", res.StatusCode)
	}
	verified := registerBody(map[string]string{"email": "danielo@uwaterloo.ca", "username": "danielo"})
	if res := h.do(t, http.MethodPost, "/auth/register", verified); res.StatusCode != http.StatusCreated {
		t.Fatalf("register: status %d", res.StatusCode)
	}
	if res := h.do(t, http.MethodPost, "/auth/verify", map[string]string{"token": h.mail.token(t)}); res.StatusCode != http.StatusOK {
		t.Fatalf("verify: status %d", res.StatusCode)
	}

	wantStatus, wantBody := resend(t, h, testEmail)
	for _, address := range []string{"danielo@uwaterloo.ca", "nobody@uwaterloo.ca"} {
		status, body := resend(t, h, address)
		if status != wantStatus || body != wantBody {
			t.Errorf("%s: %d %q, want %d %q like an unverified account", address, status, body, wantStatus, wantBody)
		}
	}

	// A send that fails answers alike too.
	h.mail.failing(true)
	if status, body := resend(t, h, testEmail); status != wantStatus || body != wantBody {
		t.Errorf("failed send: %d %q, want %d %q", status, body, wantStatus, wantBody)
	}
}

func TestResendRejectsAMalformedRequest(t *testing.T) {
	h := newHarness(t)
	for _, body := range []any{
		map[string]string{"email": "not an address"},
		map[string]string{},
	} {
		res := h.do(t, http.MethodPost, "/auth/verify/resend", body)
		if res.StatusCode != http.StatusBadRequest {
			t.Errorf("%v: status %d, want 400", body, res.StatusCode)
		}
	}
}

func TestResendIsThrottledPerAddress(t *testing.T) {
	h, _ := limitedHarness(t)
	email := registered(t, h)
	body := map[string]string{"email": email}

	for range ratelimit.ResendPerEmail.Burst {
		if res := h.do(t, http.MethodPost, "/auth/verify/resend", body); res.StatusCode != http.StatusAccepted {
			t.Fatalf("status %d, want 202", res.StatusCode)
		}
	}
	assertRefusal(t, h.do(t, http.MethodPost, "/auth/verify/resend", body))

	// Another address has its own allowance.
	other := map[string]string{"email": "someone-else@uwaterloo.ca"}
	if res := h.do(t, http.MethodPost, "/auth/verify/resend", other); res.StatusCode != http.StatusAccepted {
		t.Errorf("another address: status %d, want 202", res.StatusCode)
	}
}
