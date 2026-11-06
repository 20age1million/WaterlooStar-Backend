package db_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/20age1million/WaterlooStar-Backend/internal/db/dbtest"
	"github.com/20age1million/WaterlooStar-Backend/internal/db/sqlcgen"
)

// Conversations and messages (Phase 15).
//
// The rules worth testing hardest live in the SQL: only a participant gets a
// row, unread counts come from the other side's messages and your own read
// marker, and a conversation whose listing was taken down or whose other side
// is suspended is reported, never hidden.

// pair is an owner with a published listing, and a student asking about it.
type pair struct {
	owner, enquirer sqlcgen.User
	listing         sqlcgen.Listing
}

func newPair(t *testing.T, q *sqlcgen.Queries, prefix string) pair {
	t.Helper()
	owner := dbtest.User(t, q, prefix+"owner@uwaterloo.ca", true)
	enquirer := dbtest.User(t, q, prefix+"enquirer@uwaterloo.ca", true)
	listing := dbtest.Listing(t, q, owner.ID, dbtest.ListingOptions{Title: "Room by the park", Neighbour: "Uptown"})
	return pair{owner: owner, enquirer: enquirer, listing: listing}
}

func participantView(t *testing.T, q *sqlcgen.Queries, id, user uuid.UUID) sqlcgen.GetConversationForParticipantRow {
	t.Helper()
	row, err := q.GetConversationForParticipant(context.Background(), sqlcgen.GetConversationForParticipantParams{
		UserID: user, ID: id,
	})
	if err != nil {
		t.Fatalf("GetConversationForParticipant: %v", err)
	}
	return row
}

func TestConversationRoundTrip(t *testing.T) {
	t.Parallel()
	q, _ := dbtest.Queries(t)
	ctx := context.Background()
	p := newPair(t, q, "rt")

	conversation := dbtest.Conversation(t, q, p.listing, p.enquirer.ID)
	if conversation.ClosedAt != nil || conversation.OpenedBy != p.enquirer.ID || conversation.OwnerID != p.owner.ID {
		t.Fatalf("new conversation = %+v", conversation)
	}

	// No message yet: the last-message columns are null, not a failed scan.
	empty := participantView(t, q, conversation.ID, p.owner.ID)
	if empty.LastMessageBody != nil || empty.LastMessageSenderID != nil || empty.UnreadCount != 0 {
		t.Errorf("a conversation with no messages reported %+v", empty)
	}

	dbtest.Message(t, q, conversation.ID, p.enquirer.ID, "Is the room still free in January?")

	// The owner sees the enquirer as the counterpart, the listing summary, the
	// message as the last one, and one unread.
	asOwner := participantView(t, q, conversation.ID, p.owner.ID)
	if asOwner.CounterpartID != p.enquirer.ID || asOwner.CounterpartUsername != p.enquirer.Username {
		t.Errorf("owner's counterpart = %s, want the enquirer", asOwner.CounterpartUsername)
	}
	if asOwner.ListingTitle != "Room by the park" || asOwner.ListingNeighbourhood != "Uptown" ||
		asOwner.ListingPriceCents != p.listing.PriceCents || !asOwner.ListingAvailable || asOwner.ListingRemoved {
		t.Errorf("listing summary = %+v", asOwner)
	}
	if asOwner.LastMessageBody == nil || *asOwner.LastMessageBody != "Is the room still free in January?" ||
		asOwner.LastMessageSenderID == nil || *asOwner.LastMessageSenderID != p.enquirer.ID {
		t.Errorf("last message = %v from %v", asOwner.LastMessageBody, asOwner.LastMessageSenderID)
	}
	if asOwner.UnreadCount != 1 {
		t.Errorf("owner unread = %d, want 1", asOwner.UnreadCount)
	}

	// The enquirer sees the owner, and their own message is never unread.
	asEnquirer := participantView(t, q, conversation.ID, p.enquirer.ID)
	if asEnquirer.CounterpartID != p.owner.ID || asEnquirer.UnreadCount != 0 {
		t.Errorf("enquirer view: counterpart %s, unread %d", asEnquirer.CounterpartUsername, asEnquirer.UnreadCount)
	}

	found, err := q.GetConversationByListingAndEnquirer(ctx, sqlcgen.GetConversationByListingAndEnquirerParams{
		ListingID: p.listing.ID, EnquirerID: p.enquirer.ID,
	})
	if err != nil || found.ID != conversation.ID {
		t.Fatalf("GetConversationByListingAndEnquirer: %v", err)
	}
}

