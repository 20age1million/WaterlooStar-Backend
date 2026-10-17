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

// ------------------------------------------- Phase 17: offers open conversations

func TestAnOfferPointsAtItsConversation(t *testing.T) {
	t.Parallel()
	q, _ := dbtest.Queries(t)
	ctx := context.Background()

	student := dbtest.User(t, q, "student6@uwaterloo.ca", true)
	owner := dbtest.User(t, q, "landlord6@uwaterloo.ca", true)
	request := dbtest.Request(t, q, student.ID, dbtest.RequestOptions{})
	listing := dbtest.Listing(t, q, owner.ID, dbtest.ListingOptions{})

	offer, err := q.CreateOffer(ctx, sqlcgen.CreateOfferParams{
		RequestID: request.ID, ListingID: listing.ID, OwnerID: owner.ID, Note: "Near the park.",
	})
	if err != nil {
		t.Fatalf("CreateOffer: %v", err)
	}
	if offer.ConversationID != nil {
		t.Fatal("a new offer row already points at a conversation")
	}

	// Before it has one, the list says so: no id, not closed, not replied.
	rows, err := q.ListOffersForRequest(ctx, request.ID)
	if err != nil || len(rows) != 1 {
		t.Fatalf("ListOffersForRequest: %v (%d rows)", err, len(rows))
	}
	if rows[0].RequestOffer.ConversationID != nil || rows[0].ConversationClosedAt != nil || rows[0].PosterReplied {
		t.Errorf("an offer with no conversation reads %+v", rows[0])
	}

	// The owner opens the conversation, as the offer handler does.
	conversation, err := q.CreateConversation(ctx, sqlcgen.CreateConversationParams{
		ListingID: listing.ID, OwnerID: owner.ID, EnquirerID: student.ID, OpenedBy: owner.ID,
	})
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}
	linked, err := q.SetOfferConversation(ctx, sqlcgen.SetOfferConversationParams{
		ConversationID: conversation.ID, ID: offer.ID,
	})
	if err != nil || linked.ConversationID == nil || *linked.ConversationID != conversation.ID {
		t.Fatalf("SetOfferConversation: %v (%v)", err, linked.ConversationID)
	}
	dbtest.Message(t, q, conversation.ID, owner.ID, "Near the park.")

	// The owner's own message is not the poster replying.
	rows, _ = q.ListOffersForRequest(ctx, request.ID)
	if *rows[0].RequestOffer.ConversationID != conversation.ID || rows[0].ConversationClosedAt != nil || rows[0].PosterReplied {
		t.Errorf("before the reply: %+v", rows[0])
	}

	dbtest.Message(t, q, conversation.ID, student.ID, "Can I see it on Friday?")
	rows, _ = q.ListOffersForRequest(ctx, request.ID)
	if !rows[0].PosterReplied {
		t.Error("poster_replied is false after the poster wrote")
	}

	// Closing shows on the offer.
	if _, err := q.CloseConversation(ctx, sqlcgen.CloseConversationParams{UserID: student.ID, ID: conversation.ID}); err != nil {
		t.Fatalf("CloseConversation: %v", err)
	}
	rows, _ = q.ListOffersForRequest(ctx, request.ID)
	if rows[0].ConversationClosedAt == nil {
		t.Error("a closed conversation reads as open on the offer")
	}

	// Withdrawing the offer leaves the conversation alone.
	if _, err := q.WithdrawOffer(ctx, offer.ID); err != nil {
		t.Fatalf("WithdrawOffer: %v", err)
	}
	if _, err := q.GetConversationForParticipant(ctx, sqlcgen.GetConversationForParticipantParams{
		UserID: student.ID, ID: conversation.ID,
	}); err != nil {
		t.Errorf("the conversation went with the offer: %v", err)
	}
}

