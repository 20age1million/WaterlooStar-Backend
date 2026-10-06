package db_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/20age1million/WaterlooStar-Backend/internal/db"
	"github.com/20age1million/WaterlooStar-Backend/internal/db/dbtest"
	"github.com/20age1million/WaterlooStar-Backend/internal/db/sqlcgen"
)

// Suspension, verification by hand, and the filter that takes a suspended
// account's posts out of public view. The handler tests prove the same against
// the in-memory fake; these prove the SQL, which is where the rule lives.

func TestSuspendReinstateAndVerifyQueries(t *testing.T) {
	t.Parallel()
	q, _ := dbtest.Queries(t)
	ctx := context.Background()

	admin := dbtest.User(t, q, "operator@uwaterloo.ca", true)
	user := dbtest.User(t, q, "subject@uwaterloo.ca", false)

	first, err := q.SuspendUser(ctx, sqlcgen.SuspendUserParams{ID: user.ID, ActorID: &admin.ID, Reason: strp("First reason given")})
	if err != nil || first.SuspendedAt == nil || first.SuspendedBy == nil || *first.SuspendedBy != admin.ID {
		t.Fatalf("SuspendUser: %+v, %v", first, err)
	}

	// Suspending again matches nothing, and the original time and reason stand.
	if _, err := q.SuspendUser(ctx, sqlcgen.SuspendUserParams{ID: user.ID, ActorID: &admin.ID, Reason: strp("A later reason")}); !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("re-suspend: %v, want no rows", err)
	}
	still, _ := q.GetUserByID(ctx, user.ID)
	if *still.SuspendReason != "First reason given" || !still.SuspendedAt.Equal(*first.SuspendedAt) {
		t.Errorf("re-suspend overwrote the original: %v %v", *still.SuspendReason, still.SuspendedAt)
	}

	back, err := q.ReinstateUser(ctx, user.ID)
	if err != nil || back.SuspendedAt != nil || back.SuspendedBy != nil || back.SuspendReason != nil {
		t.Fatalf("ReinstateUser left something behind: %+v, %v", back, err)
	}
	if _, err := q.ReinstateUser(ctx, user.ID); !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("reinstating an active account: %v, want no rows", err)
	}

	verified, err := q.VerifyUserAsAdmin(ctx, user.ID)
	if err != nil || !verified.Verified {
		t.Fatalf("VerifyUserAsAdmin: %v", err)
	}
	if _, err := q.VerifyUserAsAdmin(ctx, user.ID); !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("verifying twice: %v, want no rows", err)
	}

	// A suspension is whole or absent: the CHECK refuses half of one.
	if _, err := q.SuspendUser(ctx, sqlcgen.SuspendUserParams{ID: user.ID, ActorID: &admin.ID}); err == nil {
		t.Error("a suspension without a reason was accepted")
	}
}

func TestASuspendedAccountsPostsLeavePublicReads(t *testing.T) {
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
		listings, requests           int64
		listingOpen, requestOpen     bool
		offers, offerCount, ownCount int64
		ownerSees, posterSees        int
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
		offers, _ := q.ListOffersForRequest(ctx, request.ID)
		v.offers = int64(len(offers))
		if n, _ := q.CountOffersForRequest(ctx, request.ID); n != v.offers {
			t.Errorf("CountOffersForRequest = %d, ListOffersForRequest = %d", n, v.offers)
		}
		mine, _ := q.ListRequestsByPoster(ctx, student.ID)
		if len(mine) == 1 {
			v.ownCount = mine[0].OfferCount
		}
		// The authors' own views, which suspension must not touch.
		byOwner, _ := q.ListListingsByOwner(ctx, owner.ID)
		v.ownerSees = len(byOwner)
		v.posterSees = len(mine)
		return v
	}

	everything := view{listings: 1, requests: 1, listingOpen: true, requestOpen: true,
		offers: 1, offerCount: 1, ownCount: 1, ownerSees: 1, posterSees: 1}
	if got := look(); got != everything {
		t.Fatalf("before: %+v", got)
	}

	// Suspend the owner: their listing and their offer go; the student's
	// request stays, now with no offers on it.
	if _, err := db.Suspend(ctx, run, admin.ID, owner.ID, "Scam listing reported twice"); err != nil {
		t.Fatalf("Suspend owner: %v", err)
	}
	want := view{listings: 0, requests: 1, listingOpen: false, requestOpen: true,
		offers: 0, offerCount: 0, ownCount: 0, ownerSees: 1, posterSees: 1}
	if got := look(); got != want {
		t.Errorf("owner suspended: %+v\n want %+v", got, want)
	}

	// Suspend the student too: their request goes, but not from their own view.
	if _, err := db.Suspend(ctx, run, admin.ID, student.ID, "Harassing an owner in messages"); err != nil {
		t.Fatalf("Suspend student: %v", err)
	}
	if got := look(); got.requests != 0 || got.requestOpen || got.posterSees != 1 {
		t.Errorf("student suspended: %+v", got)
	}

	// Reinstating both restores exactly what was hidden.
	for _, u := range []sqlcgen.User{owner, student} {
		if _, err := db.Reinstate(ctx, run, admin.ID, u.ID, "Appeal upheld on review"); err != nil {
			t.Fatalf("Reinstate: %v", err)
		}
	}
	if got := look(); got != everything {
		t.Errorf("after reinstating: %+v", got)
	}

	// Their statuses were never touched.
	l, _ := q.GetListingForOwner(ctx, listing.ID)
	if l.Status != "published" {
		t.Errorf("listing status = %q after the round trip", l.Status)
	}
}