func TestAStrangerGetsNoRow(t *testing.T) {
	t.Parallel()
	q, _ := dbtest.Queries(t)
	ctx := context.Background()
	p := newPair(t, q, "st")
	stranger := dbtest.User(t, q, "stranger@uwaterloo.ca", true)

	conversation := dbtest.Conversation(t, q, p.listing, p.enquirer.ID)
	dbtest.Message(t, q, conversation.ID, p.enquirer.ID, "Hello")

	if _, err := q.GetConversationForParticipant(ctx, sqlcgen.GetConversationForParticipantParams{
		UserID: stranger.ID, ID: conversation.ID,
	}); !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("a stranger read the conversation: %v", err)
	}
	if _, err := q.CloseConversation(ctx, sqlcgen.CloseConversationParams{
		UserID: stranger.ID, ID: conversation.ID,
	}); !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("a stranger closed the conversation: %v", err)
	}
	if n, err := q.MarkConversationRead(ctx, sqlcgen.MarkConversationReadParams{
		UserID: stranger.ID, ID: conversation.ID,
	}); err != nil || n != 0 {
		t.Errorf("a stranger moved a read marker: %d rows, %v", n, err)
	}
	if total, _ := q.CountConversationsForUser(ctx, sqlcgen.CountConversationsForUserParams{UserID: stranger.ID}); total != 0 {
		t.Errorf("a stranger's inbox holds %d conversations", total)
	}
	if counts, _ := q.UnreadCountsForUser(ctx, stranger.ID); counts.Messages != 0 {
		t.Errorf("a stranger has %d unread", counts.Messages)
	}
}

func TestOneConversationPerListingAndEnquirer(t *testing.T) {
	t.Parallel()
	q, _ := dbtest.Queries(t)
	ctx := context.Background()
	p := newPair(t, q, "uq")

	dbtest.Conversation(t, q, p.listing, p.enquirer.ID)
	if _, err := q.CreateConversation(ctx, sqlcgen.CreateConversationParams{
		ListingID: p.listing.ID, OwnerID: p.owner.ID, EnquirerID: p.enquirer.ID, OpenedBy: p.enquirer.ID,
	}); err == nil {
		t.Fatal("a second conversation about one listing with one student was accepted")
	}

	// A different listing by the same owner is a different conversation.
	other := dbtest.Listing(t, q, p.owner.ID, dbtest.ListingOptions{Title: "Another room"})
	dbtest.Conversation(t, q, other, p.enquirer.ID)
}

