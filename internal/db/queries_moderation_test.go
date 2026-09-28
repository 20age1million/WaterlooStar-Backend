package db_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/20age1million/WaterlooStar-Backend/internal/db"
	"github.com/20age1million/WaterlooStar-Backend/internal/db/dbtest"
	"github.com/20age1million/WaterlooStar-Backend/internal/db/sqlcgen"
)

// Takedown: the admin post reads, remove and restore, and the filter that takes
// a removed post out of every public read.

func TestAdminPostReadsSeeEverything(t *testing.T) {
	t.Parallel()
	q, _ := dbtest.Queries(t)
	ctx := context.Background()

	owner := dbtest.User(t, q, "owner@uwaterloo.ca", true)
	other := dbtest.User(t, q, "other@uwaterloo.ca", true)
	published := dbtest.Listing(t, q, owner.ID, dbtest.ListingOptions{Title: "Sunny room on Hazel"})
	dbtest.Listing(t, q, owner.ID, dbtest.ListingOptions{Status: "draft"})
	dbtest.Listing(t, q, other.ID, dbtest.ListingOptions{Status: "paused"})
	if _, err := q.RemoveListing(ctx, sqlcgen.RemoveListingParams{ID: published.ID, Reason: strp("Wrong address")}); err != nil {
		t.Fatalf("RemoveListing: %v", err)
	}
	req := dbtest.Request(t, q, other.ID, dbtest.RequestOptions{Status: "draft"})

	listings := func(p sqlcgen.ListListingsForAdminParams) int {
		t.Helper()
		p.Limit = 50
		rows, err := q.ListListingsForAdmin(ctx, p)
		if err != nil {
			t.Fatalf("ListListingsForAdmin: %v", err)
		}
		n, err := q.CountListingsForAdmin(ctx, sqlcgen.CountListingsForAdminParams{Search: p.Search, Status: p.Status, Removed: p.Removed, OwnerID: p.OwnerID})
		if err != nil || int(n) != len(rows) {
			t.Errorf("count %d (%v) disagrees with %d rows for %+v", n, err, len(rows), p)
		}
		return len(rows)
	}
	for name, tc := range map[string]struct {
		p    sqlcgen.ListListingsForAdminParams
		want int
	}{
		"everything":   {sqlcgen.ListListingsForAdminParams{}, 3},
		"removed":      {sqlcgen.ListListingsForAdminParams{Removed: boolp(true)}, 1},
		"not removed":  {sqlcgen.ListListingsForAdminParams{Removed: boolp(false)}, 2},
		"drafts":       {sqlcgen.ListListingsForAdminParams{Status: strp("draft")}, 1},
		"by owner":     {sqlcgen.ListListingsForAdminParams{OwnerID: &owner.ID}, 2},
		"search":       {sqlcgen.ListListingsForAdminParams{Search: strp("sunny")}, 1},
		"removed+mine": {sqlcgen.ListListingsForAdminParams{Removed: boolp(true), OwnerID: &other.ID}, 0},
	} {
		if got := listings(tc.p); got != tc.want {
			t.Errorf("%s: %d, want %d", name, got, tc.want)
		}
	}

	rows, err := q.ListRequestsForAdmin(ctx, sqlcgen.ListRequestsForAdminParams{Limit: 50})
	if err != nil || len(rows) != 1 || rows[0].HousingRequest.ID != req.ID || rows[0].PosterEmail != other.Email {
		t.Fatalf("ListRequestsForAdmin: %v %+v", err, rows)
	}
	if n, err := q.CountRequestsForAdmin(ctx, sqlcgen.CountRequestsForAdminParams{Status: strp("draft")}); err != nil || n != 1 {
		t.Errorf("CountRequestsForAdmin = %d (%v)", n, err)
	}
}

func TestRemoveAndRestoreLeaveStatusAlone(t *testing.T) {
	t.Parallel()
	q, _ := dbtest.Queries(t)
	ctx := context.Background()

	owner := dbtest.User(t, q, "owner@uwaterloo.ca", true)
	l := dbtest.Listing(t, q, owner.ID, dbtest.ListingOptions{Status: "paused"})
	r := dbtest.Request(t, q, owner.ID, dbtest.RequestOptions{})

	removed, err := q.RemoveListing(ctx, sqlcgen.RemoveListingParams{ID: l.ID, Reason: strp("First reason")})
	if err != nil || removed.RemovedAt == nil || removed.Status != "paused" {
		t.Fatalf("RemoveListing: %+v %v", removed, err)
	}
	if _, err := q.RemoveListing(ctx, sqlcgen.RemoveListingParams{ID: l.ID, Reason: strp("Second reason")}); !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("second removal: %v, want no rows", err)
	}
	restored, err := q.RestoreListing(ctx, l.ID)
	if err != nil || restored.RemovedAt != nil || restored.RemovedReason != nil || restored.Status != "paused" {
		t.Fatalf("RestoreListing: %+v %v", restored, err)
	}
	if _, err := q.RestoreListing(ctx, l.ID); !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("second restore: %v, want no rows", err)
	}

	if _, err := q.RemoveRequest(ctx, sqlcgen.RemoveRequestParams{ID: r.ID, Reason: strp("A reason")}); err != nil {
		t.Fatalf("RemoveRequest: %v", err)
	}
	back, err := q.RestoreRequest(ctx, r.ID)
	if err != nil || back.Status != "published" || back.RemovedAt != nil {
		t.Fatalf("RestoreRequest: %+v %v", back, err)
	}

	// A takedown is whole or absent.
	if _, err := q.RemoveListing(ctx, sqlcgen.RemoveListingParams{ID: l.ID}); err == nil {
		t.Error("a takedown without a reason was accepted")
	}
}