func TestReopeningIsTheOwnersOnlyForTheirOwnClose(t *testing.T) {
	t.Parallel()
	q, _ := dbtest.Queries(t)
	ctx := context.Background()

	student := dbtest.User(t, q, "student7@uwaterloo.ca", true)
	owner := dbtest.User(t, q, "landlord7@uwaterloo.ca", true)
	listing := dbtest.Listing(t, q, owner.ID, dbtest.ListingOptions{})
	other := dbtest.Listing(t, q, owner.ID, dbtest.ListingOptions{Title: "The other room"})

	// Closed by the owner: the owner can reopen it.
	byOwner := dbtest.Conversation(t, q, listing, student.ID)
	if _, err := q.CloseConversation(ctx, sqlcgen.CloseConversationParams{UserID: owner.ID, ID: byOwner.ID}); err != nil {
		t.Fatalf("close: %v", err)
	}
	// The enquirer cannot reopen what the owner closed.
	if _, err := q.ReopenConversation(ctx, sqlcgen.ReopenConversationParams{ID: byOwner.ID, OwnerID: student.ID}); !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("the enquirer reopened the owner's close: %v", err)
	}
	reopened, err := q.ReopenConversation(ctx, sqlcgen.ReopenConversationParams{ID: byOwner.ID, OwnerID: owner.ID})
	if err != nil {
		t.Fatalf("ReopenConversation: %v", err)
	}
	if reopened.ClosedAt != nil || reopened.ClosedBy != nil || reopened.AddressSharedAt != nil {
		t.Errorf("reopened = %+v, want open and nothing shared", reopened)
	}

	// Closed by the enquirer: nobody reopens it, the owner included.
	byStudent := dbtest.Conversation(t, q, other, student.ID)
	if _, err := q.CloseConversation(ctx, sqlcgen.CloseConversationParams{UserID: student.ID, ID: byStudent.ID}); err != nil {
		t.Fatalf("close: %v", err)
	}
	if _, err := q.ReopenConversation(ctx, sqlcgen.ReopenConversationParams{ID: byStudent.ID, OwnerID: owner.ID}); !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("the owner reopened a conversation the student closed: %v", err)
	}
	// An open conversation matches nothing either.
	if _, err := q.ReopenConversation(ctx, sqlcgen.ReopenConversationParams{ID: byOwner.ID, OwnerID: owner.ID}); !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("reopening an open conversation matched: %v", err)
	}
}