func TestTheSchemaRefusesImpossibleConversationsAndMessages(t *testing.T) {
	t.Parallel()
	q, pool := dbtest.Queries(t)
	ctx := context.Background()
	p := newPair(t, q, "ck")
	stranger := dbtest.User(t, q, "outsider@uwaterloo.ca", true)

	// Messaging yourself.
	if _, err := q.CreateConversation(ctx, sqlcgen.CreateConversationParams{
		ListingID: p.listing.ID, OwnerID: p.owner.ID, EnquirerID: p.owner.ID, OpenedBy: p.owner.ID,
	}); err == nil {
		t.Error("a conversation with yourself was accepted")
	}
	// Opened by someone outside it.
	if _, err := q.CreateConversation(ctx, sqlcgen.CreateConversationParams{
		ListingID: p.listing.ID, OwnerID: p.owner.ID, EnquirerID: p.enquirer.ID, OpenedBy: stranger.ID,
	}); err == nil {
		t.Error("a conversation opened by a non-participant was accepted")
	}

	conversation := dbtest.Conversation(t, q, p.listing, p.enquirer.ID)

	// A closure is whole or absent, and only a participant closes.
	if _, err := pool.Exec(ctx, `UPDATE conversations SET closed_at = now() WHERE id = $1`, conversation.ID); err == nil {
		t.Error("closed_at without closed_by was accepted")
	}
	if _, err := pool.Exec(ctx, `UPDATE conversations SET closed_at = now(), closed_by = $2 WHERE id = $1`,
		conversation.ID, stranger.ID); err == nil {
		t.Error("a conversation closed by a non-participant was accepted")
	}

	// Empty and over-long bodies, which the handler refuses first.
	for _, body := range []string{"", "   ", strings.Repeat("a", 4001)} {
		if _, err := q.CreateMessage(ctx, sqlcgen.CreateMessageParams{
			ConversationID: conversation.ID, SenderID: p.enquirer.ID, Body: body,
		}); err == nil {
			t.Errorf("a message of %d characters was accepted", len(body))
		}
	}
	// Exactly 4000 characters, multi-byte ones included, is fine: the limit
	// counts characters, as the handler does.
	if _, err := q.CreateMessage(ctx, sqlcgen.CreateMessageParams{
		ConversationID: conversation.ID, SenderID: p.enquirer.ID, Body: strings.Repeat("é", 4000),
	}); err != nil {
		t.Errorf("4000 characters refused: %v", err)
	}
}

func TestAConversationClosesOnce(t *testing.T) {
	t.Parallel()
	q, _ := dbtest.Queries(t)
	ctx := context.Background()
	p := newPair(t, q, "cl")
	conversation := dbtest.Conversation(t, q, p.listing, p.enquirer.ID)

	closed, err := q.CloseConversation(ctx, sqlcgen.CloseConversationParams{UserID: p.owner.ID, ID: conversation.ID})
	if err != nil || closed.ClosedAt == nil || closed.ClosedBy == nil || *closed.ClosedBy != p.owner.ID {
		t.Fatalf("CloseConversation: %+v, %v", closed, err)
	}

	// Closing again matches nothing, and the first closer stands.
	if _, err := q.CloseConversation(ctx, sqlcgen.CloseConversationParams{UserID: p.enquirer.ID, ID: conversation.ID}); !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("second close: %v, want no rows", err)
	}
	still := participantView(t, q, conversation.ID, p.enquirer.ID)
	if *still.Conversation.ClosedBy != p.owner.ID || !still.Conversation.ClosedAt.Equal(*closed.ClosedAt) {
		t.Errorf("the second close overwrote the first: %+v", still.Conversation)
	}

	// Closing does not hide it: both sides still read it.
	participantView(t, q, conversation.ID, p.owner.ID)
}

