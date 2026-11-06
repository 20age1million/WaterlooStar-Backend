package httpapi_test

import (
	"context"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/20age1million/WaterlooStar-Backend/internal/db/sqlcgen"
)

// The fake's conversations and messages.
//
// It keeps the rules the SQL keeps: one conversation per listing and enquirer,
// a participant filter on every read by id, unread counts from the read
// markers, the inbox ordered by last activity, and suspension and takedown
// reported rather than hidden. Anything looser would let a handler test pass
// against behaviour PostgreSQL would not show.

// tick is the fake's clock for conversations: strictly increasing, so a
// message sent straight after a read marker is always newer than it. The wall
// clock's resolution on some machines would otherwise make the two equal, and
// an unread message would count as read.
func (f *fakeQuerier) tick() time.Time {
	now := time.Now()
	if !now.After(f.lastTick) {
		now = f.lastTick.Add(time.Microsecond)
	}
	f.lastTick = now
	return now
}

func (f *fakeQuerier) CreateConversation(_ context.Context, arg sqlcgen.CreateConversationParams) (sqlcgen.Conversation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	// UNIQUE (listing_id, enquirer_id).
	for _, c := range f.conversations {
		if c.ListingID == arg.ListingID && c.EnquirerID == arg.EnquirerID {
			return sqlcgen.Conversation{}, errUniqueViolation
		}
	}

	now := f.tick()
	c := sqlcgen.Conversation{
		ID:         uuid.New(),
		ListingID:  arg.ListingID,
		OwnerID:    arg.OwnerID,
		EnquirerID: arg.EnquirerID,
		OpenedBy:   arg.OpenedBy,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	f.conversations = append(f.conversations, c)
	return c, nil
}

func (f *fakeQuerier) CreateMessage(_ context.Context, arg sqlcgen.CreateMessageParams) (sqlcgen.Message, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	m := sqlcgen.Message{
		ID:             uuid.New(),
		ConversationID: arg.ConversationID,
		SenderID:       arg.SenderID,
		Body:           arg.Body,
		CreatedAt:      f.tick(),
	}
	f.messages = append(f.messages, m)
	return m, nil
}

func (f *fakeQuerier) GetConversationByListingAndEnquirer(_ context.Context, arg sqlcgen.GetConversationByListingAndEnquirerParams) (sqlcgen.Conversation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	for _, c := range f.conversations {
		if c.ListingID == arg.ListingID && c.EnquirerID == arg.EnquirerID {
			return c, nil
		}
	}
	return sqlcgen.Conversation{}, pgx.ErrNoRows
}

func isParticipant(c sqlcgen.Conversation, user uuid.UUID) bool {
	return c.OwnerID == user || c.EnquirerID == user
}

// conversationView is the joined row both participant reads return.
func (f *fakeQuerier) conversationView(c sqlcgen.Conversation, user uuid.UUID) sqlcgen.GetConversationForParticipantRow {
	row := sqlcgen.GetConversationForParticipantRow{Conversation: c}

	for _, l := range f.listings {
		if l.ID == c.ListingID {
			row.ListingTitle = l.Title
			row.ListingNeighbourhood = l.Neighbourhood
			row.ListingPriceCents = l.PriceCents
			row.ListingStartDate = l.StartDate
			row.ListingEndDate = l.EndDate
			row.ListingAvailable = l.Status == "published" && l.RemovedAt == nil && f.users[c.OwnerID].SuspendedAt == nil
			row.ListingRemoved = l.RemovedAt != nil
			break
		}
	}

	counterpart := c.OwnerID
	if c.OwnerID == user {
		counterpart = c.EnquirerID
	}
	cp := f.users[counterpart]
	row.CounterpartID = cp.ID
	row.CounterpartUsername = cp.Username
	row.CounterpartAvatarUrl = cp.AvatarUrl
	row.CounterpartVerified = cp.Verified
	row.CounterpartSuspended = cp.SuspendedAt != nil

	if last, ok := f.lastMessage(c.ID); ok {
		row.LastMessageSenderID = &last.SenderID
		row.LastMessageBody = &last.Body
		row.LastMessageCreatedAt = &last.CreatedAt
	}
	row.UnreadCount = int64(len(f.unread(c, user)))
	return row
}

func (f *fakeQuerier) lastMessage(conversation uuid.UUID) (sqlcgen.Message, bool) {
	var last sqlcgen.Message
	found := false
	for _, m := range f.messages {
		if m.ConversationID == conversation && (!found || m.CreatedAt.After(last.CreatedAt)) {
			last, found = m, true
		}
	}
	return last, found
}

// unread is the other side's messages newer than user's read marker.
func (f *fakeQuerier) unread(c sqlcgen.Conversation, user uuid.UUID) []sqlcgen.Message {
	marker := c.EnquirerReadAt
	if c.OwnerID == user {
		marker = c.OwnerReadAt
	}
	out := []sqlcgen.Message{}
	for _, m := range f.messages {
		if m.ConversationID != c.ID || m.SenderID == user {
			continue
		}
		if marker == nil || m.CreatedAt.After(*marker) {
			out = append(out, m)
		}
	}
	return out
}

func (f *fakeQuerier) GetConversationForParticipant(_ context.Context, arg sqlcgen.GetConversationForParticipantParams) (sqlcgen.GetConversationForParticipantRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	for _, c := range f.conversations {
		if c.ID == arg.ID && isParticipant(c, arg.UserID) {
			return f.conversationView(c, arg.UserID), nil
		}
	}
	return sqlcgen.GetConversationForParticipantRow{}, pgx.ErrNoRows
}

// inbox is the filtered set, newest activity first.
func (f *fakeQuerier) inbox(user uuid.UUID, status, side *string) []sqlcgen.Conversation {
	out := []sqlcgen.Conversation{}
	for _, c := range f.conversations {
		if !isParticipant(c, user) {
			continue
		}
		if status != nil && (*status == "open") != (c.ClosedAt == nil) {
			continue
		}
		if side != nil && (*side == "owner") != (c.OwnerID == user) {
			continue
		}
		out = append(out, c)
	}
	activity := func(c sqlcgen.Conversation) time.Time {
		if last, ok := f.lastMessage(c.ID); ok {
			return last.CreatedAt
		}
		return c.CreatedAt
	}
	sort.SliceStable(out, func(i, j int) bool { return activity(out[i]).After(activity(out[j])) })
	return out
}

func (f *fakeQuerier) ListConversationsForUser(_ context.Context, arg sqlcgen.ListConversationsForUserParams) ([]sqlcgen.ListConversationsForUserRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	rows := []sqlcgen.ListConversationsForUserRow{}
	for _, c := range page(f.inbox(arg.UserID, arg.Status, arg.Side), arg.Limit, arg.Offset) {
		rows = append(rows, sqlcgen.ListConversationsForUserRow(f.conversationView(c, arg.UserID)))
	}
	return rows, nil
}

func (f *fakeQuerier) CountConversationsForUser(_ context.Context, arg sqlcgen.CountConversationsForUserParams) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return int64(len(f.inbox(arg.UserID, arg.Status, arg.Side))), nil
}

