package db_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/20age1million/WaterlooStar-Backend/internal/db/dbtest"
	"github.com/20age1million/WaterlooStar-Backend/internal/db/sqlcgen"
)

// The listing queries, against a real PostgreSQL.
//
// The discovery filters are the interesting part: they are one prepared
// statement with a dozen optional parameters, and no fake can say whether
// PostgreSQL will accept it.

func ptrBool(b bool) *bool           { return &b }
func ptrInt32(i int32) *int32        { return &i }
func ptrString(s string) *string     { return &s }
func ptrTime(t time.Time) *time.Time { return &t }

func date(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// browse runs the public listing query with the given filters and returns how
// many rows came back, alongside the count query's answer. The two must agree,
// which is the bug this shape of test catches.
func browse(t *testing.T, q *sqlcgen.Queries, list sqlcgen.ListListingsParams) ([]sqlcgen.ListListingsRow, int64) {
	t.Helper()
	ctx := context.Background()

	if list.Sort == "" {
		list.Sort = "new"
	}
	if list.Limit == 0 {
		list.Limit = 50
	}

	rows, err := q.ListListings(ctx, list)
	if err != nil {
		t.Fatalf("ListListings: %v", err)
	}

	total, err := q.CountListings(ctx, sqlcgen.CountListingsParams{
		Search: list.Search, StartAfter: list.StartAfter, EndBefore: list.EndBefore,
		PriceMin: list.PriceMin, PriceMax: list.PriceMax, DistanceMax: list.DistanceMax,
		BedroomsMin: list.BedroomsMin, Furnished: list.Furnished, Parking: list.Parking,
		Pets: list.Pets, Laundry: list.Laundry, Utilities: list.Utilities,
		VerifiedOnly: list.VerifiedOnly,
	})
	if err != nil {
		t.Fatalf("CountListings: %v", err)
	}

	if int64(len(rows)) != total && int64(len(rows)) < int64(list.Limit) {
		t.Errorf("ListListings returned %d rows but CountListings says %d", len(rows), total)
	}
	return rows, total
}

func TestListingRoundTrip(t *testing.T) {
	t.Parallel()
	q, _ := dbtest.Queries(t)
	ctx := context.Background()

	owner := dbtest.User(t, q, "owner@uwaterloo.ca", true)
	listing := dbtest.Listing(t, q, owner.ID, dbtest.ListingOptions{Title: "Bright room near campus"})

	published, err := q.GetPublishedListing(ctx, listing.ID)
	if err != nil {
		t.Fatalf("GetPublishedListing: %v", err)
	}
	if published.OwnerUsername != owner.Username {
		t.Errorf("owner = %q, want %q", published.OwnerUsername, owner.Username)
	}

	forOwner, err := q.GetListingForOwner(ctx, listing.ID)
	if err != nil || forOwner.ID != listing.ID {
		t.Fatalf("GetListingForOwner: %v", err)
	}

	mine, err := q.ListListingsByOwner(ctx, owner.ID)
	if err != nil || len(mine) != 1 {
		t.Fatalf("ListListingsByOwner: %v (%d rows)", err, len(mine))
	}
}

func TestSetListingStatusHidesAndRestores(t *testing.T) {
	t.Parallel()
	q, _ := dbtest.Queries(t)
	ctx := context.Background()

	// This query is why this package exists. Phase 4 shipped it using one
	// parameter as both varchar and ::text; PostgreSQL refused to type it, every
	// handler test passed, and pausing a listing silently did nothing.
	owner := dbtest.User(t, q, "pauser@uwaterloo.ca", true)
	listing := dbtest.Listing(t, q, owner.ID, dbtest.ListingOptions{Title: "A room to pause"})

	paused, err := q.SetListingStatus(ctx, sqlcgen.SetListingStatusParams{ID: listing.ID, Status: "paused"})
	if err != nil {
		t.Fatalf("SetListingStatus: %v", err)
	}
	if paused.Status != "paused" {
		t.Fatalf("status = %q, want paused", paused.Status)
	}

	if _, err := q.GetPublishedListing(ctx, listing.ID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("a paused listing was still public: %v", err)
	}

	// A listing created as published carries no published_at: CreateListing
	// never sets the column, and SetListingStatus is the only thing that does.
	// Nothing reads it today, so nothing is broken — but the first query that
	// sorts by it will find most rows empty. Recorded in the phase notes.
	if listing.PublishedAt != nil {
		t.Error("CreateListing now stamps published_at — update this test and the phase notes")
	}

	republished, err := q.SetListingStatus(ctx, sqlcgen.SetListingStatusParams{ID: listing.ID, Status: "published"})
	if err != nil {
		t.Fatalf("SetListingStatus back to published: %v", err)
	}
	if republished.PublishedAt == nil {
		t.Fatal("publishing did not stamp published_at")
	}
	if _, err := q.GetPublishedListing(ctx, listing.ID); err != nil {
		t.Fatalf("a republished listing was not public: %v", err)
	}

	// Stamped once and never moved: pausing and republishing must not make an
	// old post look new.
	if _, err := q.SetListingStatus(ctx, sqlcgen.SetListingStatusParams{ID: listing.ID, Status: "paused"}); err != nil {
		t.Fatalf("second pause: %v", err)
	}
	again, err := q.SetListingStatus(ctx, sqlcgen.SetListingStatusParams{ID: listing.ID, Status: "published"})
	if err != nil {
		t.Fatalf("second publish: %v", err)
	}
	if !again.PublishedAt.Equal(*republished.PublishedAt) {
		t.Error("published_at moved when the listing was republished")
	}
}

func TestUpdateListingChangesOnlyWhatIsGiven(t *testing.T) {
	t.Parallel()
	q, _ := dbtest.Queries(t)
	ctx := context.Background()

	owner := dbtest.User(t, q, "editor@uwaterloo.ca", true)
	listing := dbtest.Listing(t, q, owner.ID, dbtest.ListingOptions{
		Title: "Room with a desk", PriceCents: 84500,
	})

	updated, err := q.UpdateListing(ctx, sqlcgen.UpdateListingParams{
		ID: listing.ID, PriceCents: ptrInt32(79900),
	})
	if err != nil {
		t.Fatalf("UpdateListing: %v", err)
	}

	// Every other column is COALESCE'd against itself. If that ever breaks,
	// an edit becomes a wipe.
	if updated.PriceCents != 79900 {
		t.Errorf("price = %d, want 79900", updated.PriceCents)
	}
	if updated.Title != listing.Title {
		t.Errorf("title changed to %q", updated.Title)
	}
	if updated.AddressLine != listing.AddressLine || updated.BedroomsTotal != listing.BedroomsTotal {
		t.Error("an untouched column changed")
	}
}

func TestBrowseFiltersAgreeWithTheirCount(t *testing.T) {
	t.Parallel()
	q, _ := dbtest.Queries(t)

	owner := dbtest.User(t, q, "filters@uwaterloo.ca", true)
	dbtest.Listing(t, q, owner.ID, dbtest.ListingOptions{
		Title: "Cheap room by the plaza", PriceCents: 60000,
		DistanceM: dbtest.Metres(500), Furnished: true, Utilities: []string{"internet", "hydro"},
	})
	dbtest.Listing(t, q, owner.ID, dbtest.ListingOptions{
		Title: "Expensive loft far away", PriceCents: 200000,
		DistanceM: dbtest.Metres(6000), Bedrooms: 1,
	})
	dbtest.Listing(t, q, owner.ID, dbtest.ListingOptions{
		Title: "Paused room", Status: "paused",
	})

	all, total := browse(t, q, sqlcgen.ListListingsParams{})
	if total != 2 {
		t.Fatalf("public total = %d, want 2 — a paused listing is not public", total)
	}
	if len(all) != 2 {
		t.Fatalf("rows = %d, want 2", len(all))
	}

	cases := map[string]struct {
		params sqlcgen.ListListingsParams
		want   int64
	}{
		"under $1000":        {sqlcgen.ListListingsParams{PriceMax: ptrInt32(100000)}, 1},
		"within 1 km":        {sqlcgen.ListListingsParams{DistanceMax: ptrInt32(1000)}, 1},
		"furnished":          {sqlcgen.ListListingsParams{Furnished: ptrBool(true)}, 1},
		"two utilities":      {sqlcgen.ListListingsParams{Utilities: []string{"internet", "hydro"}}, 1},
		"impossible utility": {sqlcgen.ListListingsParams{Utilities: []string{"gas"}}, 0},
		"search hits one":    {sqlcgen.ListListingsParams{Search: ptrString("plaza")}, 1},
		"search hits none":   {sqlcgen.ListListingsParams{Search: ptrString("submarine")}, 0},
		"three bedrooms up":  {sqlcgen.ListListingsParams{BedroomsMin: ptrInt32(3)}, 1},
		"verified owners":    {sqlcgen.ListListingsParams{VerifiedOnly: ptrBool(true)}, 2},
		// A distance filter excludes listings with no recorded distance rather
		// than quietly including them.
		"everything at once": {sqlcgen.ListListingsParams{
			PriceMax: ptrInt32(100000), DistanceMax: ptrInt32(1000), Furnished: ptrBool(true),
			Utilities: []string{"internet"}, Search: ptrString("room"),
		}, 1},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			rows, total := browse(t, q, tc.params)
			if total != tc.want {
				t.Errorf("total = %d, want %d", total, tc.want)
			}
			if int64(len(rows)) != tc.want {
				t.Errorf("rows = %d, want %d", len(rows), tc.want)
			}
		})
	}
}

