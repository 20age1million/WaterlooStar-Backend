package db_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/20age1million/WaterlooStar-Backend/internal/db/dbtest"
	"github.com/20age1million/WaterlooStar-Backend/internal/db/sqlcgen"
)

// Location privacy and address sharing (Phase 16).
//
// Three rules live in the SQL and are tested here against PostgreSQL:
//
//   - The search index does not contain the exact address, so q cannot
//     confirm one by probing. It does contain the public cross streets.
//   - ShareConversationAddress is the only write to address_shared_at: the
//     owner only, once, while the conversation is open and the listing is
//     available and addressed.
//   - The participant reads fill place_* only once the owner has shared, and
//     withhold it - without un-sharing - while the listing is taken down or
//     its owner suspended.

const exactAddress = "742 Evergreen Terrace"

func float(v float64) *float64 { return &v }

func TestSearchDoesNotMatchTheExactAddress(t *testing.T) {
	t.Parallel()
	q, _ := dbtest.Queries(t)
	ctx := context.Background()
	owner := dbtest.User(t, q, "searcher@uwaterloo.ca", true)
	listing := dbtest.Listing(t, q, owner.ID, dbtest.ListingOptions{
		Title:        "Bright room near campus",
		AddressLine:  exactAddress,
		CrossStreets: "King St N & University Ave",
	})

	count := func(term string) int64 {
		t.Helper()
		n, err := q.CountListings(ctx, sqlcgen.CountListingsParams{Search: ptrString(term)})
		if err != nil {
			t.Fatalf("CountListings(%q): %v", term, err)
		}
		rows, err := q.ListListings(ctx, sqlcgen.ListListingsParams{Search: ptrString(term), Sort: "newest", Limit: 20})
		if err != nil {
			t.Fatalf("ListListings(%q): %v", term, err)
		}
		if int64(len(rows)) != n {
			t.Fatalf("search %q: %d rows but a count of %d", term, len(rows), n)
		}
		return n
	}

	// Words only the exact address contains: no match, on the public browse
	// or on the admin's, which reads the same column.
	for _, term := range []string{"Evergreen", "Terrace", `"742 Evergreen Terrace"`} {
		if n := count(term); n != 0 {
			t.Errorf("search %q matched %d listings by their exact address", term, n)
		}
		admin, err := q.ListListingsForAdmin(ctx, sqlcgen.ListListingsForAdminParams{Search: ptrString(term), Limit: 20})
		if err != nil {
			t.Fatalf("ListListingsForAdmin: %v", err)
		}
		if len(admin) != 0 {
			t.Errorf("admin search %q matched the exact address", term)
		}
	}

	// The public cross streets are searchable.
	if n := count("University"); n != 1 {
		t.Errorf("search on the cross streets matched %d, want 1", n)
	}

	// Editing them re-indexes them; clearing them (an empty string, which
	// COALESCE keeps) drops them from the index.
	updated, err := q.UpdateListing(ctx, sqlcgen.UpdateListingParams{ID: listing.ID, CrossStreets: ptrString("Columbia St & Phillip St")})
	if err != nil {
		t.Fatalf("UpdateListing: %v", err)
	}
	if updated.CrossStreets != "Columbia St & Phillip St" || updated.AddressLine != exactAddress {
		t.Errorf("after editing the cross streets: %q at %q", updated.CrossStreets, updated.AddressLine)
	}
	if count("Phillip") != 1 || count("University") != 0 {
		t.Error("the search index did not follow the edited cross streets")
	}
	cleared, err := q.UpdateListing(ctx, sqlcgen.UpdateListingParams{ID: listing.ID, CrossStreets: ptrString("")})
	if err != nil {
		t.Fatalf("UpdateListing clear: %v", err)
	}
	if cleared.CrossStreets != "" || count("Phillip") != 0 {
		t.Errorf("clearing the cross streets left %q", cleared.CrossStreets)
	}
	// Leaving it out leaves it alone.
	if kept, err := q.UpdateListing(ctx, sqlcgen.UpdateListingParams{ID: listing.ID, Title: ptrString("Bright room by the park")}); err != nil || kept.AddressLine != exactAddress {
		t.Errorf("an unrelated edit changed the address: %v, %q", err, kept.AddressLine)
	}

	// The month counts use the same index.
	months, err := q.ListingAvailabilityByMonth(ctx, sqlcgen.ListingAvailabilityByMonthParams{Year: 2027, Search: ptrString("Evergreen")})
	if err != nil {
		t.Fatalf("ListingAvailabilityByMonth: %v", err)
	}
	for _, m := range months {
		if m.Open != 0 {
			t.Errorf("month %d counted a listing found by its exact address", m.Month)
		}
	}
}