func (f *fakeQuerier) ListMessages(_ context.Context, arg sqlcgen.ListMessagesParams) ([]sqlcgen.Message, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	out := []sqlcgen.Message{}
	for _, m := range f.messages {
		if m.ConversationID == arg.ConversationID {
			out = append(out, m)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return page(out, arg.Limit, arg.Offset), nil
}

func (f *fakeQuerier) CountMessages(_ context.Context, conversation uuid.UUID) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	n := 0
	for _, m := range f.messages {
		if m.ConversationID == conversation {
			n++
		}
	}
	return int64(n), nil
}

func (f *fakeQuerier) CloseConversation(_ context.Context, arg sqlcgen.CloseConversationParams) (sqlcgen.Conversation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	for i, c := range f.conversations {
		// Only an open conversation matches, so the first close stands.
		if c.ID == arg.ID && c.ClosedAt == nil && isParticipant(c, arg.UserID) {
			now := f.tick()
			closer := arg.UserID
			c.ClosedAt, c.ClosedBy, c.UpdatedAt = &now, &closer, now
			f.conversations[i] = c
			return c, nil
		}
	}
	return sqlcgen.Conversation{}, pgx.ErrNoRows
}

func (f *fakeQuerier) MarkConversationRead(_ context.Context, arg sqlcgen.MarkConversationReadParams) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	for i, c := range f.conversations {
		if c.ID != arg.ID || !isParticipant(c, arg.UserID) {
			continue
		}
		now := f.tick()
		if c.OwnerID == arg.UserID {
			c.OwnerReadAt = &now
		} else {
			c.EnquirerReadAt = &now
		}
		c.UpdatedAt = now
		f.conversations[i] = c
		return 1, nil
	}
	return 0, nil
}

func (f *fakeQuerier) UnreadCountsForUser(_ context.Context, user uuid.UUID) (sqlcgen.UnreadCountsForUserRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	var row sqlcgen.UnreadCountsForUserRow
	for _, c := range f.conversations {
		if !isParticipant(c, user) {
			continue
		}
		if n := len(f.unread(c, user)); n > 0 {
			row.Conversations++
			row.Messages += int64(n)
		}
	}
	return row, nil
}

// page applies LIMIT and OFFSET.
func page[T any](items []T, limit, offset int32) []T {
	if int(offset) >= len(items) {
		return []T{}
	}
	end := int(offset) + int(limit)
	if end > len(items) {
		end = len(items)
	}
	return items[offset:end]
}
