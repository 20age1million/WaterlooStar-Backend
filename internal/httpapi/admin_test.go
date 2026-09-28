package httpapi_test

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/20age1million/WaterlooStar-Backend/internal/db"
)

// The /admin surface in Phase 9: the guard, and the read-only endpoints behind
// it. The query tests in internal/db prove the SQL; these prove who gets in.

var adminPaths = []string{
	"/admin/overview",
	"/admin/users",
	"/admin/users/3f7c1b2a-9d4e-4c8a-8f1b-2e3d4c5b6a70",
	"/admin/actions",
}

// promoteAndRelogin makes the harness's signed-in account an admin the way the
// host does — through db.ChangeRole — then logs in again, because the role
// travels in the access token and ChangeRole ends the old session.
func promoteAndRelogin(t *testing.T, h *harness, email string) string {
	t.Helper()
	user, err := h.db.GetUserByEmail(context.Background(), email)
	if err != nil {
		t.Fatalf("find %s: %v", email, err)
	}
	if _, err := db.ChangeRole(context.Background(), db.Direct(h.db), nil, user.ID, "admin", "Test operator"); err != nil {
		t.Fatalf("promote: %v", err)
	}
	if res := h.do(t, http.MethodPost, "/auth/login",
		map[string]string{"email": email, "password": testPassword}); res.StatusCode != http.StatusOK {
		t.Fatalf("login as admin: %d", res.StatusCode)
	}
	return user.ID.String()
}

type errorEnvelope struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
}

var requestIDField = regexp.MustCompile(`"request_id":"[^"]*"`)

// assertLooksUnrouted checks a refusal is indistinguishable from a path that
// does not exist — the same status, headers and bytes as the router's NoRoute,
// apart from the path it names and the request id. Comparing the decoded JSON
// alone is not enough: the first version of the guard matched it and still
// differed in Content-Type and a trailing newline.
func assertLooksUnrouted(t *testing.T, h *harness, path string) {
	t.Helper()
	const missing = "/admin-no-such-path"

	read := func(p string) (*http.Response, string) {
		t.Helper()
		res := h.do(t, http.MethodGet, p, nil)
		body, err := io.ReadAll(res.Body)
		if err != nil {
			t.Fatalf("read %s: %v", p, err)
		}
		return res, string(body)
	}

	res, body := read(path)
	ref, refBody := read(missing)

	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("GET %s = %d, want 404", path, res.StatusCode)
	}
	if got, want := res.Header.Get("Content-Type"), ref.Header.Get("Content-Type"); got != want {
		t.Errorf("GET %s Content-Type = %q, an unknown path gives %q", path, got, want)
	}
	if !requestIDField.MatchString(body) {
		t.Errorf("GET %s has no request id, where an unknown path has one", path)
	}

	normalise := func(s, p string) string {
		return strings.Replace(requestIDField.ReplaceAllString(s, `"request_id":""`), p, "PATH", 1)
	}
	if got, want := normalise(body, path), normalise(refBody, missing); got != want {
		t.Errorf("GET %s body differs from an unknown path's:\n got:  %q\n want: %q", path, got, want)
	}
}

func TestAdminSurfaceIsInvisibleToAnonymousCallers(t *testing.T) {
	h := newHarness(t)
	for _, path := range adminPaths {
		assertLooksUnrouted(t, h, path)
	}
}

func TestAdminSurfaceIsInvisibleToOrdinaryUsers(t *testing.T) {
	h, _ := signedIn(t, true)
	for _, path := range adminPaths {
		assertLooksUnrouted(t, h, path)
	}
}

type adminUserBody struct {
	ID           string  `json:"id"`
	Email        string  `json:"email"`
	Username     string  `json:"username"`
	Role         string  `json:"role"`
	Verified     bool    `json:"verified"`
	SuspendedAt  *string `json:"suspended_at"`
	ListingCount int     `json:"listing_count"`
}

type adminUserPage struct {
	Data []adminUserBody `json:"data"`
	Meta struct {
		Total int `json:"total"`
	} `json:"meta"`
}

func TestAdminCanSearchAndFilterAccounts(t *testing.T) {
	h, email := signedIn(t, true)

	// A second, unverified account to filter against.
	if res := h.do(t, http.MethodPost, "/auth/register", map[string]string{
		"email": "quiet.student@uwaterloo.ca", "username": "quietone", "password": testPassword,
	}); res.StatusCode != http.StatusCreated {
		t.Fatalf("register second account: %d", res.StatusCode)
	}

	promoteAndRelogin(t, h, email)

	cases := []struct {
		query string
		want  []string
	}{
		{"", []string{"quietone", "poster"}},
		{"?q=QUIET", []string{"quietone"}},      // a fragment, case-insensitive
		{"?q=poster%40uw", []string{"poster"}},  // part of an email address
		{"?role=admin", []string{"poster"}},
		{"?verified=false", []string{"quietone"}},
		{"?suspended=true", nil},
		{"?per_page=1", []string{"quietone"}}, // newest first
	}
	for _, tc := range cases {
		res := h.do(t, http.MethodGet, "/admin/users"+tc.query, nil)
		if res.StatusCode != http.StatusOK {
			t.Fatalf("GET /admin/users%s = %d", tc.query, res.StatusCode)
		}
		page := decode[adminUserPage](t, res)
		var got []string
		for _, u := range page.Data {
			got = append(got, u.Username)
		}
		if strings.Join(got, ",") != strings.Join(tc.want, ",") {
			t.Errorf("GET /admin/users%s = %v, want %v", tc.query, got, tc.want)
		}
	}

	// The total describes the whole match, not the page.
	page := decode[adminUserPage](t, h.do(t, http.MethodGet, "/admin/users?per_page=1", nil))
	if page.Meta.Total != 2 {
		t.Errorf("total = %d, want 2", page.Meta.Total)
	}

	// The operator's shape carries the email; the public User schema would not.
	for _, u := range page.Data {
		if u.Email == "" {
			t.Errorf("admin account %s has no email", u.Username)
		}
	}
}

