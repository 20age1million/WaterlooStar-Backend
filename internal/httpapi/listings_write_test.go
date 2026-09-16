package httpapi_test

import (
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"testing"

	"github.com/20age1million/WaterlooStar-Backend/internal/apierror"
)

// The write path. Two rules are tested hardest, because getting either wrong is
// a real failure rather than an inconvenience:
//
//   - only a verified student may post
//   - someone else's listing answers 404, never 403

func validListing() map[string]any {
	return map[string]any{
		"title":          "Bright room in a 4-bed student house",
		"body":           "Quiet side of the house.",
		"conditions":     []string{"No smoking inside."},
		"price_cents":    84500,
		"start_date":     "2027-01-01",
		"end_date":       "2027-04-30",
		"lease_months":   4,
		"term_tag":       "Winter term",
		"unit_type":      "room",
		"bedrooms_total": 4,
		"bedroom_of":     1,
		"bathrooms":      1,
		"bath_type":      "shared",
		"furnished":      true,
		"utilities":      []string{"internet", "hydro"},
		"address_line":   "Hazel St",
		"neighbourhood":  "Northdale",
	}
}

// signedIn returns a harness whose client holds a session. verified controls
// whether the account has confirmed its address.
func signedIn(t *testing.T, verified bool) (*harness, string) {
	t.Helper()
	h := newHarness(t)

	email := "poster@uwaterloo.ca"
	body := map[string]string{"email": email, "username": "poster", "password": testPassword}
	if res := h.do(t, http.MethodPost, "/auth/register", body); res.StatusCode != http.StatusCreated {
		t.Fatalf("register: %d", res.StatusCode)
	}
	if verified {
		if res := h.do(t, http.MethodPost, "/auth/verify",
			map[string]string{"token": h.mail.token(t)}); res.StatusCode != http.StatusOK {
			t.Fatalf("verify: %d", res.StatusCode)
		}
	}
	if res := h.do(t, http.MethodPost, "/auth/login",
		map[string]string{"email": email, "password": testPassword}); res.StatusCode != http.StatusOK {
		t.Fatalf("login: %d", res.StatusCode)
	}
	return h, email
}

func createListing(t *testing.T, h *harness, body map[string]any) (*http.Response, map[string]any) {
	t.Helper()
	res := h.do(t, http.MethodPost, "/listings", body)
	var out map[string]any
	_ = json.NewDecoder(res.Body).Decode(&out)
	return res, out
}

func TestCreateRequiresAVerifiedStudent(t *testing.T) {
	t.Run("anonymous is refused", func(t *testing.T) {
		h := newHarness(t)
		res, _ := createListing(t, h, validListing())
		if res.StatusCode != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", res.StatusCode)
		}
	})

	t.Run("signed in but unverified is refused", func(t *testing.T) {
		h, _ := signedIn(t, false)
		res, _ := createListing(t, h, validListing())
		// 403, not 401: they are who they say they are, they just have not
		// proved they are a student — which is the whole trust proposition.
		if res.StatusCode != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", res.StatusCode)
		}
	})

	t.Run("verified succeeds", func(t *testing.T) {
		h, _ := signedIn(t, true)
		res, listing := createListing(t, h, validListing())
		if res.StatusCode != http.StatusCreated {
			t.Fatalf("status = %d, want 201", res.StatusCode)
		}
		if listing["status"] != "published" {
			t.Errorf("status = %v, want published by default", listing["status"])
		}
		if listing["title"] != "Bright room in a 4-bed student house" {
			t.Errorf("title = %v", listing["title"])
		}
	})
}

func TestCreatedListingAppearsInPublicResults(t *testing.T) {
	h, _ := signedIn(t, true)
	if res, _ := createListing(t, h, validListing()); res.StatusCode != http.StatusCreated {
		t.Fatal("create failed")
	}

	// Anonymous read: what any visitor sees.
	_, page := getJSON[listingPageBody](t, h.server.URL+"/listings")
	if page.Meta.Total != 1 {
		t.Fatalf("public results show %d listings, want the one just posted", page.Meta.Total)
	}
}