// shareFixture is a conversation about a listing with an exact address and
// coordinates, so a revealed place has something to carry.
func shareFixture(t *testing.T, q *sqlcgen.Queries, prefix string) (pair, sqlcgen.Conversation) {
	t.Helper()
	owner := dbtest.User(t, q, prefix+"owner@uwaterloo.ca", true)
	enquirer := dbtest.User(t, q, prefix+"enquirer@uwaterloo.ca", true)
	listing := dbtest.Listing(t, q, owner.ID, dbtest.ListingOptions{
		AddressLine: exactAddress, CrossStreets: "King St N & University Ave",
		Lat: float(43.4723), Lng: float(-80.5449),
	})
	p := pair{owner: owner, enquirer: enquirer, listing: listing}
	conversation := dbtest.Conversation(t, q, listing, enquirer.ID)
	dbtest.Message(t, q, conversation.ID, enquirer.ID, "Is it still free?")
	return p, conversation
}

func share(t *testing.T, q *sqlcgen.Queries, conversation, user uuid.UUID) int64 {
	t.Helper()
	n, err := q.ShareConversationAddress(context.Background(), sqlcgen.ShareConversationAddressParams{ID: conversation, OwnerID: user})
	if err != nil {
		t.Fatalf("ShareConversationAddress: %v", err)
	}
	return n
}

func TestShareConversationAddressSetsOnceAndOnlyForTheOwner(t *testing.T) {
	t.Parallel()
	q, _ := dbtest.Queries(t)
	p, conversation := shareFixture(t, q, "so")
	stranger := dbtest.User(t, q, "so-stranger@uwaterloo.ca", true)

	if n := share(t, q, conversation.ID, p.enquirer.ID); n != 0 {
		t.Error("the enquirer shared the owner's address")
	}
	if n := share(t, q, conversation.ID, stranger.ID); n != 0 {
		t.Error("a stranger shared the address")
	}
	if view := participantView(t, q, conversation.ID, p.owner.ID); view.Conversation.AddressSharedAt != nil {
		t.Fatal("a refused share set address_shared_at")
	}

	if n := share(t, q, conversation.ID, p.owner.ID); n != 1 {
		t.Fatalf("the owner's share affected %d rows, want 1", n)
	}
	first := participantView(t, q, conversation.ID, p.owner.ID).Conversation.AddressSharedAt
	if first == nil {
		t.Fatal("the owner's share did not set address_shared_at")
	}

	// Sharing again matches nothing, so the first share's time stands.
	if n := share(t, q, conversation.ID, p.owner.ID); n != 0 {
		t.Errorf("a second share affected %d rows", n)
	}
	if again := participantView(t, q, conversation.ID, p.owner.ID).Conversation.AddressSharedAt; again == nil || !again.Equal(*first) {
		t.Errorf("a second share moved address_shared_at from %v to %v", first, again)
	}
}

