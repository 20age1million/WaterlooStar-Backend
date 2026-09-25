package httpapi_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/20age1million/WaterlooStar-Backend/internal/apierror"
)

// "Looking for Housing", through the handlers.
//
// The same two rules as listings are tested hardest — only a verified student
// may post, and someone else's request answers 404 — plus the filters, whose
// meaning is inverted here: a budget is a ceiling and a distance is a radius.

type requestBody struct {
	ID            string `json:"id"`
	Title         string `json:"title"`
	BudgetCents   int    `json:"budget_cents"`
	Occupants     int    `json:"occupants"`
	Status        string `json:"status"`
	Offers        int    `json:"offers"`
	MaxDistanceM  *int   `json:"max_distance_m"`
	Neighbourhood string `json:"neighbourhood"`
	Poster        struct {
		Username string `json:"username"`
		Verified bool   `json:"verified"`
	} `json:"poster"`
}

type requestPageBody struct {
	Data []requestBody `json:"data"`
	Meta struct {
		Page       int `json:"page"`
		PerPage    int `json:"per_page"`
		Total      int `json:"total"`
		TotalPages int `json:"total_pages"`
	} `json:"meta"`
}

func validRequest() map[string]any {
	return map[string]any{
		"title":          "Quiet room within walking distance, Winter term",
		"body":           "Coming back from a co-op in Toronto. I study late and sleep light.",
		"budget_cents":   90000,
		"start_date":     "2027-01-01",
		"end_date":       "2027-04-30",
		"lease_months":   4,
		"term_tag":       "Winter term",
		"occupants":      1,
		"max_distance_m": 2000,
		"neighbourhood":  "Northdale",
	}
}

func postRequest(t *testing.T, h *harness, body map[string]any) (*http.Response, map[string]any) {
	t.Helper()
	res := h.do(t, http.MethodPost, "/requests", body)
	var out map[string]any
	_ = json.NewDecoder(res.Body).Decode(&out)
	return res, out
}

func TestPostingARequestNeedsAVerifiedStudent(t *testing.T) {
	t.Run("anonymous is refused", func(t *testing.T) {
		h := newHarness(t)
		res, _ := postRequest(t, h, validRequest())
		if res.StatusCode != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", res.StatusCode)
		}
	})

	t.Run("signed in but unverified is refused", func(t *testing.T) {
		h, _ := signedIn(t, false)
		res, _ := postRequest(t, h, validRequest())
		// 403, not 401: they are who they say they are, they just have not
		// proved they are a student.
		if res.StatusCode != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", res.StatusCode)
		}
	})

	t.Run("verified succeeds and appears publicly", func(t *testing.T) {
		h, _ := signedIn(t, true)
		res, created := postRequest(t, h, validRequest())
		if res.StatusCode != http.StatusCreated {
			t.Fatalf("status = %d, want 201", res.StatusCode)
		}
		if created["status"] != "published" {
			t.Errorf("status = %v, want published by default", created["status"])
		}

		// Anonymous read: what any visitor sees.
		_, page := getJSON[requestPageBody](t, h.server.URL+"/requests")
		if page.Meta.Total != 1 {
			t.Fatalf("public results show %d requests, want the one just posted", page.Meta.Total)
		}
		if !page.Data[0].Poster.Verified {
			t.Error("the poster should be shown as verified")
		}
	})
}

func TestRequestValidation(t *testing.T) {
	h, _ := signedIn(t, true)

	cases := map[string]struct {
		mutate func(map[string]any)
		field  string
	}{
		"end before start": {func(b map[string]any) { b["end_date"] = "2026-01-01" }, "end_date"},
		"free":             {func(b map[string]any) { b["budget_cents"] = 0 }, "budget_cents"},
		"title too short":  {func(b map[string]any) { b["title"] = "Room" }, "title"},
		"thirteen people":  {func(b map[string]any) { b["occupants"] = 13 }, "occupants"},
		"lease too long":   {func(b map[string]any) { b["lease_months"] = 36 }, "lease_months"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			body := validRequest()
			tc.mutate(body)

			res := h.do(t, http.MethodPost, "/requests", body)
			if res.StatusCode != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", res.StatusCode)
			}
			env := decode[apierror.Envelope](t, res)
			if env.Details[tc.field] == "" {
				t.Errorf("expected a problem against %q, got %+v", tc.field, env.Details)
			}
		})
	}
}

