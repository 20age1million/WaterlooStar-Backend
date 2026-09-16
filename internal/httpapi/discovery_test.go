package httpapi_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/20age1million/WaterlooStar-Backend/internal/apierror"
	"github.com/20age1million/WaterlooStar-Backend/internal/db/sqlcgen"
)

// A small, deliberately varied set: each listing differs from the others in the
// dimensions the filters cut on, so a filter that did nothing would be obvious.
func seedVariety(t *testing.T, db *fakeQuerier) {
	t.Helper()
	ctx := context.Background()

	verified, err := db.CreateUser(ctx, sqlcgen.CreateUserParams{
		Email: "meil@uwaterloo.ca", Username: "meil", PasswordHash: "x",
	})
	if err != nil {
		t.Fatalf("create verified owner: %v", err)
	}
	if _, err := db.MarkUserVerified(ctx, verified.ID); err != nil {
		t.Fatalf("verify owner: %v", err)
	}

	// Left unverified on purpose, for the verified_only filter.
	unverified, err := db.CreateUser(ctx, sqlcgen.CreateUserParams{
		Email: "newbie@uwaterloo.ca", Username: "newbie", PasswordHash: "x",
	})
	if err != nil {
		t.Fatalf("create unverified owner: %v", err)
	}

	d := func(v int32) *int32 { return &v }
	date := func(y int, m time.Month, day int) time.Time {
		return time.Date(y, m, day, 0, 0, 0, 0, time.UTC)
	}

	rows := []sqlcgen.CreateListingParams{
		{
			OwnerID: verified.ID, Title: "Bright room near campus", Body: "Quiet house on Hazel St.",
			PriceCents: 84500, StartDate: date(2027, 1, 1), EndDate: date(2027, 4, 30),
			LeaseMonths: 4, TermTag: "Winter term", UnitType: "room", BedroomsTotal: 4,
			Bathrooms: 1, BathType: "shared", Furnished: true,
			Utilities: []string{"internet", "hydro"}, AddressLine: "Hazel St",
			Neighbourhood: "Northdale", DistanceM: d(800), CommuteMode: "walk",
			Status: "published", CreatedAt: time.Now().Add(-1 * time.Hour),
		},
		{
			OwnerID: verified.ID, Title: "Quiet basement apartment", Body: "Separate entrance.",
			PriceCents: 110000, StartDate: date(2027, 1, 1), EndDate: date(2027, 8, 31),
			LeaseMonths: 8, TermTag: "8 months", UnitType: "unit", BedroomsTotal: 1,
			Bathrooms: 1, BathType: "private", Furnished: false,
			Utilities: []string{"hydro"}, AddressLine: "Regina St N",
			Neighbourhood: "Uptown", DistanceM: d(1300), CommuteMode: "walk",
			Parking: true, Status: "published", CreatedAt: time.Now().Add(-2 * time.Hour),
		},
		{
			OwnerID: verified.ID, Title: "Two-bedroom for a pair of co-ops", Body: "Parking included.",
			PriceCents: 165000, StartDate: date(2027, 9, 1), EndDate: date(2028, 4, 30),
			LeaseMonths: 8, TermTag: "Fall + Winter", UnitType: "unit", BedroomsTotal: 2,
			Bathrooms: 1, BathType: "private", Furnished: true,
			Utilities: []string{"water", "heat"}, AddressLine: "King St N",
			Neighbourhood: "Uptown", DistanceM: d(2600), CommuteMode: "bus",
			Parking: true, Pets: true, Laundry: true,
			Status: "published", CreatedAt: time.Now().Add(-3 * time.Hour),
		},
		{
			// Unverified owner, and no recorded distance.
			OwnerID: unverified.ID, Title: "Studio with blackout blinds", Body: "Desk wall.",
			PriceCents: 129000, StartDate: date(2027, 1, 1), EndDate: date(2027, 12, 31),
			LeaseMonths: 12, TermTag: "Full year", UnitType: "studio", BedroomsTotal: 0,
			Bathrooms: 1, BathType: "private", Furnished: true,
			Utilities:   []string{"internet", "hydro", "water", "heat", "gas"},
			AddressLine: "University Ave W", Neighbourhood: "Northdale",
			CommuteMode: "walk", Status: "published", CreatedAt: time.Now().Add(-4 * time.Hour),
		},
	}

	for _, r := range rows {
		if _, err := db.CreateListing(ctx, r); err != nil {
			t.Fatalf("create listing %q: %v", r.Title, err)
		}
	}
}