func TestShareConversationAddressRefusals(t *testing.T) {
	t.Parallel()
	q, _ := dbtest.Queries(t)
	ctx := context.Background()
	admin := dbtest.User(t, q, "sr-moderator@uwaterloo.ca", true)

	cases := map[string]func(p pair, c sqlcgen.Conversation){
		"closed": func(p pair, c sqlcgen.Conversation) {
			if _, err := q.CloseConversation(ctx, sqlcgen.CloseConversationParams{UserID: p.enquirer.ID, ID: c.ID}); err != nil {
				t.Fatalf("CloseConversation: %v", err)
			}
		},
		"paused": func(p pair, _ sqlcgen.Conversation) {
			if _, err := q.SetListingStatus(ctx, sqlcgen.SetListingStatusParams{ID: p.listing.ID, Status: "paused"}); err != nil {
				t.Fatalf("pause: %v", err)
			}
		},
		"taken down": func(p pair, _ sqlcgen.Conversation) {
			if _, err := q.RemoveListing(ctx, sqlcgen.RemoveListingParams{ID: p.listing.ID, ActorID: &admin.ID, Reason: strp("Scam")}); err != nil {
				t.Fatalf("RemoveListing: %v", err)
			}
		},
		"owner suspended": func(p pair, _ sqlcgen.Conversation) {
			if _, err := q.SuspendUser(ctx, sqlcgen.SuspendUserParams{ID: p.owner.ID, ActorID: &admin.ID, Reason: strp("Scam")}); err != nil {
				t.Fatalf("SuspendUser: %v", err)
			}
		},
		"enquirer suspended": func(p pair, _ sqlcgen.Conversation) {
			if _, err := q.SuspendUser(ctx, sqlcgen.SuspendUserParams{ID: p.enquirer.ID, ActorID: &admin.ID, Reason: strp("Spam")}); err != nil {
				t.Fatalf("SuspendUser: %v", err)
			}
		},
		"no address": func(p pair, _ sqlcgen.Conversation) {
			if _, err := q.UpdateListing(ctx, sqlcgen.UpdateListingParams{ID: p.listing.ID, AddressLine: ptrString("  ")}); err != nil {
				t.Fatalf("UpdateListing: %v", err)
			}
		},
	}

	i := 0
	for name, apply := range cases {
		i++
		p, conversation := shareFixture(t, q, string(rune('a'+i))+"sr")
		apply(p, conversation)
		if n := share(t, q, conversation.ID, p.owner.ID); n != 0 {
			t.Errorf("%s: the share went through", name)
		}
		if view := participantView(t, q, conversation.ID, p.enquirer.ID); view.Conversation.AddressSharedAt != nil || view.PlaceAddressLine != nil {
			t.Errorf("%s: address_shared_at %v, place %v", name, view.Conversation.AddressSharedAt, view.PlaceAddressLine)
		}
	}
}

