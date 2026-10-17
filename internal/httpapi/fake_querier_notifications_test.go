package httpapi_test

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/20age1million/WaterlooStar-Backend/internal/db/sqlcgen"
)

// The fake's half of the notification emails (Phase 18): the opt-out switch,
// and the recipient read, keeping the SQL's rules - the other participant,
// no row for a stranger, and unread counted up to and including the message.

func (f *fakeQuerier) SetEmailNotifications(_ context.Context, arg sqlcgen.SetEmailNotificationsParams) (sqlcgen.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	u, ok := f.users[arg.ID]
	if !ok {
		return sqlcgen.User{}, pgx.ErrNoRows
	}
	u.EmailNotifications = arg.EmailNotifications
	u.UpdatedAt = time.Now()
	f.users[arg.ID] = u
	return u, nil
}

func (f *fakeQuerier) GetNotificationRecipient(_ context.Context, arg sqlcgen.GetNotificationRecipientParams) (sqlcgen.GetNotificationRecipientRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	for _, c := range f.conversations {
		if c.ID != arg.ConversationID || !isParticipant(c, arg.SenderID) {
			continue
		}
		recipientID := c.OwnerID
		if c.OwnerID == arg.SenderID {
			recipientID = c.EnquirerID
		}
		r, s := f.users[recipientID], f.users[arg.SenderID]
		row := sqlcgen.GetNotificationRecipientRow{
			RecipientID:                 r.ID,
			RecipientEmail:              r.Email,
			RecipientVerified:           r.Verified,
			RecipientSuspended:          r.SuspendedAt != nil,
			RecipientEmailNotifications: r.EmailNotifications,
			SenderUsername:              s.Username,
		}
		for _, l := range f.listings {
			if l.ID == c.ListingID {
				row.ListingTitle = l.Title
				break
			}
		}
		if arg.MessageID != nil {
			var through *time.Time
			for _, m := range f.messages {
				if m.ID == *arg.MessageID && m.ConversationID == c.ID {
					at := m.CreatedAt
					through = &at
				}
			}
			if through != nil {
				for _, m := range f.unread(c, recipientID) {
					if !m.CreatedAt.After(*through) {
						row.UnreadThroughMessage++
					}
				}
			}
		}
		return row, nil
	}
	return sqlcgen.GetNotificationRecipientRow{}, pgx.ErrNoRows
}