func TestARemovedPostLeavesEveryPublicRead(t *testing.T) {
	t.Parallel()
	q, pool := dbtest.Queries(t)
	ctx := context.Background()
	run := db.PoolTx(pool)

	admin := dbtest.User(t, q, "operator@uwaterloo.ca", true)
	owner := dbtest.User(t, q, "owner@uwaterloo.ca", true)
	student := dbtest.User(t, q, "student@uwaterloo.ca", true)
	listing := dbtest.Listing(t, q, owner.ID, dbtest.ListingOptions{})
	request := dbtest.Request(t, q, student.ID, dbtest.RequestOptions{})
	if _, err := q.CreateOffer(ctx, sqlcgen.CreateOfferParams{RequestID: request.ID, ListingID: listing.ID, OwnerID: owner.ID}); err != nil {
		t.Fatalf("CreateOffer: %v", err)
	}

	type view struct {
		listings, requests       int64
		listingOpen, requestOpen bool
		offers, offerCount       int64
		ownerSees                int
	}
	look := func() view {
		t.Helper()
		var v view
		_, v.listings = browse(t, q, sqlcgen.ListListingsParams{})
		rows, total := browseRequests(t, q, sqlcgen.ListRequestsParams{})
		v.requests = total
		if len(rows) == 1 {
			v.offerCount = rows[0].OfferCount
		}
		_, err := q.GetPublishedListing(ctx, listing.ID)
		v.listingOpen = err == nil
		_, err = q.GetPublishedRequest(ctx, request.ID)
		v.requestOpen = err == nil
		v.offers, _ = q.CountOffersForRequest(ctx, request.ID)
		mine, _ := q.ListListingsByOwner(ctx, owner.ID)
		v.ownerSees = len(mine)
		return v
	}
	all := view{1, 1, true, true, 1, 1, 1}
	if got := look(); got != all {
		t.Fatalf("before: %+v", got)
	}

	if err := db.RemovePost(ctx, run, admin.ID, db.SubjectListing, listing.ID, "Address belongs to someone else"); err != nil {
		t.Fatalf("RemovePost listing: %v", err)
	}
	if got, want := look(), (view{0, 1, false, true, 0, 0, 1}); got != want {
		t.Errorf("listing removed: %+v, want %+v", got, want)
	}
	if err := db.RemovePost(ctx, run, admin.ID, db.SubjectRequest, request.ID, "Phone number in the body text"); err != nil {
		t.Fatalf("RemovePost request: %v", err)
	}
	if got := look(); got.requests != 0 || got.requestOpen {
		t.Errorf("request removed: %+v", got)
	}
	if err := db.RemovePost(ctx, run, admin.ID, db.SubjectListing, listing.ID, "Again, a second time"); !errors.Is(err, db.ErrAlreadyRemoved) {
		t.Errorf("second RemovePost: %v", err)
	}

	if err := db.RestorePost(ctx, run, admin.ID, db.SubjectListing, listing.ID, ""); err != nil {
		t.Fatalf("RestorePost listing: %v", err)
	}
	if err := db.RestorePost(ctx, run, admin.ID, db.SubjectRequest, request.ID, "Number was their own"); err != nil {
		t.Fatalf("RestorePost request: %v", err)
	}
	if got := look(); got != all {
		t.Errorf("after restoring: %+v", got)
	}
	if err := db.RestorePost(ctx, run, admin.ID, db.SubjectListing, listing.ID, ""); !errors.Is(err, db.ErrNotRemoved) {
		t.Errorf("second RestorePost: %v", err)
	}

	entries, err := q.ListAdminActionsForSubject(ctx, sqlcgen.ListAdminActionsForSubjectParams{SubjectType: db.SubjectListing, SubjectID: listing.ID})
	if err != nil || len(entries) != 2 {
		t.Fatalf("listing ledger: %v (%d)", err, len(entries))
	}
	restore, remove := entries[0].AdminAction, entries[1].AdminAction
	if restore.Action != db.ActionRestorePost || restore.Reason != db.DefaultRestoreReason {
		t.Errorf("restore entry = %s %q", restore.Action, restore.Reason)
	}
	var detail map[string]any
	_ = json.Unmarshal(remove.Detail, &detail)
	if remove.Action != db.ActionRemovePost || detail["status"] != "published" {
		t.Errorf("remove entry = %s %v", remove.Action, detail)
	}
}
