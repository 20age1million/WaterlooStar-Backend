// Package email delivers transactional mail.
//
// No provider is chosen yet — that decision is deliberately deferred. Everything
// goes through the Sender interface, and the only implementation today writes to
// the log. When a provider is picked (Resend, Postmark, SES), it implements
// Sender and is swapped in at wiring time in cmd/api; nothing else changes.
package email

import (
	"context"
	"fmt"
	"log/slog"
)

// Message is one outbound email.
type Message struct {
	To      string
	Subject string
	Body    string
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
func VerificationMessage(to, link string) Message {
	return Message{
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
func PasswordResetMessage(to, link string) Message {
	return Message{
		To:      to,
		Subject: "Reset your WaterlooStar password",
		Body: fmt.Sprintf(
			"Someone asked to reset the password for this address.\n\n%s\n\n"+
				"The link is good for one hour. If it was not you, nothing has changed "+
				"and you can ignore this email.",
			link),
	}
}
