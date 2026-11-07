package db_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/20age1million/WaterlooStar-Backend/internal/db/dbtest"
	"github.com/20age1million/WaterlooStar-Backend/internal/db/sqlcgen"
)

// Notification emails (Phase 18): the opt-out switch, and the one read that
// says who to email about something done in a conversation and whether a reply
// email is due.

func TestEmailNotificationsDefaultOnAndCanBeTurnedOff(t *testing.T) {
	t.Parallel()
	q, _ := dbtest.Queries(t)
	ctx := context.Background()

	user := dbtest.User(t, q, "switch@uwaterloo.ca", true)
	if !user.EmailNotifications {
		t.Fatal("a new account should have notifications on (the column's default)")
	}

	off, err := q.SetEmailNotifications(ctx, sqlcgen.SetEmailNotificationsParams{ID: user.ID, EmailNotifications: false})
	if err != nil {
		t.Fatalf("SetEmailNotifications: %v", err)
	}
	if off.EmailNotifications || off.ID != user.ID || off.Email != user.Email {
		t.Errorf("after turning off: %+v", off)
	}
	if again, _ := q.GetUserByID(ctx, user.ID); again.EmailNotifications {
		t.Error("the switch did not persist")
	}

	on, err := q.SetEmailNotifications(ctx, sqlcgen.SetEmailNotificationsParams{ID: user.ID, EmailNotifications: true})
	if err != nil || !on.EmailNotifications {
		t.Errorf("turning back on: %+v, %v", on.EmailNotifications, err)
	}

	if _, err := q.SetEmailNotifications(ctx, sqlcgen.SetEmailNotificationsParams{ID: uuid.New(), EmailNotifications: false}); !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("an unknown user: err = %v, want no rows", err)
	}
}

func TestNotificationRecipientIsTheOtherParticipant(t *testing.T) {
	t.Parallel()
	q, _ := dbtest.Queries(t)
	ctx := context.Background()
	p := newPair(t, q, "nr")
	conversation := dbtest.Conversation(t, q, p.listing, p.enquirer.ID)
	dbtest.Message(t, q, conversation.ID, p.enquirer.ID, "Is the room still free in January?")

	recipient := func(sender uuid.UUID, message *uuid.UUID) (sqlcgen.GetNotificationRecipientRow, error) {
		return q.GetNotificationRecipient(ctx, sqlcgen.GetNotificationRecipientParams{
			ConversationID: conversation.ID, SenderID: sender, MessageID: message,
		})
	}

	// The enquirer acted: the owner is told, with the enquirer's name and the
	// listing's title. Nothing in the row is a message body or an address.
	toOwner, err := recipient(p.enquirer.ID, nil)
	if err != nil {
		t.Fatalf("GetNotificationRecipient: %v", err)
	}
	if toOwner.RecipientID != p.owner.ID || toOwner.RecipientEmail != p.owner.Email ||
		toOwner.SenderUsername != p.enquirer.Username || toOwner.ListingTitle != "Room by the park" {
		t.Errorf("to the owner: %+v", toOwner)
	}
	if !toOwner.RecipientVerified || toOwner.RecipientSuspended || !toOwner.RecipientEmailNotifications {
		t.Errorf("owner's state: %+v", toOwner)
	}
	// No message named: nothing is counted.
	if toOwner.UnreadThroughMessage != 0 {
		t.Errorf("unread with no message = %d, want 0", toOwner.UnreadThroughMessage)
	}

	// The owner acted: the enquirer is told.
	toEnquirer, err := recipient(p.owner.ID, nil)
	if err != nil || toEnquirer.RecipientID != p.enquirer.ID || toEnquirer.SenderUsername != p.owner.Username {
		t.Errorf("to the enquirer: %+v, %v", toEnquirer, err)
	}

	// A stranger gets no row.
	stranger := dbtest.User(t, q, "nrstranger@uwaterloo.ca", true)
	if _, err := recipient(stranger.ID, nil); !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("a stranger: err = %v, want no rows", err)
	}

	// The recipient's switch and suspension are reported.
	if _, err := q.SetEmailNotifications(ctx, sqlcgen.SetEmailNotificationsParams{ID: p.owner.ID, EmailNotifications: false}); err != nil {
		t.Fatalf("SetEmailNotifications: %v", err)
	}
	admin := dbtest.User(t, q, "nrmoderator@uwaterloo.ca", true)
	if _, err := q.SuspendUser(ctx, sqlcgen.SuspendUserParams{ID: p.owner.ID, ActorID: &admin.ID, Reason: strp("Spam")}); err != nil {
		t.Fatalf("SuspendUser: %v", err)
	}
	if row, _ := recipient(p.enquirer.ID, nil); row.RecipientEmailNotifications || !row.RecipientSuspended {
		t.Errorf("after opting out and suspension: %+v", row)
	}

	// An unverified recipient is reported as such.
	unverified := dbtest.User(t, q, "nrnew@uwaterloo.ca", false)
	other := dbtest.Conversation(t, q, dbtest.Listing(t, q, p.enquirer.ID, dbtest.ListingOptions{Title: "Other room"}), unverified.ID)
	row, err := q.GetNotificationRecipient(ctx, sqlcgen.GetNotificationRecipientParams{ConversationID: other.ID, SenderID: p.enquirer.ID})
	if err != nil || row.RecipientVerified || row.RecipientID != unverified.ID || row.ListingTitle != "Other room" {
		t.Errorf("an unverified recipient: %+v, %v", row, err)
	}
}

