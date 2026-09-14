package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/20age1million/WaterlooStar-Backend/internal/apierror"
	"github.com/20age1million/WaterlooStar-Backend/internal/db/sqlcgen"
)

type listingBody struct {
	ID            string   `json:"id"`
	Title         string   `json:"title"`
	PriceCents    int      `json:"price_cents"`
	DepositCents  *int     `json:"deposit_cents"`
	StartDate     string   `json:"start_date"`
	EndDate       string   `json:"end_date"`
	LeaseMonths   int      `json:"lease_months"`
	TermTag       string   `json:"term_tag"`
	UnitType      string   `json:"unit_type"`
	BedroomsTotal int      `json:"bedrooms_total"`
	BedroomOf     *int     `json:"bedroom_of"`
	Bathrooms     float64  `json:"bathrooms"`
	BathType      string   `json:"bath_type"`
	Furnished     bool     `json:"furnished"`
	Utilities     []string `json:"utilities"`
	AddressLine   string   `json:"address_line"`
	Status        string   `json:"status"`
	Conditions    []string `json:"conditions"`
	Photos        []struct {
		ID string `json:"id"`
	} `json:"photos"`
	Owner struct {
		ID       string `json:"id"`
		Username string `json:"username"`
		Verified bool   `json:"verified"`
	} `json:"owner"`
}

type listingPageBody struct {
	Data []listingBody `json:"data"`
	Meta struct {
		Page       int `json:"page"`
		PerPage    int `json:"per_page"`
		Total      int `json:"total"`
		TotalPages int `json:"total_pages"`
	} `json:"meta"`
}

// seedListings puts n published listings in the fake, newest first by index 0.
func seedListings(t *testing.T, db *fakeQuerier, n int) []sqlcgen.Listing {
	t.Helper()
	ctx := context.Background()

	owner, err := db.CreateUser(ctx, sqlcgen.CreateUserParams{
		Email:        "poster@uwaterloo.ca",
		Username:     "poster",
		PasswordHash: "x",
	})
	if err != nil {
		t.Fatalf("create owner: %v", err)
	}
	if _, err := db.MarkUserVerified(ctx, owner.ID); err != nil {
		t.Fatalf("verify owner: %v", err)
	}

	deposit := int32(84500)
	bedroomOf := int32(1)
	out := make([]sqlcgen.Listing, 0, n)
	for i := 0; i < n; i++ {
		l, err := db.CreateListing(ctx, sqlcgen.CreateListingParams{
			OwnerID:       owner.ID,
			Title:         "Listing " + string(rune('A'+i)),
			Body:          "A description.",
			Conditions:    []string{"No smoking."},
			PriceCents:    int32(80000 + i*1000),
			DepositCents:  &deposit,
			StartDate:     time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
			EndDate:       time.Date(2027, 4, 30, 0, 0, 0, 0, time.UTC),
			LeaseMonths:   4,
			TermTag:       "Winter term",
			UnitType:      "room",
			BedroomsTotal: 4,
			BedroomOf:     &bedroomOf,
			Bathrooms:     1,
			BathType:      "shared",
			Furnished:     true,
			Utilities:     []string{"internet", "hydro"},
			AddressLine:   "Hazel St",
			Neighbourhood: "Northdale",
			CommuteMode:   "walk",
			Status:        "published",
			// Descending creation time, so index 0 is newest.
			CreatedAt: time.Now().Add(time.Duration(-i) * time.Hour),
		})
		if err != nil {
			t.Fatalf("create listing: %v", err)
		}
		out = append(out, l)
	}
	return out
}

func getJSON[T any](t *testing.T, url string) (*http.Response, T) {
	t.Helper()
	res, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	t.Cleanup(func() { res.Body.Close() })

	var body T
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decode %s: %v", url, err)
	}
	return res, body
}

