package db_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/20age1million/WaterlooStar-Backend/internal/db/dbtest"
	"github.com/20age1million/WaterlooStar-Backend/internal/db/sqlcgen"
)

// Listing terms: shorter stays, the bill estimate, and what search does with
// them. These run against PostgreSQL because the rules live in its CHECKs and
// in date arithmetic the in-memory fake only imitates.

func i32(v int32) *int32 { return &v }

func titles(rows []sqlcgen.ListListingsRow) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Listing.Title)
	}
	return out
}

func TestFullCoverFilterHoldsForBothKindsOfListing(t *testing.T) {
	t.Parallel()
	q, _ := dbtest.Queries(t)
	owner := dbtest.User(t, q, "owner@uwaterloo.ca", true)

	dbtest.Listing(t, q, owner.ID, dbtest.ListingOptions{Title: "Whole lease only, Jan to Apr"})
	dbtest.Listing(t, q, owner.ID, dbtest.ListingOptions{
		Title: "Shorter stays, Jan to Aug", EndDate: date(2027, 8, 31), LeaseMonths: 8,
		ShorterStays: true, MinStayMonths: i32(2),
	})

	// A shorter window inside both: both cover it, so both appear.
	rows, total := browse(t, q, sqlcgen.ListListingsParams{StartAfter: ptrTime(date(2027, 1, 1)), EndBefore: ptrTime(date(2027, 2, 28))})
	if total != 2 {
		t.Errorf("Jan–Feb window: %v, want both", titles(rows))
	}
	// A window past both: neither covers it, whatever its terms.
	if rows, total := browse(t, q, sqlcgen.ListListingsParams{StartAfter: ptrTime(date(2027, 1, 1)), EndBefore: ptrTime(date(2027, 9, 30))}); total != 0 {
		t.Errorf("Jan–Sep window: %v, want none", titles(rows))
	}
}

func TestMatchRanksExactWindowListingsFirst(t *testing.T) {
	t.Parallel()
	q, _ := dbtest.Queries(t)
	owner := dbtest.User(t, q, "owner@uwaterloo.ca", true)
	aug := date(2027, 8, 31)

	// Created oldest first, so "newest first" alone would reverse this order:
	// the ranking has to be what puts the exact ones on top.
	dbtest.Listing(t, q, owner.ID, dbtest.ListingOptions{Title: "A exact dates"})
	dbtest.Listing(t, q, owner.ID, dbtest.ListingOptions{Title: "C shorter stays from 2", EndDate: aug, LeaseMonths: 8, ShorterStays: true, MinStayMonths: i32(2)})
	dbtest.Listing(t, q, owner.ID, dbtest.ListingOptions{Title: "D shorter stays from 6", EndDate: aug, LeaseMonths: 8, ShorterStays: true, MinStayMonths: i32(6)})
	dbtest.Listing(t, q, owner.ID, dbtest.ListingOptions{Title: "B whole lease to Aug", EndDate: aug, LeaseMonths: 8})

	window := sqlcgen.ListListingsParams{StartAfter: ptrTime(date(2027, 1, 1)), EndBefore: ptrTime(date(2027, 4, 30)), Sort: "match"}
	rows, _ := browse(t, q, window)
	got := titles(rows)
	want := []string{"C shorter stays from 2", "A exact dates", "B whole lease to Aug", "D shorter stays from 6"}
	if len(got) != 4 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] || got[3] != want[3] {
		t.Errorf("match order = %v\n            want %v\n(a minimum of 6 months does not fit a 4-month window)", got, want)
	}

	// A window exactly the minimum long counts: Jan 1 + 2 months - 1 day is Feb 28.
	twoMonths := sqlcgen.ListListingsParams{StartAfter: ptrTime(date(2027, 1, 1)), EndBefore: ptrTime(date(2027, 2, 28)), Sort: "match"}
	if rows, _ := browse(t, q, twoMonths); titles(rows)[0] != "C shorter stays from 2" {
		t.Errorf("two-month window led with %v", titles(rows))
	}

	// Without a window, match is newest first, as before.
	if rows, _ := browse(t, q, sqlcgen.ListListingsParams{Sort: "match"}); titles(rows)[0] != "B whole lease to Aug" {
		t.Errorf("match without a window led with %v, want the newest", titles(rows))
	}
}

func TestAllInCostNeverGuessesAnUnstatedEstimate(t *testing.T) {
	t.Parallel()
	q, _ := dbtest.Queries(t)
	owner := dbtest.User(t, q, "owner@uwaterloo.ca", true)

	dbtest.Listing(t, q, owner.ID, dbtest.ListingOptions{Title: "800 plus 50", PriceCents: 80000, BillsEstimateCents: i32(5000)})
	dbtest.Listing(t, q, owner.ID, dbtest.ListingOptions{Title: "700, bills unstated", PriceCents: 70000})
	dbtest.Listing(t, q, owner.ID, dbtest.ListingOptions{Title: "900, all included", PriceCents: 90000, BillsEstimateCents: i32(0)})

	// The cheapest rent is excluded: its monthly cost is unknown, not $700.
	if rows, total := browse(t, q, sqlcgen.ListListingsParams{AllInMax: i32(86000)}); total != 1 || rows[0].Listing.Title != "800 plus 50" {
		t.Errorf("all-in under $860 = %v", titles(rows))
	}
	rows, _ := browse(t, q, sqlcgen.ListListingsParams{Sort: "allInAsc"})
	if got := titles(rows); got[0] != "800 plus 50" || got[1] != "900, all included" || got[2] != "700, bills unstated" {
		t.Errorf("allInAsc = %v, want known costs cheapest first, unknown last", got)
	}
}