func query(t *testing.T, srv string, qs string) listingPageBody {
	t.Helper()
	_, body := getJSON[listingPageBody](t, srv+"/listings?"+qs)
	return body
}

func titles(p listingPageBody) []string {
	out := make([]string, 0, len(p.Data))
	for _, l := range p.Data {
		out = append(out, l.Title)
	}
	return out
}

func TestFilterBySearch(t *testing.T) {
	db := newFakeQuerier()
	srv := newTestServer(t, db)
	seedVariety(t, db)

	// Title, body, neighbourhood and address are all searchable.
	for _, tc := range []struct{ q, want string }{
		{"basement", "Quiet basement apartment"},
		{"Hazel", "Bright room near campus"},
		{"Uptown", ""}, // two listings; only checking it narrows
	} {
		page := query(t, srv.URL, "q="+tc.q)
		if page.Meta.Total == 0 {
			t.Errorf("q=%q matched nothing", tc.q)
		}
		if tc.want != "" && (len(page.Data) != 1 || page.Data[0].Title != tc.want) {
			t.Errorf("q=%q gave %v, want only %q", tc.q, titles(page), tc.want)
		}
	}

	if page := query(t, srv.URL, "q=nothingmatchesthis"); page.Meta.Total != 0 {
		t.Errorf("a nonsense search returned %d listings", page.Meta.Total)
	}

	// Blank is not a search for the empty string.
	if page := query(t, srv.URL, "q=%20%20"); page.Meta.Total != 4 {
		t.Errorf("a whitespace search returned %d, want all 4", page.Meta.Total)
	}
}

func TestFilterByPrice(t *testing.T) {
	db := newFakeQuerier()
	srv := newTestServer(t, db)
	seedVariety(t, db)

	if page := query(t, srv.URL, "price_max_cents=100000"); page.Meta.Total != 1 {
		t.Errorf("under $1000 gave %v, want just the $845 room", titles(page))
	}
	if page := query(t, srv.URL, "price_min_cents=120000"); page.Meta.Total != 2 {
		t.Errorf("over $1200 gave %v, want 2", titles(page))
	}
	if page := query(t, srv.URL, "price_min_cents=100000&price_max_cents=140000"); page.Meta.Total != 2 {
		t.Errorf("a band gave %v, want 2", titles(page))
	}
}

func TestFilterByDistance(t *testing.T) {
	db := newFakeQuerier()
	srv := newTestServer(t, db)
	seedVariety(t, db)

	if page := query(t, srv.URL, "distance_max_m=1000"); page.Meta.Total != 1 {
		t.Errorf("within 1 km gave %v, want 1", titles(page))
	}

	// "Within 2.6 km" must not quietly include the listing whose distance is
	// unknown — that would be claiming something the data does not support.
	page := query(t, srv.URL, "distance_max_m=3000")
	if page.Meta.Total != 3 {
		t.Errorf("within 3 km gave %v, want the 3 with a recorded distance", titles(page))
	}
	for _, title := range titles(page) {
		if title == "Studio with blackout blinds" {
			t.Error("a listing with no recorded distance must be excluded by a distance filter")
		}
	}
}

func TestFilterByTermWindow(t *testing.T) {
	db := newFakeQuerier()
	srv := newTestServer(t, db)
	seedVariety(t, db)

	// Available by 1 Jan and running to 30 Apr — the Winter term.
	page := query(t, srv.URL, "start_after=2027-01-01&end_before=2027-04-30")
	if page.Meta.Total != 3 {
		t.Errorf("Winter window gave %v, want the 3 that cover it", titles(page))
	}
	for _, title := range titles(page) {
		if title == "Two-bedroom for a pair of co-ops" {
			t.Error("a listing starting in September cannot serve a January window")
		}
	}
}

func TestFilterByAmenities(t *testing.T) {
	db := newFakeQuerier()
	srv := newTestServer(t, db)
	seedVariety(t, db)

	cases := map[string]int{
		"furnished=true":  3,
		"furnished=false": 1,
		"parking=true":    2,
		"pets=true":       1,
		"laundry=true":    1,
		"bedrooms_min=2":  2,
		"bedrooms_min=4":  1,
	}
	for qs, want := range cases {
		if page := query(t, srv.URL, qs); page.Meta.Total != want {
			t.Errorf("%s gave %d (%v), want %d", qs, page.Meta.Total, titles(page), want)
		}
	}
}