func TestBrowseSortsAndPages(t *testing.T) {
	t.Parallel()
	q, _ := dbtest.Queries(t)

	owner := dbtest.User(t, q, "sorter@uwaterloo.ca", true)
	for _, price := range []int32{120000, 80000, 100000} {
		dbtest.Listing(t, q, owner.ID, dbtest.ListingOptions{
			Title: "Room", PriceCents: price, DistanceM: dbtest.Metres(price / 100),
		})
	}

	// Every sort the API offers has to be a value PostgreSQL accepts in the
	// ORDER BY CASE; a typo here is a runtime error, not a compile error.
	for _, sort := range []string{"new", "priceAsc", "priceDesc", "distance", "match"} {
		t.Run(sort, func(t *testing.T) {
			rows, _ := browse(t, q, sqlcgen.ListListingsParams{Sort: sort})
			if len(rows) != 3 {
				t.Fatalf("rows = %d, want 3", len(rows))
			}
			if sort == "priceAsc" && rows[0].Listing.PriceCents != 80000 {
				t.Errorf("cheapest first gave %d", rows[0].Listing.PriceCents)
			}
			if sort == "priceDesc" && rows[0].Listing.PriceCents != 120000 {
				t.Errorf("dearest first gave %d", rows[0].Listing.PriceCents)
			}
		})
	}

	page, err := q.ListListings(context.Background(), sqlcgen.ListListingsParams{
		Sort: "priceAsc", Limit: 2, Offset: 2,
	})
	if err != nil {
		t.Fatalf("ListListings paging: %v", err)
	}
	if len(page) != 1 || page[0].Listing.PriceCents != 120000 {
		t.Errorf("the last page held %d rows", len(page))
	}
}

