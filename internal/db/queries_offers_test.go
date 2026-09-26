package db_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/20age1million/WaterlooStar-Backend/internal/db/dbtest"
	"github.com/20age1million/WaterlooStar-Backend/internal/db/sqlcgen"
)

// Offers: an owner answering a request with one of their own listings.
//
// The rule worth testing hardest is that an offer is only as alive as its
// listing. Nothing withdraws an offer when a place is taken down — the read
// query simply stops returning it — so if that join ever loosens, students
// start seeing rooms that no longer exist.

func TestOfferRoundTrip(t *testing.T) {
	t.Parallel()
	q, _ := dbtest.Queries(t)
	ctx := context.Background()

	student := dbtest.User(t, q, "student@uwaterloo.ca", true)
	owner := dbtest.User(t, q, "landlord@uwaterloo.ca", true)
	request := dbtest.Request(t, q, student.ID, dbtest.RequestOptions{})
	listing := dbtest.Listing(t, q, owner.ID, dbtest.ListingOptions{Title: "Room two doors from the library"})

	offer, err := q.CreateOffer(ctx, sqlcgen.CreateOfferParams{
		RequestID: request.ID, ListingID: listing.ID, OwnerID: owner.ID,
		Note: "Quiet side of the house.",
	})
	if err != nil {
		t.Fatalf("CreateOffer: %v", err)
	}

	rows, err := q.ListOffersForRequest(ctx, request.ID)
	if err != nil || len(rows) != 1 {
		t.Fatalf("ListOffersForRequest: %v (%d rows)", err, len(rows))
	}
	// The listing travels with the offer, so the student sees a real place.
	if rows[0].Listing.Title != listing.Title || rows[0].OwnerUsername != owner.Username {
		t.Errorf("offer row = %+v", rows[0])
	}

	count, err := q.CountOffersForRequest(ctx, request.ID)
	if err != nil || count != 1 {
		t.Fatalf("CountOffersForRequest = %d (%v)", count, err)
	}

	byID, err := q.GetOffer(ctx, offer.ID)
	if err != nil || byID.OwnerID != owner.ID {
		t.Fatalf("GetOffer: %v", err)
	}

	mine, err := q.ListOffersByOwner(ctx, owner.ID)
	if err != nil || len(mine) != 1 {
		t.Fatalf("ListOffersByOwner: %v (%d rows)", err, len(mine))
	}

	existing, err := q.GetOfferForListing(ctx, sqlcgen.GetOfferForListingParams{
		RequestID: request.ID, ListingID: listing.ID,
	})
	if err != nil || existing.ID != offer.ID {
		t.Fatalf("GetOfferForListing: %v", err)
	}
}

func TestTakingTheListingDownRemovesItsOffers(t *testing.T) {
	t.Parallel()
	q, _ := dbtest.Queries(t)
	ctx := context.Background()

	student := dbtest.User(t, q, "student2@uwaterloo.ca", true)
	owner := dbtest.User(t, q, "landlord2@uwaterloo.ca", true)
	request := dbtest.Request(t, q, student.ID, dbtest.RequestOptions{})
	listing := dbtest.Listing(t, q, owner.ID, dbtest.ListingOptions{})

	if _, err := q.CreateOffer(ctx, sqlcgen.CreateOfferParams{
		RequestID: request.ID, ListingID: listing.ID, OwnerID: owner.ID,
	}); err != nil {
		t.Fatalf("CreateOffer: %v", err)
	}

	// Archive the place. Nothing touches the offer.
	if _, err := q.SetListingStatus(ctx, sqlcgen.SetListingStatusParams{
		ID: listing.ID, Status: "archived",
	}); err != nil {
		t.Fatalf("SetListingStatus: %v", err)
	}

	rows, err := q.ListOffersForRequest(ctx, request.ID)
	if err != nil {
		t.Fatalf("ListOffersForRequest: %v", err)
	}
	if len(rows) != 0 {
		t.Error("an offer outlived the listing behind it")
	}
	if count, _ := q.CountOffersForRequest(ctx, request.ID); count != 0 {
		t.Errorf("count = %d, want 0", count)
	}

	// Pausing hides it too, and republishing brings the offer back — the offer
	// follows the place rather than the other way round.
	if _, err := q.SetListingStatus(ctx, sqlcgen.SetListingStatusParams{
		ID: listing.ID, Status: "published",
	}); err != nil {
		t.Fatalf("republish: %v", err)
	}
	if count, _ := q.CountOffersForRequest(ctx, request.ID); count != 1 {
		t.Errorf("count = %d after republishing, want 1", count)
	}
}