func TestAvailabilityCountsEachMonthAListingIsOpen(t *testing.T) {
	t.Parallel()
	q, _ := dbtest.Queries(t)
	ctx := context.Background()
	owner := dbtest.User(t, q, "owner@uwaterloo.ca", true)

	dbtest.Listing(t, q, owner.ID, dbtest.ListingOptions{Title: "January only", StartDate: date(2027, 1, 1), EndDate: date(2027, 1, 31), LeaseMonths: 1})
	dbtest.Listing(t, q, owner.ID, dbtest.ListingOptions{Title: "Mid Feb to mid Mar", StartDate: date(2027, 2, 15), EndDate: date(2027, 3, 10), LeaseMonths: 1, PriceCents: 150000})
	dbtest.Listing(t, q, owner.ID, dbtest.ListingOptions{Title: "A draft", Status: "draft"})

	rows, err := q.ListingAvailabilityByMonth(ctx, sqlcgen.ListingAvailabilityByMonthParams{Year: 2027})
	if err != nil {
		t.Fatalf("ListingAvailabilityByMonth: %v", err)
	}
	if len(rows) != 12 {
		t.Fatalf("%d months, want twelve", len(rows))
	}
	got := make([]int32, 12)
	for _, r := range rows {
		got[r.Month-1] = r.Open
	}
	// A listing ending Jan 31 is not open in February; the draft counts nowhere.
	want := []int32{1, 1, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("by month = %v, want %v", got, want)
		}
	}

	filtered, err := q.ListingAvailabilityByMonth(ctx, sqlcgen.ListingAvailabilityByMonthParams{Year: 2027, PriceMax: i32(100000)})
	if err != nil || len(filtered) != 12 || filtered[1].Open != 0 || filtered[0].Open != 1 {
		t.Errorf("with a rent ceiling: %+v (%v)", filtered, err)
	}

	other, err := q.ListingAvailabilityByMonth(ctx, sqlcgen.ListingAvailabilityByMonthParams{Year: 2031})
	if err != nil || len(other) != 12 || other[0].Open != 0 {
		t.Errorf("an empty year: %+v (%v)", other, err)
	}
}

func TestListingTermsConstraints(t *testing.T) {
	t.Parallel()
	q, _ := dbtest.Queries(t)
	ctx := context.Background()
	owner := dbtest.User(t, q, "owner@uwaterloo.ca", true)
	base := dbtest.Listing(t, q, owner.ID, dbtest.ListingOptions{})

	refused := func(name string, p sqlcgen.UpdateListingParams) {
		t.Helper()
		p.ID = base.ID
		_, err := q.UpdateListing(ctx, p)
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "23514" {
			t.Errorf("%s: %v, want a CHECK violation", name, err)
		}
	}
	yes, no := true, false
	refused("shorter stays without a minimum", sqlcgen.UpdateListingParams{ShorterStays: &yes})
	refused("a minimum without shorter stays", sqlcgen.UpdateListingParams{SetMinStay: true, MinStayMonths: i32(2)})
	refused("a minimum longer than the lease", sqlcgen.UpdateListingParams{ShorterStays: &yes, SetMinStay: true, MinStayMonths: i32(5)})
	refused("an estimate over $1,000", sqlcgen.UpdateListingParams{SetBillsEstimate: true, BillsEstimateCents: i32(100001)})

	// Set, then cleared back to NULL — which COALESCE alone could not do.
	on, err := q.UpdateListing(ctx, sqlcgen.UpdateListingParams{ID: base.ID, ShorterStays: &yes, SetMinStay: true, MinStayMonths: i32(2), SetBillsEstimate: true, BillsEstimateCents: i32(4000)})
	if err != nil || !on.ShorterStays || *on.MinStayMonths != 2 || *on.BillsEstimateCents != 4000 {
		t.Fatalf("set terms: %+v %v", on, err)
	}
	untouched, err := q.UpdateListing(ctx, sqlcgen.UpdateListingParams{ID: base.ID, Title: ptrString("A new title for the room")})
	if err != nil || untouched.MinStayMonths == nil || untouched.BillsEstimateCents == nil {
		t.Errorf("an unrelated edit cleared the terms: %+v %v", untouched, err)
	}
	off, err := q.UpdateListing(ctx, sqlcgen.UpdateListingParams{ID: base.ID, ShorterStays: &no, SetMinStay: true, SetBillsEstimate: true})
	if err != nil || off.ShorterStays || off.MinStayMonths != nil || off.BillsEstimateCents != nil {
		t.Errorf("clear terms: %+v %v", off, err)
	}
}