func TestPhotoQueriesReturnNothingForAListingWithNone(t *testing.T) {
	t.Parallel()
	q, _ := dbtest.Queries(t)
	ctx := context.Background()

	owner := dbtest.User(t, q, "gallery@uwaterloo.ca", true)
	listing := dbtest.Listing(t, q, owner.ID, dbtest.ListingOptions{})

	// Photos are deferred until a storage provider is chosen, so these queries
	// only ever return empty today. They are still executed here, because the
	// point of this package is that no query ships unexecuted.
	photos, err := q.ListPhotosForListing(ctx, listing.ID)
	if err != nil {
		t.Fatalf("ListPhotosForListing: %v", err)
	}
	if len(photos) != 0 {
		t.Errorf("photos = %d, want none", len(photos))
	}

	batch, err := q.ListPhotosForListings(ctx, []uuid.UUID{listing.ID})
	if err != nil {
		t.Fatalf("ListPhotosForListings: %v", err)
	}
	if len(batch) != 0 {
		t.Errorf("photos = %d, want none", len(batch))
	}
}

func TestDeleteAllListingsEmptiesTheTable(t *testing.T) {
	t.Parallel()
	q, _ := dbtest.Queries(t)

	// What `cmd/seed` calls before reseeding.
	owner := dbtest.User(t, q, "reseed@uwaterloo.ca", true)
	dbtest.Listing(t, q, owner.ID, dbtest.ListingOptions{})

	if err := q.DeleteAllListings(context.Background()); err != nil {
		t.Fatalf("DeleteAllListings: %v", err)
	}
	if _, total := browse(t, q, sqlcgen.ListListingsParams{}); total != 0 {
		t.Errorf("total = %d after deleting everything", total)
	}
}