func TestCreateValidation(t *testing.T) {
	h, _ := signedIn(t, true)

	cases := map[string]struct {
		mutate func(map[string]any)
		field  string
	}{
		"end before start":     {func(b map[string]any) { b["end_date"] = "2026-01-01" }, "end_date"},
		"free rent":            {func(b map[string]any) { b["price_cents"] = 0 }, "price_cents"},
		"title too short":      {func(b map[string]any) { b["title"] = "Room" }, "title"},
		"room 9 of 4":          {func(b map[string]any) { b["bedroom_of"] = 9 }, "bedroom_of"},
		"no address":           {func(b map[string]any) { b["address_line"] = "  " }, "address_line"},
		"studio with a bednum": {func(b map[string]any) { b["unit_type"] = "studio"; b["bedroom_of"] = 1 }, "bedroom_of"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			body := validListing()
			tc.mutate(body)

			res := h.do(t, http.MethodPost, "/listings", body)
			if res.StatusCode != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", res.StatusCode)
			}
			env := decode[apierror.Envelope](t, res)
			if env.Details[tc.field] == "" {
				// A field-level message is the difference between a usable form
				// and a mystery.
				t.Errorf("expected a problem against %q, got %+v", tc.field, env.Details)
			}
		})
	}
}

func TestUpdateChangesOnlyWhatIsSent(t *testing.T) {
	h, _ := signedIn(t, true)
	_, created := createListing(t, h, validListing())
	id := created["id"].(string)

	res := h.do(t, http.MethodPatch, "/listings/"+id, map[string]any{"price_cents": 79900})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	updated := decode[map[string]any](t, res)

	if updated["price_cents"].(float64) != 79900 {
		t.Errorf("price_cents = %v, want the new rent", updated["price_cents"])
	}
	// Everything omitted must survive. A PUT would have wiped these.
	if updated["title"] != created["title"] {
		t.Errorf("title changed to %v", updated["title"])
	}
	if updated["address_line"] != created["address_line"] {
		t.Errorf("address_line changed to %v", updated["address_line"])
	}
	if updated["bedrooms_total"] != created["bedrooms_total"] {
		t.Errorf("bedrooms_total changed to %v", updated["bedrooms_total"])
	}
}

func TestUpdateIsValidatedAgainstTheWholeListing(t *testing.T) {
	h, _ := signedIn(t, true)
	_, created := createListing(t, h, validListing())
	id := created["id"].(string)

	// Moving only the end date still inverts the term. A patch checked in
	// isolation would not notice.
	res := h.do(t, http.MethodPatch, "/listings/"+id, map[string]any{"end_date": "2026-06-01"})
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", res.StatusCode)
	}
	if decode[apierror.Envelope](t, res).Details["end_date"] == "" {
		t.Error("expected a problem against end_date")
	}
}

func TestLifecycle(t *testing.T) {
	h, _ := signedIn(t, true)
	_, created := createListing(t, h, validListing())
	id := created["id"].(string)

	publicTotal := func() int {
		_, page := getJSON[listingPageBody](t, h.server.URL+"/listings")
		return page.Meta.Total
	}

	if publicTotal() != 1 {
		t.Fatal("a published listing should be publicly visible")
	}

	// Pause hides it.
	if res := h.do(t, http.MethodPost, "/listings/"+id+"/status",
		map[string]string{"status": "paused"}); res.StatusCode != http.StatusOK {
		t.Fatalf("pause: %d", res.StatusCode)
	}
	if publicTotal() != 0 {
		t.Error("a paused listing must not appear in public results")
	}

	// Publishing brings it back.
	if res := h.do(t, http.MethodPost, "/listings/"+id+"/status",
		map[string]string{"status": "published"}); res.StatusCode != http.StatusOK {
		t.Fatalf("republish: %d", res.StatusCode)
	}
	if publicTotal() != 1 {
		t.Error("republishing should restore it")
	}

	// Archive is terminal.
	if res := h.do(t, http.MethodPost, "/listings/"+id+"/status",
		map[string]string{"status": "archived"}); res.StatusCode != http.StatusOK {
		t.Fatalf("archive: %d", res.StatusCode)
	}
	res := h.do(t, http.MethodPost, "/listings/"+id+"/status",
		map[string]string{"status": "published"})
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("un-archiving gave %d, want 400 — archived is final", res.StatusCode)
	}
}

