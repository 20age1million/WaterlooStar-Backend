package httpapi_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

// Listing terms over HTTP: posting and editing shorter stays and the bill
// estimate, what a listing response says about them, and the availability read.

func withTerms(overrides map[string]any) map[string]any {
	body := validListing()
	for k, v := range overrides {
		body[k] = v
	}
	return body
}

// rawField reads one field exactly as it was sent, so "null" and "absent" can
// be told apart.
func rawField(t *testing.T, body map[string]any, field string) string {
	t.Helper()
	v, ok := body[field]
	if !ok {
		return "absent"
	}
	b, _ := json.Marshal(v)
	return string(b)
}

func TestListingTermsAreStoredAndReported(t *testing.T) {
	h, _ := signedIn(t, true)

	res, body := createListing(t, h, withTerms(map[string]any{
		"shorter_stays": true, "min_stay_months": 2, "bills_estimate_cents": 4500,
	}))
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create = %d: %v", res.StatusCode, body)
	}
	for field, want := range map[string]string{
		"shorter_stays": "true", "min_stay_months": "2", "bills_estimate_cents": "4500", "all_in_cents": "89000",
	} {
		if got := rawField(t, body, field); got != want {
			t.Errorf("%s = %s, want %s", field, got, want)
		}
	}

	// Without an estimate the all-in cost is null — present, and never guessed.
	_, plain := createListing(t, h, validListing())
	for field, want := range map[string]string{
		"shorter_stays": "false", "min_stay_months": "null", "bills_estimate_cents": "null", "all_in_cents": "null",
	} {
		if got := rawField(t, plain, field); got != want {
			t.Errorf("a listing without terms: %s = %s, want %s", field, got, want)
		}
	}

	// $0 is a statement: everything included.
	_, included := createListing(t, h, withTerms(map[string]any{"bills_estimate_cents": 0}))
	if got := rawField(t, included, "all_in_cents"); got != "84500" {
		t.Errorf("an estimate of $0: all_in_cents = %s, want the rent", got)
	}
}

func TestListingTermsValidation(t *testing.T) {
	h, _ := signedIn(t, true)

	cases := map[string]struct {
		overrides map[string]any
		field     string
	}{
		"shorter stays without a minimum":  {map[string]any{"shorter_stays": true}, "min_stay_months"},
		"a minimum without shorter stays":  {map[string]any{"min_stay_months": 2}, "min_stay_months"},
		"a minimum longer than the lease":  {map[string]any{"shorter_stays": true, "min_stay_months": 5}, "min_stay_months"},
		"a minimum of zero":                {map[string]any{"shorter_stays": true, "min_stay_months": 0}, "min_stay_months"},
		"an estimate over $1,000":          {map[string]any{"bills_estimate_cents": 100001}, "bills_estimate_cents"},
		"a negative estimate":              {map[string]any{"bills_estimate_cents": -1}, "bills_estimate_cents"},
	}
	for name, tc := range cases {
		res, body := createListing(t, h, withTerms(tc.overrides))
		if res.StatusCode != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400", name, res.StatusCode)
			continue
		}
		details, _ := body["details"].(map[string]any)
		if body["code"] != "validation_failed" || details[tc.field] == nil {
			t.Errorf("%s: %v, want validation_failed on %s", name, body, tc.field)
		}
	}
}

func TestEditingListingTerms(t *testing.T) {
	h, _ := signedIn(t, true)
	_, created := createListing(t, h, withTerms(map[string]any{
		"shorter_stays": true, "min_stay_months": 3, "bills_estimate_cents": 4500,
	}))
	id := created["id"].(string)

	patch := func(body map[string]any) (int, map[string]any) {
		t.Helper()
		res := h.do(t, http.MethodPatch, "/listings/"+id, body)
		var out map[string]any
		_ = json.NewDecoder(res.Body).Decode(&out)
		return res.StatusCode, out
	}

	// Shortening the lease below the minimum is refused, judged on the whole
	// listing as it would stand.
	status, body := patch(map[string]any{"end_date": "2027-02-28", "lease_months": 2})
	if status != http.StatusBadRequest {
		t.Errorf("lease below the minimum = %d: %v", status, body)
	}

	// An unrelated edit leaves the terms alone.
	if status, body := patch(map[string]any{"title": "A brighter title for the room"}); status != http.StatusOK ||
		rawField(t, body, "min_stay_months") != "3" || rawField(t, body, "bills_estimate_cents") != "4500" {
		t.Errorf("unrelated edit = %d: min %s, estimate %s", status, rawField(t, body, "min_stay_months"), rawField(t, body, "bills_estimate_cents"))
	}

	// Turning shorter stays off clears the minimum without being told to.
	if status, body := patch(map[string]any{"shorter_stays": false}); status != http.StatusOK ||
		rawField(t, body, "min_stay_months") != "null" {
		t.Errorf("shorter stays off = %d: min %s, want null", status, rawField(t, body, "min_stay_months"))
	}

	// An estimate can be withdrawn back to unknown, and the all-in cost goes with it.
	if status, body := patch(map[string]any{"bills_estimate_cents": nil}); status != http.StatusOK ||
		rawField(t, body, "bills_estimate_cents") != "null" || rawField(t, body, "all_in_cents") != "null" {
		t.Errorf("clear the estimate = %d: %s, %s", status, rawField(t, body, "bills_estimate_cents"), rawField(t, body, "all_in_cents"))
	}
}

func TestBrowsingByAllInCostAndWindow(t *testing.T) {
	h, _ := signedIn(t, true)
	createListing(t, h, withTerms(map[string]any{"title": "Known cost, 845 plus 45", "bills_estimate_cents": 4500}))
	createListing(t, h, withTerms(map[string]any{"title": "Unknown cost, rent only 845"}))

	_, page := getJSON[listingPageBody](t, h.server.URL+"/listings?all_in_max_cents=90000")
	if page.Meta.Total != 1 {
		t.Errorf("all_in_max_cents: %d listings, want only the one with a known cost", page.Meta.Total)
	}
	res, err := http.Get(h.server.URL + "/listings?sort=allInAsc")
	if err != nil || res.StatusCode != http.StatusOK {
		t.Errorf("sort=allInAsc: %v %d", err, res.StatusCode)
	}
	res.Body.Close()
}

func TestAvailabilityIsPublicAndAlwaysTwelveMonths(t *testing.T) {
	h, _ := signedIn(t, true)
	createListing(t, h, validListing()) // Jan 1 – Apr 30, 2027

	// Anonymous: http.Get carries no session.
	res, err := http.Get(h.server.URL + "/listings/availability?year=2027")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("availability = %d", res.StatusCode)
	}
	raw, _ := io.ReadAll(res.Body)
	var body struct {
		Year   int `json:"year"`
		Months []struct {
			Month, Open int
		} `json:"months"`
	}
	if err := json.NewDecoder(bytes.NewReader(raw)).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Year != 2027 || len(body.Months) != 12 {
		t.Fatalf("availability = %s", raw)
	}
	for i, m := range body.Months {
		want := 0
		if i < 4 {
			want = 1
		}
		if m.Month != i+1 || m.Open != want {
			t.Errorf("month %d = %+v, want %d open", i+1, m, want)
		}
	}

	for _, q := range []string{"?year=1999", "", "?year=2027&price_min_cents=900&price_max_cents=100"} {
		r, err := http.Get(h.server.URL + "/listings/availability" + q)
		if err != nil {
			t.Fatal(err)
		}
		r.Body.Close()
		if r.StatusCode != http.StatusBadRequest {
			t.Errorf("availability%s = %d, want 400", q, r.StatusCode)
		}
	}
}
