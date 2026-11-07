// Package email delivers transactional mail.
//
// Everything goes through the Sender interface. ResendSender delivers through
// Resend's email API and is used when RESEND_API_KEY is set (Phase 13). Without a key, LogSender writes each message to the log instead —
// in development that is how you complete a signup, and in production it means
// nothing is delivered. cmd/api chooses between them at startup.
package email

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Message is one outbound email.
type Message struct {
	// ID names this message so a repeated send of it is delivered once. It is
	// derived from the link's token (see MessageID), so a new link is a new
	// message.
	ID      string
	To      string
	Subject string
	Body    string
}

// MessageID derives a message's ID from the hash of the token its link
// carries. It is hashed again so that the value stored in the database never
// leaves the service, and it uses only characters that are safe in an
// idempotency key.
func MessageID(kind string, tokenHash []byte) string {
	sum := sha256.Sum256(tokenHash)
	return kind + "-" + hex.EncodeToString(sum[:16])
}

// Sender delivers messages.
type Sender interface {
	Send(ctx context.Context, msg Message) error
}

// LogSender writes messages to the log instead of delivering them.
//
// In development this is not merely a stub: the verification link it prints is
// how you complete a signup without a mail provider.
type LogSender struct {
	log *slog.Logger
}

// NewLogSender builds the placeholder sender.
func NewLogSender(log *slog.Logger) *LogSender {
	return &LogSender{log: log}
}

// Send logs the message. It never fails, so a missing provider cannot break
// registration in development.
func (s *LogSender) Send(_ context.Context, msg Message) error {
	s.log.Info("email (not sent — no provider configured)",
		slog.String("to", msg.To),
		slog.String("subject", msg.Subject),
		slog.String("body", msg.Body),
	)
	return nil
}

// VerificationMessage builds the address-confirmation email.
func VerificationMessage(id, to, link string) Message {
	return Message{
		ID:      id,
		To:      to,
		Subject: "Confirm your WaterlooStar account",
		Body: fmt.Sprintf(
			"Welcome to WaterlooStar.\n\n"+
				"Confirm this address to message posters, save listings and publish your own:\n\n%s\n\n"+
				"The link is good for 24 hours. If you did not sign up, ignore this email.",
			link),
	}
}

// PasswordResetMessage builds the password-reset email.
func PasswordResetMessage(id, to, link string) Message {
	return Message{
		ID:      id,
		To:      to,
		Subject: "Reset your WaterlooStar password",
		Body: fmt.Sprintf(
			"Someone asked to reset the password for this address.\n\n%s\n\n"+
				"The link is good for one hour. If it was not you, nothing has changed "+
				"and you can ignore this email.",
			link),
	}
}

// ------------------------------------------------------------ notifications
//
// The messaging notifications (Phase 18). Each says who, about which listing,
// and links to the conversation. None of them takes a message's text or the
// listing's address, so none can carry either: an inbox is not a private
// place, and a forwarded email would get round the owner's choice of whom to
// share the address with. TestNotificationsCarryNoMessageTextOrAddress, here
// and in internal/httpapi, holds them to it.
//
// Their IDs come from MessageID over the conversation's id
// ("conversation-opened", "address-shared") or the message's
// ("reply-waiting"), so a retried request sends once.

// maxTitleInEmail keeps a long listing title from making a subject line
// nobody can read. The body carries the same shortened title.
const maxTitleInEmail = 80

// maxUsernameInEmail is the longest username the API accepts.
const maxUsernameInEmail = 32

// notificationFooter ends every notification.
const notificationFooter = "\n\nYou get these emails because you have a WaterlooStar account. " +
	"To stop them, turn off email notifications in your account settings on WaterlooStar. " +
	"Your messages stay in your inbox there either way."

// ConversationOpenedMessage tells a listing's owner that a student wrote to
// them about it. The message itself is not included.
func ConversationOpenedMessage(id, to, from, listingTitle, link string) Message {
	from, title := oneLine(from, maxUsernameInEmail), oneLine(listingTitle, maxTitleInEmail)
	return Message{
		ID:      id,
		To:      to,
		Subject: fmt.Sprintf("%s wants to talk about %s", from, title),
		Body: fmt.Sprintf(
			"%s sent you a message on WaterlooStar about your listing \"%s\".\n\n"+
				"Read it and reply here:\n\n%s\n\n"+
				"The message is not in this email. Your email address is not shown to %s."+
				notificationFooter,
			from, title, link, from),
	}
}

// OfferConversationMessage tells a student who posted a housing request that
// an owner offered them a listing, which opened a conversation. The offer's
// note is not included.
func OfferConversationMessage(id, to, from, listingTitle, link string) Message {
	from, title := oneLine(from, maxUsernameInEmail), oneLine(listingTitle, maxTitleInEmail)
	return Message{
		ID:      id,
		To:      to,
		Subject: fmt.Sprintf("%s offered you %s", from, title),
		Body: fmt.Sprintf(
			"%s answered your housing request on WaterlooStar with a place: \"%s\".\n\n"+
				"See the offer and reply here:\n\n%s\n\n"+
				"Replying is how you answer an offer. Your email address is not shown to %s."+
				notificationFooter,
			from, title, link, from),
	}
}

// AddressSharedMessage tells a student that the owner shared a place's exact
// address with them. The address is not included: it is in the
// conversation, for that student only, and stays there.
func AddressSharedMessage(id, to, from, listingTitle, link string) Message {
	from, title := oneLine(from, maxUsernameInEmail), oneLine(listingTitle, maxTitleInEmail)
	return Message{
		ID:      id,
		To:      to,
		Subject: fmt.Sprintf("%s shared the address of %s", from, title),
		Body: fmt.Sprintf(
			"%s shared the exact address of \"%s\" with you on WaterlooStar.\n\n"+
				"It is in your conversation, not in this email:\n\n%s"+
				notificationFooter,
			from, title, link),
	}
}

// ReplyWaitingMessage tells a student a reply is waiting in a conversation.
// It is sent for the first message they have not read, so it says that no
// more will follow until they read it. The reply is not included.
func ReplyWaitingMessage(id, to, from, listingTitle, link string) Message {
	from, title := oneLine(from, maxUsernameInEmail), oneLine(listingTitle, maxTitleInEmail)
	return Message{
		ID:      id,
		To:      to,
		Subject: fmt.Sprintf("%s replied about %s", from, title),
		Body: fmt.Sprintf(
			"%s replied in your WaterlooStar conversation about \"%s\".\n\n"+
				"Read it here:\n\n%s\n\n"+
				"The reply is not in this email. You will not get another email about this "+
				"conversation until you have read it."+
				notificationFooter,
			from, title, link),
	}
}

// oneLine makes user-written text safe for a subject line: control characters
// and line breaks become spaces, runs of space collapse, and anything past max
// characters is cut with an ellipsis.
func oneLine(s string, max int) string {
	s = strings.Join(strings.FieldsFunc(s, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsControl(r)
	}), " ")
	if utf8.RuneCountInString(s) > max {
		s = strings.TrimSpace(string([]rune(s)[:max-1])) + "…"
	}
	return s
}