func TestDeleteArchivesRatherThanRemoving(t *testing.T) {
	h, _ := signedIn(t, true)
	_, created := createListing(t, h, validListing())
	id := created["id"].(string)

	if res := h.do(t, http.MethodDelete, "/listings/"+id, nil); res.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: %d", res.StatusCode)
	}

	// Gone from public view...
	res, _ := getJSON[map[string]any](t, h.server.URL+"/listings/"+id)
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("a deleted listing should 404 publicly, got %d", res.StatusCode)
	}

	// ...but the row survives, because questions and saves hang off it.
	mine := h.do(t, http.MethodGet, "/me/listings", nil)
	listings := decode[[]map[string]any](t, mine)
	if len(listings) != 1 || listings[0]["status"] != "archived" {
		t.Errorf("owner should still see it as archived, got %+v", listings)
	}
}

func TestAnotherUsersListingIsInvisible(t *testing.T) {
	owner, _ := signedIn(t, true)
	_, created := createListing(t, owner, validListing())
	id := created["id"].(string)

	// A second, unrelated verified account.
	stranger := newHarness(t)
	stranger.db = owner.db
	stranger.mail = owner.mail
	jar, _ := cookiejar.New(nil)
	stranger.client = &http.Client{Jar: jar}

	// Point the stranger at the same server, then sign them in.
	stranger.server = owner.server
	reg := map[string]string{"email": "other@uwaterloo.ca", "username": "other", "password": testPassword}
	if res := stranger.do(t, http.MethodPost, "/auth/register", reg); res.StatusCode != http.StatusCreated {
		t.Fatalf("register stranger: %d", res.StatusCode)
	}
	if res := stranger.do(t, http.MethodPost, "/auth/verify",
		map[string]string{"token": stranger.mail.token(t)}); res.StatusCode != http.StatusOK {
		t.Fatalf("verify stranger: %d", res.StatusCode)
	}
	if res := stranger.do(t, http.MethodPost, "/auth/login",
		map[string]string{"email": "other@uwaterloo.ca", "password": testPassword}); res.StatusCode != http.StatusOK {
		t.Fatalf("login stranger: %d", res.StatusCode)
	}

	// 404 everywhere, never 403: a 403 would confirm the listing exists.
	for _, tc := range []struct {
		method string
		path   string
		body   any
	}{
		{http.MethodPatch, "/listings/" + id, map[string]any{"price_cents": 1}},
		{http.MethodDelete, "/listings/" + id, nil},
		{http.MethodPost, "/listings/" + id + "/status", map[string]string{"status": "archived"}},
	} {
		res := stranger.do(t, tc.method, tc.path, tc.body)
		if res.StatusCode != http.StatusNotFound {
			t.Errorf("%s %s gave %d, want 404", tc.method, tc.path, res.StatusCode)
		}
	}

	// And it is untouched.
	still := owner.do(t, http.MethodGet, "/me/listings", nil)
	mine := decode[[]map[string]any](t, still)
	if len(mine) != 1 || mine[0]["status"] != "published" {
		t.Errorf("the owner's listing was altered: %+v", mine)
	}
}

func TestMyListingsShowsEveryStatus(t *testing.T) {
	h, _ := signedIn(t, true)

	draft := validListing()
	draft["status"] = "draft"
	draft["title"] = "A draft I have not finished"
	if res, _ := createListing(t, h, draft); res.StatusCode != http.StatusCreated {
		t.Fatal("create draft failed")
	}
	if res, _ := createListing(t, h, validListing()); res.StatusCode != http.StatusCreated {
		t.Fatal("create published failed")
	}

	// The public endpoint hides the draft...
	_, page := getJSON[listingPageBody](t, h.server.URL+"/listings")
	if page.Meta.Total != 1 {
		t.Errorf("public results show %d, want only the published one", page.Meta.Total)
	}

	// ...and the owner's own endpoint shows both.
	res := h.do(t, http.MethodGet, "/me/listings", nil)
	mine := decode[[]map[string]any](t, res)
	if len(mine) != 2 {
		t.Errorf("owner sees %d listings, want both", len(mine))
	}
}

func TestMyListingsRequiresASession(t *testing.T) {
	h := newHarness(t)
	res := h.do(t, http.MethodGet, "/me/listings", nil)
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", res.StatusCode)
	}
}