func TestWithdrawingAnOfferHidesIt(t *testing.T) {
	t.Parallel()
	q, _ := dbtest.Queries(t)
	ctx := context.Background()

	student := dbtest.User(t, q, "student3@uwaterloo.ca", true)
	owner := dbtest.User(t, q, "landlord3@uwaterloo.ca", true)
	request := dbtest.Request(t, q, student.ID, dbtest.RequestOptions{})
	listing := dbtest.Listing(t, q, owner.ID, dbtest.ListingOptions{})

	offer, err := q.CreateOffer(ctx, sqlcgen.CreateOfferParams{
		RequestID: request.ID, ListingID: listing.ID, OwnerID: owner.ID,
	})
	if err != nil {
		t.Fatalf("CreateOffer: %v", err)
	}

	withdrawn, err := q.WithdrawOffer(ctx, offer.ID)
	if err != nil {
		t.Fatalf("WithdrawOffer: %v", err)
	}
	if withdrawn.WithdrawnAt == nil {
		t.Fatal("withdrawn_at was not stamped")
	}

	rows, _ := q.ListOffersForRequest(ctx, request.ID)
	if len(rows) != 0 {
		t.Error("a withdrawn offer was still shown")
	}

	// The row stays, so the unique constraint still applies: withdrawing is not
	// a way round "one offer per listing per request".
	if _, err := q.GetOffer(ctx, offer.ID); err != nil {
		t.Errorf("the withdrawn row should still exist: %v", err)
	}
	if _, err := q.WithdrawOffer(ctx, offer.ID); !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("withdrawing twice should match nothing: %v", err)
	}
}

func TestTheSameListingCannotBeOfferedTwice(t *testing.T) {
	t.Parallel()
	q, _ := dbtest.Queries(t)
	ctx := context.Background()

	student := dbtest.User(t, q, "student4@uwaterloo.ca", true)
	owner := dbtest.User(t, q, "landlord4@uwaterloo.ca", true)
	request := dbtest.Request(t, q, student.ID, dbtest.RequestOptions{})
	listing := dbtest.Listing(t, q, owner.ID, dbtest.ListingOptions{})

	params := sqlcgen.CreateOfferParams{RequestID: request.ID, ListingID: listing.ID, OwnerID: owner.ID}
	if _, err := q.CreateOffer(ctx, params); err != nil {
		t.Fatalf("CreateOffer: %v", err)
	}
	// Offering the same place twice is noise, not emphasis. The handler says so
	// in words; the constraint is what makes it true.
	if _, err := q.CreateOffer(ctx, params); err == nil {
		t.Fatal("the same listing was offered twice against one request")
	}
}

func TestOffersCountedOnARequestAreTheVisibleOnes(t *testing.T) {
	t.Parallel()
	q, _ := dbtest.Queries(t)
	ctx := context.Background()

	student := dbtest.User(t, q, "student5@uwaterloo.ca", true)
	owner := dbtest.User(t, q, "landlord5@uwaterloo.ca", true)
	request := dbtest.Request(t, q, student.ID, dbtest.RequestOptions{})

	live := dbtest.Listing(t, q, owner.ID, dbtest.ListingOptions{Title: "Still available"})
	gone := dbtest.Listing(t, q, owner.ID, dbtest.ListingOptions{Title: "Taken down since"})
	dropped := dbtest.Listing(t, q, owner.ID, dbtest.ListingOptions{Title: "Offered then withdrawn"})

	for _, l := range []sqlcgen.Listing{live, gone, dropped} {
		if _, err := q.CreateOffer(ctx, sqlcgen.CreateOfferParams{
			RequestID: request.ID, ListingID: l.ID, OwnerID: owner.ID,
		}); err != nil {
			t.Fatalf("CreateOffer: %v", err)
		}
	}
	if _, err := q.SetListingStatus(ctx, sqlcgen.SetListingStatusParams{ID: gone.ID, Status: "archived"}); err != nil {
		t.Fatalf("archive: %v", err)
	}
	droppedOffer, _ := q.GetOfferForListing(ctx, sqlcgen.GetOfferForListingParams{
		RequestID: request.ID, ListingID: dropped.ID,
	})
	if _, err := q.WithdrawOffer(ctx, droppedOffer.ID); err != nil {
		t.Fatalf("WithdrawOffer: %v", err)
	}

	// The count the student sees on their own request, and the browse query's
	// count, must both be one — not three.
	rows, _ := q.ListRequestsByPoster(ctx, student.ID)
	if len(rows) != 1 || rows[0].OfferCount != 1 {
		t.Errorf("own request shows %d offers, want 1", rows[0].OfferCount)
	}

	public, err := q.GetPublishedRequest(ctx, request.ID)
	if err != nil {
		t.Fatalf("GetPublishedRequest: %v", err)
	}
	if public.OfferCount != 1 {
		t.Errorf("public request shows %d offers, want 1", public.OfferCount)
	}

	browsed, _ := browseRequests(t, q, sqlcgen.ListRequestsParams{})
	if len(browsed) != 1 || browsed[0].OfferCount != 1 {
		t.Errorf("browse shows %d offers", browsed[0].OfferCount)
	}
}