func TestUnreadCountsFromBothSides(t *testing.T) {
	t.Parallel()
	q, _ := dbtest.Queries(t)
	ctx := context.Background()
	p := newPair(t, q, "ur")
	conversation := dbtest.Conversation(t, q, p.listing, p.enquirer.ID)

	// Two from the enquirer before any reply: there is no accept step.
	dbtest.Message(t, q, conversation.ID, p.enquirer.ID, "Hi, is it free?")
	dbtest.Message(t, q, conversation.ID, p.enquirer.ID, "I could view it Saturday.")

	unread := func(user uuid.UUID) sqlcgen.UnreadCountsForUserRow {
		t.Helper()
		row, err := q.UnreadCountsForUser(ctx, user)
		if err != nil {
			t.Fatalf("UnreadCountsForUser: %v", err)
		}
		return row
	}

	if got := unread(p.owner.ID); got.Conversations != 1 || got.Messages != 2 {
		t.Errorf("owner unread = %+v, want 1 conversation, 2 messages", got)
	}
	if got := unread(p.enquirer.ID); got.Messages != 0 {
		t.Errorf("the enquirer's own messages count as unread: %+v", got)
	}

	// The owner reads, then replies: the owner's count clears, the enquirer's
	// rises.
	if n, err := q.MarkConversationRead(ctx, sqlcgen.MarkConversationReadParams{UserID: p.owner.ID, ID: conversation.ID}); err != nil || n != 1 {
		t.Fatalf("MarkConversationRead: %d, %v", n, err)
	}
	if got := unread(p.owner.ID); got.Messages != 0 || got.Conversations != 0 {
		t.Errorf("owner unread after reading = %+v", got)
	}
	dbtest.Message(t, q, conversation.ID, p.owner.ID, "Yes - Saturday works.")
	if got := unread(p.enquirer.ID); got.Conversations != 1 || got.Messages != 1 {
		t.Errorf("enquirer unread = %+v, want 1/1", got)
	}
	if got := unread(p.owner.ID); got.Messages != 0 {
		t.Errorf("the owner's reply counts as unread for the owner: %+v", got)
	}

	// Marking read moves only the asking side's marker.
	view := participantView(t, q, conversation.ID, p.enquirer.ID)
	if view.Conversation.OwnerReadAt == nil || view.Conversation.EnquirerReadAt != nil {
		t.Errorf("markers: owner %v, enquirer %v", view.Conversation.OwnerReadAt, view.Conversation.EnquirerReadAt)
	}
	if view.UnreadCount != 1 {
		t.Errorf("per-conversation unread for the enquirer = %d, want 1", view.UnreadCount)
	}

	// A message after the owner's marker is unread again.
	dbtest.Message(t, q, conversation.ID, p.enquirer.ID, "Great, see you then.")
	if got := unread(p.owner.ID); got.Messages != 1 {
		t.Errorf("owner unread after a new message = %+v, want 1", got)
	}
}