func TestEditingARequestChangesOnlyWhatIsSent(t *testing.T) {
	h, _ := signedIn(t, true)
	_, created := postRequest(t, h, validRequest())
	id := created["id"].(string)

	res := h.do(t, http.MethodPatch, "/requests/"+id, map[string]any{"budget_cents": 105000})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	updated := decode[map[string]any](t, res)

	if updated["budget_cents"].(float64) != 105000 {
		t.Errorf("budget_cents = %v", updated["budget_cents"])
	}
	// Everything omitted must survive.
	if updated["title"] != created["title"] || updated["neighbourhood"] != created["neighbourhood"] {
		t.Error("an untouched field changed")
	}
}

func TestEditingARequestIsValidatedAgainstTheWholeThing(t *testing.T) {
	h, _ := signedIn(t, true)
	_, created := postRequest(t, h, validRequest())

	// Moving only the end date still inverts the term.
	res := h.do(t, http.MethodPatch, "/requests/"+created["id"].(string),
		map[string]any{"end_date": "2026-06-01"})
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", res.StatusCode)
	}
	if decode[apierror.Envelope](t, res).Details["end_date"] == "" {
		t.Error("expected a problem against end_date")
	}
}

func TestRequestLifecycle(t *testing.T) {
	h, _ := signedIn(t, true)
	_, created := postRequest(t, h, validRequest())
	id := created["id"].(string)

	publicTotal := func() int {
		_, page := getJSON[requestPageBody](t, h.server.URL+"/requests")
		return page.Meta.Total
	}

	if publicTotal() != 1 {
		t.Fatal("a published request should be publicly visible")
	}

	if res := h.do(t, http.MethodPost, "/requests/"+id+"/status",
		map[string]string{"status": "paused"}); res.StatusCode != http.StatusOK {
		t.Fatalf("pause: %d", res.StatusCode)
	}
	if publicTotal() != 0 {
		t.Error("a paused request must not appear in public results")
	}

	if res := h.do(t, http.MethodPost, "/requests/"+id+"/status",
		map[string]string{"status": "published"}); res.StatusCode != http.StatusOK {
		t.Fatalf("republish: %d", res.StatusCode)
	}
	if publicTotal() != 1 {
		t.Error("a republished request should be public again")
	}

	// Taking it down archives rather than deletes, and archiving is terminal.
	if res := h.do(t, http.MethodDelete, "/requests/"+id, nil); res.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: %d", res.StatusCode)
	}
	if publicTotal() != 0 {
		t.Error("a taken-down request must not appear in public results")
	}
	res := h.do(t, http.MethodPost, "/requests/"+id+"/status", map[string]string{"status": "published"})
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("un-archiving gave %d, want 400", res.StatusCode)
	}

	// The poster still sees it, which is the only way to.
	mine := h.do(t, http.MethodGet, "/me/requests", nil)
	if mine.StatusCode != http.StatusOK {
		t.Fatalf("GET /me/requests: %d", mine.StatusCode)
	}
	if rows := decode[[]requestBody](t, mine); len(rows) != 1 || rows[0].Status != "archived" {
		t.Errorf("own requests = %+v", rows)
	}
}