func TestAdminOverviewCounts(t *testing.T) {
	h, email := signedIn(t, true)
	promoteAndRelogin(t, h, email)

	res := h.do(t, http.MethodGet, "/admin/overview", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /admin/overview = %d", res.StatusCode)
	}
	body := decode[struct {
		Users struct {
			Total, Verified, Suspended, Admins int
		} `json:"users"`
		ActionsLastWeek int `json:"actions_last_week"`
	}](t, res)

	if body.Users.Total != 1 || body.Users.Verified != 1 || body.Users.Admins != 1 || body.Users.Suspended != 0 {
		t.Errorf("users = %+v", body.Users)
	}
	if body.ActionsLastWeek != 1 {
		t.Errorf("actions_last_week = %d, want 1 (the promotion)", body.ActionsLastWeek)
	}
}

type ledgerEntry struct {
	Action      string         `json:"action"`
	SubjectType string         `json:"subject_type"`
	SubjectID   string         `json:"subject_id"`
	Reason      string         `json:"reason"`
	Detail      map[string]any `json:"detail"`
	Actor       *struct {
		Username string `json:"username"`
	} `json:"actor"`
}

func TestAdminAccountDetailCarriesItsLedger(t *testing.T) {
	h, email := signedIn(t, true)
	id := promoteAndRelogin(t, h, email)

	res := h.do(t, http.MethodGet, "/admin/users/"+id, nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /admin/users/{id} = %d", res.StatusCode)
	}
	body := decode[struct {
		User    adminUserBody `json:"user"`
		Actions []ledgerEntry `json:"actions"`
	}](t, res)

	if body.User.Role != "admin" || body.User.SuspendedAt != nil {
		t.Errorf("user = %+v", body.User)
	}
	if len(body.Actions) != 1 {
		t.Fatalf("actions = %d, want the one promotion", len(body.Actions))
	}
	a := body.Actions[0]
	if a.Action != "set_role" || a.SubjectType != "user" || a.SubjectID != id || a.Reason != "Test operator" {
		t.Errorf("entry = %+v", a)
	}
	if a.Detail["from"] != "user" || a.Detail["to"] != "admin" {
		t.Errorf("detail = %v, want from user to admin", a.Detail)
	}
	// Made from the host, so there is no actor — an explicit null, not an omission.
	if a.Actor != nil {
		t.Errorf("actor = %+v, want null for a change made with cmd/admin", a.Actor)
	}
}

func TestAdminUnknownAccountIs404(t *testing.T) {
	h, email := signedIn(t, true)
	promoteAndRelogin(t, h, email)

	res := h.do(t, http.MethodGet, "/admin/users/3f7c1b2a-9d4e-4c8a-8f1b-2e3d4c5b6a70", nil)
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", res.StatusCode)
	}
	if got := decode[errorEnvelope](t, res); got.Message != "No account with that id." {
		t.Errorf("message = %q", got.Message)
	}
}

func TestAdminLedgerPage(t *testing.T) {
	h, email := signedIn(t, true)
	promoteAndRelogin(t, h, email)

	res := h.do(t, http.MethodGet, "/admin/actions", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /admin/actions = %d", res.StatusCode)
	}
	page := decode[struct {
		Data []ledgerEntry `json:"data"`
		Meta struct {
			Total int `json:"total"`
		} `json:"meta"`
	}](t, res)
	if page.Meta.Total != 1 || len(page.Data) != 1 || page.Data[0].Action != "set_role" {
		t.Errorf("ledger = %+v", page)
	}
}

// Nothing may rewrite the ledger. The guarantee is an absence — no UPDATE and
// no DELETE against admin_actions anywhere in the generated query set — so the
// test reads the generated SQL for one.
func TestLedgerHasNoUpdateOrDelete(t *testing.T) {
	dir := filepath.Join("..", "db", "sqlcgen")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}

	rewrite := regexp.MustCompile(`(?is)\b(update\s+admin_actions|delete\s+from\s+admin_actions)\b`)
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		src, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		if m := rewrite.Find(src); m != nil {
			t.Errorf("%s contains %q. admin_actions is append-only: record a correcting action instead of editing history.", e.Name(), m)
		}
	}
}