func TestAccountActionsEndSessionsAndLog(t *testing.T) {
	t.Parallel()
	q, pool := dbtest.Queries(t)
	ctx := context.Background()
	run := db.PoolTx(pool)

	admin := dbtest.User(t, q, "operator@uwaterloo.ca", true)
	user := dbtest.User(t, q, "subject@uwaterloo.ca", false)

	hash := []byte("a refresh token hash for this test")
	if err := q.CreateRefreshToken(ctx, sqlcgen.CreateRefreshTokenParams{
		TokenHash: hash, UserID: user.ID, ExpiresAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("CreateRefreshToken: %v", err)
	}

	if _, err := db.Suspend(ctx, run, user.ID, user.ID, "Trying to suspend myself"); !errors.Is(err, db.ErrSelfAction) {
		t.Errorf("self-suspension: %v, want ErrSelfAction", err)
	}
	if _, err := db.Suspend(ctx, run, admin.ID, user.ID, "Scam listing reported twice"); err != nil {
		t.Fatalf("Suspend: %v", err)
	}
	if _, err := q.GetLiveRefreshToken(ctx, hash); !errors.Is(err, pgx.ErrNoRows) {
		t.Error("the suspended account's session survived")
	}
	if _, err := db.Suspend(ctx, run, admin.ID, user.ID, "Scam listing reported twice"); !errors.Is(err, db.ErrAlreadySuspended) {
		t.Errorf("second suspension: %v", err)
	}
	if _, err := db.Reinstate(ctx, run, admin.ID, user.ID, "Appeal upheld on review"); err != nil {
		t.Fatalf("Reinstate: %v", err)
	}
	if _, err := db.Reinstate(ctx, run, admin.ID, user.ID, "Appeal upheld on review"); !errors.Is(err, db.ErrNotSuspended) {
		t.Errorf("second reinstatement: %v", err)
	}
	if _, err := db.VerifyByHand(ctx, run, admin.ID, user.ID, "Checked their WatCard in person"); err != nil {
		t.Fatalf("VerifyByHand: %v", err)
	}
	if _, err := db.VerifyByHand(ctx, run, admin.ID, user.ID, "Checked their WatCard in person"); !errors.Is(err, db.ErrAlreadyVerified) {
		t.Errorf("second verification: %v", err)
	}
	if _, err := db.ChangeRole(ctx, run, &admin.ID, admin.ID, "user", "Stepping down myself"); !errors.Is(err, db.ErrSelfAction) {
		t.Errorf("self role change: %v, want ErrSelfAction", err)
	}

	// Exactly one row per change that happened; none for any refusal.
	rows, err := q.ListAdminActionsForSubject(ctx, sqlcgen.ListAdminActionsForSubjectParams{SubjectType: db.SubjectUser, SubjectID: user.ID})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, r := range rows {
		got = append(got, r.AdminAction.Action)
		if r.ActorUsername == nil || *r.ActorUsername != admin.Username {
			t.Errorf("%s has actor %v", r.AdminAction.Action, r.ActorUsername)
		}
	}
	if len(got) != 3 || got[0] != db.ActionVerify || got[1] != db.ActionReinstate || got[2] != db.ActionSuspend {
		t.Errorf("ledger = %v, want verify, reinstate, suspend", got)
	}
}
