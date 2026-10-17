package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/20age1million/WaterlooStar-Backend/internal/db/sqlcgen"
	"github.com/20age1million/WaterlooStar-Backend/internal/email"
	"github.com/20age1million/WaterlooStar-Backend/internal/ratelimit"
)

// Notification emails (messaging, Phase 18).
//
// Three events email the other participant of a conversation: it being opened
// (from a listing, or by an offer), the owner sharing the address, and the
// first message they have not read. The rules:
//
//   - **Never the message text, never the address.** The builders in
//     internal/email take neither, and GetNotificationRecipient reads
//     neither, so there is nothing here that could put them in an email.
//   - **After the write commits, and never in its way.** notify is called
//     once the transaction is done. It reads the recipient, decides, and hands
//     the send to a goroutine, so a slow Resend holds no lock and delays no
//     response. A failed send is logged and changes nothing.
//   - **Within caps.** NotifyPerRecipient and NotifyGlobal, keyed on the
//     recipient. Going over skips the email; the request still succeeds.
//   - **Only to someone who can act on it.** Nothing is sent to a recipient
//     who opted out, is suspended or has not confirmed their address.

// notifySendTimeout bounds one notification's send. The request that caused
// it has already been answered, so this is not the request's context: gin
// reuses its context once the handler returns.
const notifySendTimeout = 15 * time.Second

// notificationKind is which email, and the kind in its message ID.
type notificationKind string

const (
	notifyConversationOpened notificationKind = "conversation-opened"
	notifyOfferOpened        notificationKind = "offer-opened"
	notifyAddressShared      notificationKind = "address-shared"
	notifyReplyWaiting       notificationKind = "reply-waiting"
)

// notification is one event that may send an email.
type notification struct {
	kind           notificationKind
	conversationID uuid.UUID
	// senderID is who acted; the email goes to the other participant.
	senderID uuid.UUID
	// messageID is the reply, for notifyReplyWaiting only.
	messageID *uuid.UUID
}

// notify decides whether this event sends an email and, if so, sends it in
// the background. It never returns an error: nothing about a notification may
// change the response to the request that caused it.
func (s *Server) notify(ctx context.Context, n notification) {
	attrs := []any{
		slog.String("notification", string(n.kind)),
		slog.String("conversation_id", n.conversationID.String()),
	}

	recipient, err := s.queries.GetNotificationRecipient(ctx, sqlcgen.GetNotificationRecipientParams{
		ConversationID: n.conversationID,
		SenderID:       n.senderID,
		MessageID:      n.messageID,
	})
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			s.log.Error("read notification recipient", append(attrs, slog.String("error", err.Error()))...)
		}
		return
	}
	attrs = append(attrs, slog.String("recipient_id", recipient.RecipientID.String()))

	// One reply email per unread spell: only for the first message the
	// recipient has not read. Later ones wait until they read it.
	if n.kind == notifyReplyWaiting && recipient.UnreadThroughMessage != 1 {
		return
	}

	switch {
	case !recipient.RecipientEmailNotifications:
		s.log.Info("notification not sent: turned off", attrs...)
		return
	case recipient.RecipientSuspended:
		s.log.Info("notification not sent: account suspended", attrs...)
		return
	case !recipient.RecipientVerified:
		s.log.Info("notification not sent: address not confirmed", attrs...)
		return
	}

	// The global cap is checked before the recipient's is spent, so a
	// service-wide refusal does not also cost the recipient a token.
	if d := s.limiter.Peek(ratelimit.NotifyGlobal, ratelimit.GlobalKey(ratelimit.NotifyGlobal)); !d.OK {
		s.log.Warn("notification not sent: daily cap reached", append(attrs, slog.String("rule", ratelimit.NotifyGlobal.Name))...)
		return
	}
	if d := s.limiter.Allow(ratelimit.NotifyPerRecipient, ratelimit.NotifyPerRecipient.Name+":"+recipient.RecipientID.String()); !d.OK {
		s.log.Warn("notification not sent: daily cap reached", append(attrs, slog.String("rule", ratelimit.NotifyPerRecipient.Name))...)
		return
	}
	if d := s.limiter.Allow(ratelimit.NotifyGlobal, ratelimit.GlobalKey(ratelimit.NotifyGlobal)); !d.OK {
		s.log.Warn("notification not sent: daily cap reached", append(attrs, slog.String("rule", ratelimit.NotifyGlobal.Name))...)
		return
	}

	msg := s.notificationMessage(n, recipient)

	s.notifications.Add(1)
	go func() {
		defer s.notifications.Done()
		sendCtx, cancel := context.WithTimeout(context.Background(), notifySendTimeout)
		defer cancel()
		// Logged by ids and Resend's error, as Register logs a failed
		// verification email. ResendSender's errors never carry the
		// recipient's address, the link or the key.
		if err := s.mailer.Send(sendCtx, msg); err != nil {
			s.log.Error("send notification email", append(attrs, slog.String("error", err.Error()))...)
			return
		}
		s.log.Info("notification sent", attrs...)
	}()
}

// notificationMessage builds the email. Its ID derives from the conversation
// or, for a reply, the message, so a retried request is delivered once. An
// offer that opens a conversation shares the "conversation-opened" ID: a
// conversation is opened once, however it was opened.
func (s *Server) notificationMessage(n notification, r sqlcgen.GetNotificationRecipientRow) email.Message {
	link := s.cfg.AppURL + "/messages/" + n.conversationID.String()
	conversation := n.conversationID[:]

	switch n.kind {
	case notifyOfferOpened:
		return email.OfferConversationMessage(email.MessageID(string(notifyConversationOpened), conversation),
			r.RecipientEmail, r.SenderUsername, r.ListingTitle, link)
	case notifyAddressShared:
		return email.AddressSharedMessage(email.MessageID(string(notifyAddressShared), conversation),
			r.RecipientEmail, r.SenderUsername, r.ListingTitle, link)
	case notifyReplyWaiting:
		return email.ReplyWaitingMessage(email.MessageID(string(notifyReplyWaiting), n.messageID[:]),
			r.RecipientEmail, r.SenderUsername, r.ListingTitle, link)
	default:
		return email.ConversationOpenedMessage(email.MessageID(string(notifyConversationOpened), conversation),
			r.RecipientEmail, r.SenderUsername, r.ListingTitle, link)
	}
}

// WaitForNotifications blocks until every notification email already handed
// to the background has been sent or has failed. cmd/api calls it on shutdown
// so a deploy does not cut an email off mid-send; tests call it so they can
// read what was sent.
func (s *Server) WaitForNotifications() {
	s.notifications.Wait()
}