func TestListListingsReturnsPublishedRows(t *testing.T) {
	db := newFakeQuerier()
	srv := newTestServer(t, db)
	seedListings(t, db, 3)

	res, page := getJSON[listingPageBody](t, srv.URL+"/listings")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}

	if len(page.Data) != 3 {
		t.Fatalf("returned %d listings, want 3", len(page.Data))
	}
	if page.Meta.Total != 3 || page.Meta.Page != 1 || page.Meta.PerPage != 20 || page.Meta.TotalPages != 1 {
		t.Errorf("meta = %+v, want page 1, per_page 20, total 3, total_pages 1", page.Meta)
	}

	first := page.Data[0]
	if first.Owner.Username != "poster" || !first.Owner.Verified {
		t.Errorf("owner summary = %+v, want the verified poster", first.Owner)
	}
	// A field that is always a list must serialise as [], never null, so the
	// client does not have to guard it.
	if first.Photos == nil {
		t.Error("photos should be an empty array, not null")
	}
}

func TestListListingsOrdersNewestFirst(t *testing.T) {
	db := newFakeQuerier()
	srv := newTestServer(t, db)
	created := seedListings(t, db, 3)

	_, page := getJSON[listingPageBody](t, srv.URL+"/listings")

	// seedListings backdates by index, so index 0 is the newest.
	if page.Data[0].ID != created[0].ID.String() {
		t.Errorf("first result = %q, want the newest listing %q", page.Data[0].ID, created[0].ID)
	}
}

func TestListListingsExcludesUnpublished(t *testing.T) {
	db := newFakeQuerier()
	srv := newTestServer(t, db)
	seedListings(t, db, 2)

	// Draft, paused and archived posts are nobody else's business.
	for _, status := range []string{"draft", "paused", "archived"} {
		db.mu.Lock()
		hidden := db.listings[0]
		hidden.ID = uuid.New()
		hidden.Status = status
		db.listings = append(db.listings, hidden)
		db.mu.Unlock()
	}

	_, page := getJSON[listingPageBody](t, srv.URL+"/listings")
	if page.Meta.Total != 2 || len(page.Data) != 2 {
		t.Errorf("got %d of %d, want only the 2 published listings", len(page.Data), page.Meta.Total)
	}
}

func TestListListingsPaginates(t *testing.T) {
	db := newFakeQuerier()
	srv := newTestServer(t, db)
	created := seedListings(t, db, 5)

	_, first := getJSON[listingPageBody](t, srv.URL+"/listings?page=1&per_page=2")
	if len(first.Data) != 2 || first.Meta.TotalPages != 3 || first.Meta.Total != 5 {
		t.Fatalf("page 1 meta = %+v, len = %d; want 2 of 5 across 3 pages", first.Meta, len(first.Data))
	}

	_, third := getJSON[listingPageBody](t, srv.URL+"/listings?page=3&per_page=2")
	if len(third.Data) != 1 {
		t.Fatalf("page 3 returned %d listings, want the 1 remainder", len(third.Data))
	}
	if third.Data[0].ID != created[4].ID.String() {
		t.Errorf("page 3 has the wrong listing: %q", third.Data[0].ID)
	}

	// Past the end is an empty page, not an error.
	res, beyond := getJSON[listingPageBody](t, srv.URL+"/listings?page=99&per_page=2")
	if res.StatusCode != http.StatusOK || len(beyond.Data) != 0 {
		t.Errorf("page 99: status %d with %d listings, want 200 with none", res.StatusCode, len(beyond.Data))
	}
}

func TestListListingsClampsPerPage(t *testing.T) {
	db := newFakeQuerier()
	srv := newTestServer(t, db)
	seedListings(t, db, 3)

	// Clamped rather than rejected — refusing an oversized request helps nobody.
	_, page := getJSON[listingPageBody](t, srv.URL+"/listings?per_page=5000")
	if page.Meta.PerPage != 100 {
		t.Errorf("per_page = %d, want it clamped to 100", page.Meta.PerPage)
	}

	// And the meta reports what was used, so a client can tell.
	_, zero := getJSON[listingPageBody](t, srv.URL+"/listings?per_page=0")
	if zero.Meta.PerPage != 20 {
		t.Errorf("per_page=0 gave %d, want the default 20", zero.Meta.PerPage)
	}
}