func TestSomeoneElsesRequestIsNotFound(t *testing.T) {
	h, _ := signedIn(t, true)
	_, created := postRequest(t, h, validRequest())
	id := created["id"].(string)

	// A second account in the same harness.
	other := "stranger@uwaterloo.ca"
	if res := h.do(t, http.MethodPost, "/auth/register",
		map[string]string{"email": other, "username": "stranger", "password": testPassword}); res.StatusCode != http.StatusCreated {
		t.Fatalf("register: %d", res.StatusCode)
	}
	if res := h.do(t, http.MethodPost, "/auth/verify",
		map[string]string{"token": h.mail.token(t)}); res.StatusCode != http.StatusOK {
		t.Fatalf("verify: %d", res.StatusCode)
	}
	if res := h.do(t, http.MethodPost, "/auth/login",
		map[string]string{"email": other, "password": testPassword}); res.StatusCode != http.StatusOK {
		t.Fatalf("login: %d", res.StatusCode)
	}

	// 404 rather than 403: a 403 would confirm the request exists.
	for _, attempt := range []struct {
		method string
		path   string
		body   any
	}{
		{http.MethodPatch, "/requests/" + id, map[string]any{"budget_cents": 1}},
		{http.MethodDelete, "/requests/" + id, nil},
		{http.MethodPost, "/requests/" + id + "/status", map[string]string{"status": "paused"}},
	} {
		t.Run(attempt.method, func(t *testing.T) {
			res := h.do(t, attempt.method, attempt.path, attempt.body)
			if res.StatusCode != http.StatusNotFound {
				t.Errorf("status = %d, want 404", res.StatusCode)
			}
		})
	}
}

func TestBrowseRequestFiltersFromTheOwnersSide(t *testing.T) {
	h, _ := signedIn(t, true)

	modest := validRequest()
	modest["title"] = "Single room near the plaza, quiet please"
	modest["budget_cents"] = 70000
	modest["max_distance_m"] = 1000
	modest["occupants"] = 1

	generous := validRequest()
	generous["title"] = "Two roommates want a two-bedroom"
	generous["budget_cents"] = 160000
	generous["max_distance_m"] = 5000
	generous["occupants"] = 2

	anywhere := validRequest()
	anywhere["title"] = "Anywhere on a bus route"
	anywhere["budget_cents"] = 100000
	delete(anywhere, "max_distance_m")

	for _, body := range []map[string]any{modest, generous, anywhere} {
		if res, _ := postRequest(t, h, body); res.StatusCode != http.StatusCreated {
			t.Fatalf("seeding a request failed: %d", res.StatusCode)
		}
	}

	cases := map[string]struct {
		query string
		want  int
	}{
		"everything":              {"", 3},
		"can afford $900":         {"?budget_min_cents=90000", 2},
		"budget under $1000":      {"?budget_max_cents=100000", 2},
		"a place 3 km out":        {"?distance_min_m=3000", 2},
		"a place 500 m out":       {"?distance_min_m=500", 3},
		"room for one":            {"?occupants_max=1", 2},
		"search":                  {"?q=roommates", 1},
		"search matching nothing": {"?q=submarine", 0},
		"combined":                {"?budget_min_cents=90000&distance_min_m=3000", 2},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, page := getJSON[requestPageBody](t, h.server.URL+"/requests"+tc.query)
			if page.Meta.Total != tc.want {
				t.Errorf("total = %d, want %d", page.Meta.Total, tc.want)
			}
			if len(page.Data) != tc.want {
				t.Errorf("rows = %d, want %d", len(page.Data), tc.want)
			}
		})
	}
}

func TestGetRequestAnswers404ForAnythingNotPublished(t *testing.T) {
	h, _ := signedIn(t, true)

	draft := validRequest()
	draft["status"] = "draft"
	_, created := postRequest(t, h, draft)
	id := created["id"].(string)

	res, _ := getJSON[map[string]any](t, h.server.URL+"/requests/"+id)
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("a draft was publicly readable: %d", res.StatusCode)
	}

	// Its poster still sees it in their own list.
	mine := h.do(t, http.MethodGet, "/me/requests", nil)
	if rows := decode[[]requestBody](t, mine); len(rows) != 1 || rows[0].Status != "draft" {
		t.Errorf("own requests = %+v", rows)
	}

	// And a malformed id is a 400, not a 500.
	bad, _ := getJSON[map[string]any](t, fmt.Sprintf("%s/requests/not-a-uuid", h.server.URL))
	if bad.StatusCode != http.StatusBadRequest {
		t.Errorf("malformed id gave %d, want 400", bad.StatusCode)
	}
}