func TestTheInboxIsOrderedByLastActivityAndFilters(t *testing.T) {
	t.Parallel()
	q, _ := dbtest.Queries(t)
	ctx := context.Background()

	student := dbtest.User(t, q, "student@uwaterloo.ca", true)
	ownerA := dbtest.User(t, q, "ownera@uwaterloo.ca", true)
	ownerB := dbtest.User(t, q, "ownerb@uwaterloo.ca", true)
	asker := dbtest.User(t, q, "asker@uwaterloo.ca", true)

	listingA := dbtest.Listing(t, q, ownerA.ID, dbtest.ListingOptions{Title: "A"})
	listingB := dbtest.Listing(t, q, ownerB.ID, dbtest.ListingOptions{Title: "B"})
	ownListing := dbtest.Listing(t, q, student.ID, dbtest.ListingOptions{Title: "Mine"})

	// The student asks about A, then B, and is asked about their own place.
	aboutA := dbtest.Conversation(t, q, listingA, student.ID)
	dbtest.Message(t, q, aboutA.ID, student.ID, "About A")
	aboutB := dbtest.Conversation(t, q, listingB, student.ID)
	dbtest.Message(t, q, aboutB.ID, student.ID, "About B")
	aboutMine := dbtest.Conversation(t, q, ownListing, asker.ID)
	dbtest.Message(t, q, aboutMine.ID, asker.ID, "About yours")

	// A reply in A makes it the newest activity.
	dbtest.Message(t, q, aboutA.ID, ownerA.ID, "Reply in A")

	list := func(params sqlcgen.ListConversationsForUserParams) []string {
		t.Helper()
		params.UserID = student.ID
		if params.Limit == 0 {
			params.Limit = 20
		}
		rows, err := q.ListConversationsForUser(ctx, params)
		if err != nil {
			t.Fatalf("ListConversationsForUser: %v", err)
		}
		out := make([]string, 0, len(rows))
		for _, r := range rows {
			out = append(out, r.ListingTitle)
		}
		return out
	}
	count := func(params sqlcgen.CountConversationsForUserParams) int64 {
		t.Helper()
		params.UserID = student.ID
		n, err := q.CountConversationsForUser(ctx, params)
		if err != nil {
			t.Fatalf("CountConversationsForUser: %v", err)
		}
		return n
	}

	if got := strings.Join(list(sqlcgen.ListConversationsForUserParams{}), ","); got != "A,Mine,B" {
		t.Errorf("inbox order = %s, want A,Mine,B", got)
	}
	if n := count(sqlcgen.CountConversationsForUserParams{}); n != 3 {
		t.Errorf("count = %d, want 3", n)
	}

	// The last message travels with the row, newest first.
	rows, _ := q.ListConversationsForUser(ctx, sqlcgen.ListConversationsForUserParams{UserID: student.ID, Limit: 1})
	if len(rows) != 1 || rows[0].LastMessageBody == nil || *rows[0].LastMessageBody != "Reply in A" || rows[0].UnreadCount != 1 {
		t.Errorf("first row = %+v", rows)
	}

	// Paging.
	if got := strings.Join(list(sqlcgen.ListConversationsForUserParams{Limit: 1, Offset: 1}), ","); got != "Mine" {
		t.Errorf("page 2 of 1 = %s, want Mine", got)
	}

	// Side filters.
	owner, enquirer := "owner", "enquirer"
	if got := strings.Join(list(sqlcgen.ListConversationsForUserParams{Side: &owner}), ","); got != "Mine" {
		t.Errorf("side=owner = %s, want Mine", got)
	}
	if got := strings.Join(list(sqlcgen.ListConversationsForUserParams{Side: &enquirer}), ","); got != "A,B" {
		t.Errorf("side=enquirer = %s, want A,B", got)
	}
	if n := count(sqlcgen.CountConversationsForUserParams{Side: &enquirer}); n != 2 {
		t.Errorf("count side=enquirer = %d, want 2", n)
	}

	// Status filters.
	if _, err := q.CloseConversation(ctx, sqlcgen.CloseConversationParams{UserID: ownerB.ID, ID: aboutB.ID}); err != nil {
		t.Fatalf("close: %v", err)
	}
	open, closed := "open", "closed"
	if got := strings.Join(list(sqlcgen.ListConversationsForUserParams{Status: &closed}), ","); got != "B" {
		t.Errorf("status=closed = %s, want B", got)
	}
	if n := count(sqlcgen.CountConversationsForUserParams{Status: &open}); n != 2 {
		t.Errorf("count status=open = %d, want 2", n)
	}
	if n := count(sqlcgen.CountConversationsForUserParams{Status: &open, Side: &enquirer}); n != 1 {
		t.Errorf("count open+enquirer = %d, want 1", n)
	}
}

func TestMessagesAreListedNewestFirst(t *testing.T) {
	t.Parallel()
	q, _ := dbtest.Queries(t)
	ctx := context.Background()
	p := newPair(t, q, "ms")
	conversation := dbtest.Conversation(t, q, p.listing, p.enquirer.ID)

	for _, body := range []string{"first", "second", "third"} {
		dbtest.Message(t, q, conversation.ID, p.enquirer.ID, body)
	}

	rows, err := q.ListMessages(ctx, sqlcgen.ListMessagesParams{ConversationID: conversation.ID, Limit: 2})
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(rows) != 2 || rows[0].Body != "third" || rows[1].Body != "second" {
		t.Errorf("page 1 = %+v", rows)
	}
	rows, _ = q.ListMessages(ctx, sqlcgen.ListMessagesParams{ConversationID: conversation.ID, Limit: 2, Offset: 2})
	if len(rows) != 1 || rows[0].Body != "first" {
		t.Errorf("page 2 = %+v", rows)
	}
	if n, err := q.CountMessages(ctx, conversation.ID); err != nil || n != 3 {
		t.Errorf("CountMessages = %d, %v", n, err)
	}
}