func TestFilterByUtilitiesRequiresAll(t *testing.T) {
	db := newFakeQuerier()
	srv := newTestServer(t, db)
	seedVariety(t, db)

	if page := query(t, srv.URL, "utilities=hydro"); page.Meta.Total != 3 {
		t.Errorf("hydro gave %v, want 3", titles(page))
	}
	// Both, not either: asking for internet and gas means a listing must include
	// both to qualify.
	page := query(t, srv.URL, "utilities=internet&utilities=gas")
	if page.Meta.Total != 1 || page.Data[0].Title != "Studio with blackout blinds" {
		t.Errorf("internet+gas gave %v, want only the all-inclusive studio", titles(page))
	}
}

func TestFilterByVerifiedOwner(t *testing.T) {
	db := newFakeQuerier()
	srv := newTestServer(t, db)
	seedVariety(t, db)

	page := query(t, srv.URL, "verified_only=true")
	if page.Meta.Total != 3 {
		t.Errorf("verified_only gave %v, want the 3 from the verified poster", titles(page))
	}
	// verified_only=false must not become a filter for unverified listings.
	if page := query(t, srv.URL, "verified_only=false"); page.Meta.Total != 4 {
		t.Errorf("verified_only=false gave %d, want all 4", page.Meta.Total)
	}
}

func TestSorting(t *testing.T) {
	db := newFakeQuerier()
	srv := newTestServer(t, db)
	seedVariety(t, db)

	asc := query(t, srv.URL, "sort=priceAsc")
	for i := 1; i < len(asc.Data); i++ {
		if asc.Data[i-1].PriceCents > asc.Data[i].PriceCents {
			t.Fatalf("priceAsc out of order: %v", titles(asc))
		}
	}

	desc := query(t, srv.URL, "sort=priceDesc")
	for i := 1; i < len(desc.Data); i++ {
		if desc.Data[i-1].PriceCents < desc.Data[i].PriceCents {
			t.Fatalf("priceDesc out of order: %v", titles(desc))
		}
	}

	// The listing with no distance sorts last rather than first.
	dist := query(t, srv.URL, "sort=distance")
	if dist.Data[0].Title != "Bright room near campus" {
		t.Errorf("distance sort led with %q, want the closest", dist.Data[0].Title)
	}
	if dist.Data[len(dist.Data)-1].Title != "Studio with blackout blinds" {
		t.Errorf("a listing with no distance should sort last, got %v", titles(dist))
	}

	// match is accepted and behaves as newest until Phase 8 scores it.
	if m, n := query(t, srv.URL, "sort=match"), query(t, srv.URL, "sort=new"); titles(m)[0] != titles(n)[0] {
		t.Errorf("sort=match should currently match sort=new")
	}
}

func TestFiltersCompose(t *testing.T) {
	db := newFakeQuerier()
	srv := newTestServer(t, db)
	seedVariety(t, db)

	page := query(t, srv.URL, "furnished=true&verified_only=true&price_max_cents=100000")
	if page.Meta.Total != 1 || page.Data[0].Title != "Bright room near campus" {
		t.Errorf("combined filters gave %v, want just the bright room", titles(page))
	}

	// A combination nothing satisfies is an empty page, not an error.
	empty := query(t, srv.URL, "furnished=true&pets=true&price_max_cents=50000")
	if empty.Meta.Total != 0 || len(empty.Data) != 0 {
		t.Errorf("an impossible combination returned %v", titles(empty))
	}
}

func TestCountReflectsTheFilters(t *testing.T) {
	db := newFakeQuerier()
	srv := newTestServer(t, db)
	seedVariety(t, db)

	// The total must describe the filtered set, not the table — it is the number
	// shown to a user as "N places available".
	page := query(t, srv.URL, "furnished=true&per_page=1")
	if page.Meta.Total != 3 {
		t.Errorf("total = %d, want 3 matching rows regardless of page size", page.Meta.Total)
	}
	if len(page.Data) != 1 {
		t.Errorf("per_page=1 returned %d rows", len(page.Data))
	}
	if page.Meta.TotalPages != 3 {
		t.Errorf("total_pages = %d, want 3", page.Meta.TotalPages)
	}
}

func TestRejectsImpossibleRanges(t *testing.T) {
	srv := newTestServer(t, newFakeQuerier())

	for _, qs := range []string{
		"price_min_cents=200000&price_max_cents=100000",
		"start_after=2027-06-01&end_before=2027-01-01",
	} {
		res, env := getJSON[apierror.Envelope](t, srv.URL+"/listings?"+qs)
		if res.StatusCode != http.StatusBadRequest {
			t.Errorf("%s gave %d, want 400", qs, res.StatusCode)
		}
		if env.Code == "" || env.Message == "" {
			t.Errorf("%s did not use the standard envelope: %+v", qs, env)
		}
	}
}