// TestReplyEmailIsDueOncePerUnreadSpell is the reply rule: a message is the
// first unread one when the recipient's unread count, up to and including it,
// is one.
func TestReplyEmailIsDueOncePerUnreadSpell(t *testing.T) {
	t.Parallel()
	q, _ := dbtest.Queries(t)
	ctx := context.Background()
	p := newPair(t, q, "rs")
	conversation := dbtest.Conversation(t, q, p.listing, p.enquirer.ID)

	through := func(sender uuid.UUID, message sqlcgen.Message) int64 {
		t.Helper()
		row, err := q.GetNotificationRecipient(ctx, sqlcgen.GetNotificationRecipientParams{
			ConversationID: conversation.ID, SenderID: sender, MessageID: &message.ID,
		})
		if err != nil {
			t.Fatalf("GetNotificationRecipient: %v", err)
		}
		return row.UnreadThroughMessage
	}
	read := func(user uuid.UUID) {
		t.Helper()
		if _, err := q.MarkConversationRead(ctx, sqlcgen.MarkConversationReadParams{UserID: user, ID: conversation.ID}); err != nil {
			t.Fatalf("MarkConversationRead: %v", err)
		}
	}

	first := dbtest.Message(t, q, conversation.ID, p.enquirer.ID, "Hello")
	second := dbtest.Message(t, q, conversation.ID, p.enquirer.ID, "Are you there?")
	if n := through(p.enquirer.ID, first); n != 1 {
		t.Errorf("first unread message: %d, want 1", n)
	}
	// Counted up to the message named, so the first is still the first even
	// once a second has arrived; the second is not a first.
	if n := through(p.enquirer.ID, second); n != 2 {
		t.Errorf("second unread message: %d, want 2", n)
	}

	// The owner's own message is never unread for the owner; for the
	// enquirer it is the first unread reply.
	reply := dbtest.Message(t, q, conversation.ID, p.owner.ID, "Yes, still free")
	if n := through(p.owner.ID, reply); n != 1 {
		t.Errorf("the owner's first reply, to the enquirer: %d, want 1", n)
	}

	// Once the owner reads, the enquirer's next message starts a new spell.
	read(p.owner.ID)
	third := dbtest.Message(t, q, conversation.ID, p.enquirer.ID, "Great, can I see it Saturday?")
	if n := through(p.enquirer.ID, third); n != 1 {
		t.Errorf("first message after a read: %d, want 1", n)
	}

	// A message from another conversation counts nothing here.
	elsewhere := dbtest.Conversation(t, q, dbtest.Listing(t, q, p.owner.ID, dbtest.ListingOptions{Title: "Second room"}), p.enquirer.ID)
	stray := dbtest.Message(t, q, elsewhere.ID, p.enquirer.ID, "And this one?")
	if n := through(p.enquirer.ID, stray); n != 0 {
		t.Errorf("a message from another conversation: %d, want 0", n)
	}
}