func TestThePlaceIsRevealedOnlyAfterTheOwnerShares(t *testing.T) {
	t.Parallel()
	q, _ := dbtest.Queries(t)
	ctx := context.Background()
	p, conversation := shareFixture(t, q, "pl")
	admin := dbtest.User(t, q, "pl-moderator@uwaterloo.ca", true)

	// The same enquirer about another of the owner's listings, and another
	// enquirer about the same listing: neither is touched by this share.
	other := dbtest.Listing(t, q, p.owner.ID, dbtest.ListingOptions{Title: "A second room nearby", AddressLine: "12 Other St"})
	sameEnquirerElsewhere := dbtest.Conversation(t, q, other, p.enquirer.ID)
	second := dbtest.User(t, q, "pl-second@uwaterloo.ca", true)
	sameListingOtherEnquirer := dbtest.Conversation(t, q, p.listing, second.ID)

	inboxRow := func(user, id uuid.UUID) sqlcgen.ListConversationsForUserRow {
		t.Helper()
		rows, err := q.ListConversationsForUser(ctx, sqlcgen.ListConversationsForUserParams{UserID: user, Limit: 20})
		if err != nil {
			t.Fatalf("ListConversationsForUser: %v", err)
		}
		for _, r := range rows {
			if r.Conversation.ID == id {
				return r
			}
		}
		t.Fatalf("conversation %s not in the inbox", id)
		return sqlcgen.ListConversationsForUserRow{}
	}
	// revealed reports what both participant reads say, for both participants,
	// failing if they disagree.
	revealed := func(id uuid.UUID, users ...uuid.UUID) bool {
		t.Helper()
		var seen []bool
		for _, user := range users {
			one := participantView(t, q, id, user)
			many := inboxRow(user, id)
			if (one.PlaceAddressLine == nil) != (many.PlaceAddressLine == nil) {
				t.Fatalf("the single read and the inbox disagree about the place")
			}
			seen = append(seen, one.PlaceAddressLine != nil)
		}
		for _, s := range seen[1:] {
			if s != seen[0] {
				t.Fatalf("the two participants disagree about the place")
			}
		}
		return seen[0]
	}

	// Messages and reading reveal nothing.
	dbtest.Message(t, q, conversation.ID, p.owner.ID, "Yes, it is.")
	if _, err := q.MarkConversationRead(ctx, sqlcgen.MarkConversationReadParams{UserID: p.enquirer.ID, ID: conversation.ID}); err != nil {
		t.Fatalf("MarkConversationRead: %v", err)
	}
	if revealed(conversation.ID, p.owner.ID, p.enquirer.ID) {
		t.Fatal("the place is revealed before the owner shared it")
	}

	if share(t, q, conversation.ID, p.owner.ID) != 1 {
		t.Fatal("the owner could not share")
	}
	if !revealed(conversation.ID, p.owner.ID, p.enquirer.ID) {
		t.Fatal("the place is not revealed after the owner shared it")
	}
	view := participantView(t, q, conversation.ID, p.enquirer.ID)
	if *view.PlaceAddressLine != exactAddress || view.PlaceLat == nil || *view.PlaceLat != 43.4723 ||
		view.PlaceLng == nil || *view.PlaceLng != -80.5449 {
		t.Errorf("place = %v (%v, %v)", *view.PlaceAddressLine, view.PlaceLat, view.PlaceLng)
	}

	// One conversation only.
	if revealed(sameEnquirerElsewhere.ID, p.owner.ID, p.enquirer.ID) {
		t.Error("the share revealed the address of the owner's other listing")
	}
	if revealed(sameListingOtherEnquirer.ID, p.owner.ID, second.ID) {
		t.Error("the share revealed the address to another enquirer")
	}

	// The address is read live: a correction shows.
	if _, err := q.UpdateListing(ctx, sqlcgen.UpdateListingParams{ID: p.listing.ID, AddressLine: ptrString("744 Evergreen Terrace")}); err != nil {
		t.Fatalf("UpdateListing: %v", err)
	}
	if got := participantView(t, q, conversation.ID, p.enquirer.ID).PlaceAddressLine; *got != "744 Evergreen Terrace" {
		t.Errorf("after a correction the place reads %q", *got)
	}

	// Taken down: withheld, not un-shared. Restored: back.
	if _, err := q.RemoveListing(ctx, sqlcgen.RemoveListingParams{ID: p.listing.ID, ActorID: &admin.ID, Reason: strp("Check")}); err != nil {
		t.Fatalf("RemoveListing: %v", err)
	}
	if revealed(conversation.ID, p.owner.ID, p.enquirer.ID) {
		t.Error("a taken-down listing's place is still revealed")
	}
	if participantView(t, q, conversation.ID, p.enquirer.ID).Conversation.AddressSharedAt == nil {
		t.Error("the takedown un-shared the address")
	}
	if _, err := q.RestoreListing(ctx, p.listing.ID); err != nil {
		t.Fatalf("RestoreListing: %v", err)
	}
	if !revealed(conversation.ID, p.owner.ID, p.enquirer.ID) {
		t.Error("restoring the listing did not restore the place")
	}

	// Owner suspended: withheld. Reinstated: back.
	if _, err := q.SuspendUser(ctx, sqlcgen.SuspendUserParams{ID: p.owner.ID, ActorID: &admin.ID, Reason: strp("Check")}); err != nil {
		t.Fatalf("SuspendUser: %v", err)
	}
	if revealed(conversation.ID, p.owner.ID, p.enquirer.ID) {
		t.Error("a suspended owner's place is still revealed")
	}
	if _, err := q.ReinstateUser(ctx, p.owner.ID); err != nil {
		t.Fatalf("ReinstateUser: %v", err)
	}
	if !revealed(conversation.ID, p.owner.ID, p.enquirer.ID) {
		t.Error("reinstating the owner did not restore the place")
	}

	// Closing stops sharing, but a share already made stays: the enquirer
	// has seen it, and closing un-shares nothing.
	if _, err := q.CloseConversation(ctx, sqlcgen.CloseConversationParams{UserID: p.enquirer.ID, ID: conversation.ID}); err != nil {
		t.Fatalf("CloseConversation: %v", err)
	}
	if !revealed(conversation.ID, p.owner.ID, p.enquirer.ID) {
		t.Error("closing the conversation withheld a shared place")
	}
}