// The backfill in migration 13, run against offers made before it.
func TestTheOfferBackfillOpensConversationsForLiveOffers(t *testing.T) {
	t.Parallel()
	staged := dbtest.NewAt(t, 12)
	q := sqlcgen.New(staged.Pool)
	ctx := context.Background()

	owner := dbtest.User(t, q, "owner@uwaterloo.ca", true)
	banned := dbtest.User(t, q, "banned@uwaterloo.ca", true)
	poster := dbtest.User(t, q, "poster@uwaterloo.ca", true)
	other := dbtest.User(t, q, "other@uwaterloo.ca", true)
	gone := dbtest.User(t, q, "gone@uwaterloo.ca", true)

	room := dbtest.Listing(t, q, owner.ID, dbtest.ListingOptions{Title: "Room"})
	paused := dbtest.Listing(t, q, owner.ID, dbtest.ListingOptions{Title: "Paused room"})
	takenDown := dbtest.Listing(t, q, owner.ID, dbtest.ListingOptions{Title: "Taken down"})
	bannedRoom := dbtest.Listing(t, q, banned.ID, dbtest.ListingOptions{Title: "Banned owner's room"})
	refused := dbtest.Listing(t, q, owner.ID, dbtest.ListingOptions{Title: "Refused"})

	winter := dbtest.Request(t, q, poster.ID, dbtest.RequestOptions{Title: "Winter"})
	spring := dbtest.Request(t, q, poster.ID, dbtest.RequestOptions{Title: "Spring"})
	others := dbtest.Request(t, q, other.ID, dbtest.RequestOptions{Title: "Other's"})
	removedRequest := dbtest.Request(t, q, other.ID, dbtest.RequestOptions{Title: "Removed request"})
	gonesRequest := dbtest.Request(t, q, gone.ID, dbtest.RequestOptions{Title: "Suspended poster's"})
	ownersOwn := dbtest.Request(t, q, owner.ID, dbtest.RequestOptions{Title: "The owner's own"})

	if _, err := q.SetListingStatus(ctx, sqlcgen.SetListingStatusParams{ID: paused.ID, Status: "paused"}); err != nil {
		t.Fatalf("pause: %v", err)
	}
	if _, err := q.RemoveListing(ctx, sqlcgen.RemoveListingParams{ID: takenDown.ID, Reason: strp("Scam")}); err != nil {
		t.Fatalf("remove listing: %v", err)
	}
	if _, err := q.RemoveRequest(ctx, sqlcgen.RemoveRequestParams{ID: removedRequest.ID, Reason: strp("Spam")}); err != nil {
		t.Fatalf("remove request: %v", err)
	}
	for _, u := range []sqlcgen.User{banned, gone} {
		if _, err := q.SuspendUser(ctx, sqlcgen.SuspendUserParams{ID: u.ID, Reason: strp("Scam")}); err != nil {
			t.Fatalf("suspend: %v", err)
		}
	}

	// The other student already asked about "Refused" and closed the thread.
	closed := dbtest.Conversation(t, q, refused, other.ID)
	dbtest.Message(t, q, closed.ID, other.ID, "Is it near campus?")
	if _, err := q.CloseConversation(ctx, sqlcgen.CloseConversationParams{UserID: other.ID, ID: closed.ID}); err != nil {
		t.Fatalf("close: %v", err)
	}

	// The offers, as Phase 7 wrote them: request_offers has no conversation_id
	// at version 12, so raw SQL rather than the generated CreateOffer.
	base := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	offer := func(n int, request sqlcgen.HousingRequest, listing sqlcgen.Listing, note string, withdrawn bool) uuid.UUID {
		t.Helper()
		var id uuid.UUID
		var withdrawnAt *time.Time
		if withdrawn {
			w := base.Add(time.Duration(n)*time.Hour + time.Minute)
			withdrawnAt = &w
		}
		if err := staged.Pool.QueryRow(ctx, `
			INSERT INTO request_offers (request_id, listing_id, owner_id, note, withdrawn_at, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $6)
			RETURNING id`,
			request.ID, listing.ID, listing.OwnerID, note, withdrawnAt, base.Add(time.Duration(n)*time.Hour),
		).Scan(&id); err != nil {
			t.Fatalf("seed offer %d: %v", n, err)
		}
		return id
	}
	live := offer(1, winter, room, "  Two doors from the library.  ", false)
	joined := offer(2, spring, room, "   ", false)           // same listing, same poster: one conversation
	onPaused := offer(3, winter, paused, "Paused for now", false) // the owner's own state: included
	withdrawn := offer(4, others, room, "Withdrawn", true)
	onTakenDown := offer(5, others, takenDown, "Taken down", false)
	fromBanned := offer(6, others, bannedRoom, "Banned owner", false)
	onRemovedRequest := offer(7, removedRequest, room, "Removed request", false)
	toGone := offer(8, gonesRequest, room, "Suspended poster", false)
	selfOffer := offer(9, ownersOwn, room, "Self", false)
	intoClosed := offer(10, others, refused, "Please reconsider", false)

	staged.MigrateTo(t, 13)
	q = sqlcgen.New(staged.Pool)

	conversationOf := func(id uuid.UUID) *uuid.UUID {
		t.Helper()
		o, err := q.GetOffer(ctx, id)
		if err != nil {
			t.Fatalf("GetOffer: %v", err)
		}
		return o.ConversationID
	}

	// Left without one: withdrawn, hidden by moderation, or the owner's own.
	for name, id := range map[string]uuid.UUID{
		"withdrawn": withdrawn, "taken-down listing": onTakenDown, "suspended owner": fromBanned,
		"taken-down request": onRemovedRequest, "suspended poster": toGone, "own request": selfOffer,
	} {
		if c := conversationOf(id); c != nil {
			t.Errorf("the %s offer was given conversation %s", name, *c)
		}
	}

	// The two offers of one listing to one student share a conversation,
	// opened by the owner at the first offer's time, sharing nothing.
	a, b := conversationOf(live), conversationOf(joined)
	if a == nil || b == nil || *a != *b {
		t.Fatalf("live offers' conversations = %v, %v, want one shared", a, b)
	}
	conversation, err := q.GetConversationForParticipant(ctx, sqlcgen.GetConversationForParticipantParams{
		UserID: poster.ID, ID: *a,
	})
	if err != nil {
		t.Fatalf("the backfilled conversation is not the poster's: %v", err)
	}
	c := conversation.Conversation
	if c.OwnerID != owner.ID || c.EnquirerID != poster.ID || c.OpenedBy != owner.ID || c.ListingID != room.ID {
		t.Errorf("backfilled conversation = %+v", c)
	}
	if !c.CreatedAt.Equal(base.Add(time.Hour)) || c.ClosedAt != nil {
		t.Errorf("created_at = %v, closed_at = %v", c.CreatedAt, c.ClosedAt)
	}
	if c.AddressSharedAt != nil || conversation.PlaceAddressLine != nil {
		t.Error("the backfill shared an address")
	}

	// The non-blank note became a message, trimmed, at the offer's time; the
	// blank one did not. It is unread by the poster.
	messages, err := q.ListMessages(ctx, sqlcgen.ListMessagesParams{ConversationID: *a, Limit: 10})
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(messages) != 1 || messages[0].Body != "Two doors from the library." ||
		messages[0].SenderID != owner.ID || !messages[0].CreatedAt.Equal(base.Add(time.Hour)) {
		t.Errorf("messages = %+v", messages)
	}
	if conversation.UnreadCount != 1 {
		t.Errorf("unread for the poster = %d, want 1", conversation.UnreadCount)
	}

	// The paused listing's offer has its own conversation.
	if p := conversationOf(onPaused); p == nil || *p == *a {
		t.Errorf("paused listing's offer conversation = %v", p)
	}

	// The closed conversation is joined but stays closed, and gets no message.
	if c := conversationOf(intoClosed); c == nil || *c != closed.ID {
		t.Errorf("the offer on a closed pair points at %v, want %s", c, closed.ID)
	}
	still, err := q.GetConversationForParticipant(ctx, sqlcgen.GetConversationForParticipantParams{UserID: other.ID, ID: closed.ID})
	if err != nil || still.Conversation.ClosedAt == nil {
		t.Errorf("the closed conversation was reopened: %v", err)
	}
	if n, _ := q.CountMessages(ctx, closed.ID); n != 1 {
		t.Errorf("the closed conversation has %d messages, want the student's 1", n)
	}

	// No conversation anywhere has a shared address.
	var shared int
	if err := staged.Pool.QueryRow(ctx, `SELECT count(*) FROM conversations WHERE address_shared_at IS NOT NULL`).Scan(&shared); err != nil || shared != 0 {
		t.Errorf("%d conversations have a shared address (%v)", shared, err)
	}
	// Three conversations: the room, the paused room, and the one already there.
	var total int
	if err := staged.Pool.QueryRow(ctx, `SELECT count(*) FROM conversations`).Scan(&total); err != nil || total != 3 {
		t.Errorf("%d conversations, want 3 (%v)", total, err)
	}

	// Down one step drops the link and keeps the conversations; up again is
	// clean, and re-links what is still live.
	staged.MigrateTo(t, 12)
	if err := staged.Pool.QueryRow(ctx, `SELECT count(*) FROM conversations`).Scan(&total); err != nil || total != 3 {
		t.Errorf("after down-one: %d conversations, want 3 kept (%v)", total, err)
	}
	staged.MigrateTo(t, 13)
	q = sqlcgen.New(staged.Pool)
	if again := conversationOf(live); again == nil || *again != *a {
		t.Errorf("after up again the live offer points at %v, want %s", again, *a)
	}
	// The down migration keeps the backfill's messages, so running the
	// backfill again must not post the notes twice.
	if n, _ := q.CountMessages(ctx, *a); n != 1 {
		t.Errorf("after a down-one and up, the note is in the conversation %d times, want 1", n)
	}
}