func TestSuspensionAndTakedownAreReportedNotHidden(t *testing.T) {
	t.Parallel()
	q, _ := dbtest.Queries(t)
	ctx := context.Background()
	p := newPair(t, q, "md")
	admin := dbtest.User(t, q, "moderator@uwaterloo.ca", true)
	conversation := dbtest.Conversation(t, q, p.listing, p.enquirer.ID)
	dbtest.Message(t, q, conversation.ID, p.enquirer.ID, "Is this real?")

	inbox := func(user uuid.UUID) []sqlcgen.ListConversationsForUserRow {
		t.Helper()
		rows, err := q.ListConversationsForUser(ctx, sqlcgen.ListConversationsForUserParams{UserID: user, Limit: 20})
		if err != nil {
			t.Fatalf("ListConversationsForUser: %v", err)
		}
		return rows
	}

	// The owner is suspended. The enquirer, who may be the one reporting
	// them, still sees the conversation, flagged.
	if _, err := q.SuspendUser(ctx, sqlcgen.SuspendUserParams{ID: p.owner.ID, ActorID: &admin.ID, Reason: strp("Scam listing")}); err != nil {
		t.Fatalf("SuspendUser: %v", err)
	}
	view := participantView(t, q, conversation.ID, p.enquirer.ID)
	if !view.CounterpartSuspended || view.ListingAvailable {
		t.Errorf("with the owner suspended: counterpart_suspended %v, listing_available %v",
			view.CounterpartSuspended, view.ListingAvailable)
	}
	if rows := inbox(p.enquirer.ID); len(rows) != 1 || !rows[0].CounterpartSuspended {
		t.Errorf("the enquirer's inbox hid or did not flag it: %+v", rows)
	}
	if _, err := q.ReinstateUser(ctx, p.owner.ID); err != nil {
		t.Fatalf("ReinstateUser: %v", err)
	}
	if view := participantView(t, q, conversation.ID, p.enquirer.ID); view.CounterpartSuspended || !view.ListingAvailable {
		t.Errorf("reinstating did not restore it: %+v", view)
	}

	// The listing is taken down: reported as removed and unavailable to both.
	if _, err := q.RemoveListing(ctx, sqlcgen.RemoveListingParams{ID: p.listing.ID, ActorID: &admin.ID, Reason: strp("Scam")}); err != nil {
		t.Fatalf("RemoveListing: %v", err)
	}
	for _, user := range []uuid.UUID{p.owner.ID, p.enquirer.ID} {
		view := participantView(t, q, conversation.ID, user)
		if !view.ListingRemoved || view.ListingAvailable {
			t.Errorf("taken down: removed %v, available %v", view.ListingRemoved, view.ListingAvailable)
		}
		if rows := inbox(user); len(rows) != 1 || !rows[0].ListingRemoved {
			t.Errorf("inbox after takedown = %+v", rows)
		}
	}
	if _, err := q.RestoreListing(ctx, p.listing.ID); err != nil {
		t.Fatalf("RestoreListing: %v", err)
	}

	// Paused by its owner: no longer public, but not taken down, so the
	// conversation can carry on.
	if _, err := q.SetListingStatus(ctx, sqlcgen.SetListingStatusParams{ID: p.listing.ID, Status: "paused"}); err != nil {
		t.Fatalf("pause: %v", err)
	}
	if view := participantView(t, q, conversation.ID, p.enquirer.ID); view.ListingAvailable || view.ListingRemoved {
		t.Errorf("paused: available %v, removed %v", view.ListingAvailable, view.ListingRemoved)
	}
}
