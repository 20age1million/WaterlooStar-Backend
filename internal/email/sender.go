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