func TestListListingsOnEmptyDatabase(t *testing.T) {
	srv := newTestServer(t, newFakeQuerier())

	res, page := getJSON[listingPageBody](t, srv.URL+"/listings")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	if len(page.Data) != 0 || page.Meta.Total != 0 || page.Meta.TotalPages != 0 {
		t.Errorf("empty database gave %+v with %d rows", page.Meta, len(page.Data))
	}
	if page.Data == nil {
		t.Error("data should be an empty array, not null")
	}
}

func TestGetListingReturnsStructuredDetail(t *testing.T) {
	db := newFakeQuerier()
	srv := newTestServer(t, db)
	created := seedListings(t, db, 1)

	res, listing := getJSON[listingBody](t, srv.URL+"/listings/"+created[0].ID.String())
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}

	// The point of the phase: facts arrive as fields, not as sentences to parse.
	if listing.PriceCents != 80000 {
		t.Errorf("price_cents = %d, want 80000 (cents, not dollars)", listing.PriceCents)
	}
	if listing.StartDate != "2027-01-01" || listing.EndDate != "2027-04-30" {
		t.Errorf("dates = %s..%s, want ISO dates", listing.StartDate, listing.EndDate)
	}
	if listing.BedroomsTotal != 4 || listing.BedroomOf == nil || *listing.BedroomOf != 1 {
		t.Errorf("bedrooms = %d, of = %v; want the parts of \"1 of 4 bed\"", listing.BedroomsTotal, listing.BedroomOf)
	}
	if len(listing.Utilities) != 2 {
		t.Errorf("utilities = %v, want the included bills as a list", listing.Utilities)
	}
	if len(listing.Conditions) != 1 {
		t.Errorf("conditions = %v, want the poster's terms as a list", listing.Conditions)
	}
	if listing.BathType != "shared" || listing.UnitType != "room" {
		t.Errorf("bath_type = %q, unit_type = %q", listing.BathType, listing.UnitType)
	}
}

func TestGetListingUnknownIDReturns404Envelope(t *testing.T) {
	srv := newTestServer(t, newFakeQuerier())

	res, env := getJSON[apierror.Envelope](t, srv.URL+"/listings/"+uuid.NewString())
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", res.StatusCode)
	}
	if env.Code != apierror.CodeNotFound || env.Message == "" {
		t.Errorf("envelope = %+v, want the standard not_found shape", env)
	}
}

func TestGetListingHidesUnpublished(t *testing.T) {
	db := newFakeQuerier()
	srv := newTestServer(t, db)
	created := seedListings(t, db, 1)

	for _, status := range []string{"draft", "paused", "archived"} {
		t.Run(status, func(t *testing.T) {
			db.mu.Lock()
			db.listings[0].Status = status
			db.mu.Unlock()

			// 404 rather than 403: whether a post was taken down, or never
			// existed, is not something a stranger should be able to tell.
			res, env := getJSON[apierror.Envelope](t, srv.URL+"/listings/"+created[0].ID.String())
			if res.StatusCode != http.StatusNotFound {
				t.Fatalf("%s listing: status = %d, want 404", status, res.StatusCode)
			}
			if env.Code != apierror.CodeNotFound {
				t.Errorf("code = %q, want not_found", env.Code)
			}
		})
	}
}

func TestGetListingMalformedIDReturns400(t *testing.T) {
	srv := newTestServer(t, newFakeQuerier())

	res, env := getJSON[apierror.Envelope](t, srv.URL+"/listings/not-a-uuid")
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", res.StatusCode)
	}
	// Even a failure inside generated request binding must use the envelope.
	if env.Code == "" || env.Message == "" {
		t.Errorf("envelope = %+v, want the standard shape", env)
	}
}

func TestListingEndpointsArePublic(t *testing.T) {
	db := newFakeQuerier()
	srv := newTestServer(t, db)
	created := seedListings(t, db, 1)

	// No cookie is sent. Browsing is the first thing a prospective user does;
	// requiring an account to look would defeat the housing hub.
	for _, path := range []string{"/listings", "/listings/" + created[0].ID.String()} {
		res, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusOK {
			t.Errorf("%s returned %d to an anonymous visitor, want 200", path, res.StatusCode)
		}
	}
}
