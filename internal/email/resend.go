package email

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/mail"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const (
	// resendAPIURL is Resend's public API. Only POST /emails is used.
	resendAPIURL = "https://api.resend.com"

	// resendSendTimeout bounds one send. Sending happens inside the request that
	// asked for the email, so a slow Resend must not hold that request for long.
	resendSendTimeout = 10 * time.Second

	// resendErrorBodyLimit caps how much of an error answer is read.
	resendErrorBodyLimit = 64 << 10
)

// ResendSender delivers messages through Resend's email API.
//
// It is one documented call, made with net/http rather than Resend's SDK, so
// that this file alone decides what an error may contain: the body carries a
// link, which is a credential.
type ResendSender struct {
	apiKey  string
	from    string
	baseURL string
	client  *http.Client
}

// NewResendSender builds a sender that authenticates with the given API key and
// sends from the given mailbox, whose domain must be verified with Resend.
func NewResendSender(apiKey string, from mail.Address) *ResendSender {
	return newResendSender(apiKey, from, resendAPIURL, resendSendTimeout)
}

// newResendSender lets tests point the sender at a fake Resend and shorten the
// timeout.
func newResendSender(apiKey string, from mail.Address, baseURL string, timeout time.Duration) *ResendSender {
	return &ResendSender{
		apiKey:  apiKey,
		from:    from.String(),
		baseURL: baseURL,
		client:  &http.Client{Timeout: timeout},
	}
}

// resendEmail is the body of POST /emails.
type resendEmail struct {
	From    string   `json:"from"`
	To      []string `json:"to"`
	Subject string   `json:"subject"`
	Text    string   `json:"text"`
}

// resendError is the body of an error answer.
type resendError struct {
	StatusCode int    `json:"statusCode"`
	Name       string `json:"name"`
	Message    string `json:"message"`
}

// Send hands the message to Resend. Success means Resend accepted it, not that
// it was delivered.
func (s *ResendSender) Send(ctx context.Context, msg Message) error {
	body, err := json.Marshal(resendEmail{
		From:    s.from,
		To:      []string{msg.To},
		Subject: msg.Subject,
		Text:    msg.Body,
	})
	if err != nil {
		return fmt.Errorf("resend email: encode: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL+"/emails", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("resend email: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+s.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "WaterlooStar-Backend")
	if msg.ID != "" {
		req.Header.Set("Idempotency-Key", msg.ID)
	}

	res, err := s.client.Do(req)
	if err != nil {
		return describeTransportError(err, s.client.Timeout)
	}
	defer res.Body.Close()

	if res.StatusCode >= 200 && res.StatusCode < 300 {
		_, _ = io.Copy(io.Discard, res.Body)
		return nil
	}
	return s.describeResendError(res, msg)
}

// urlPattern finds anything that could be a link in Resend's message.
var urlPattern = regexp.MustCompile(`https?://\S+`)

// describeResendError names the status, Resend's error name and its message —
// the message is what says, for instance, that the domain is not verified. It
// can quote the request, so the recipient, any URL and the key are redacted
// from it first. An answer that is not Resend's JSON is never passed through.
func (s *ResendSender) describeResendError(res *http.Response, msg Message) error {
	var body resendError
	raw, _ := io.ReadAll(io.LimitReader(res.Body, resendErrorBodyLimit))
	if err := json.Unmarshal(raw, &body); err != nil || body.Name == "" {
		return fmt.Errorf("resend email: status %d", res.StatusCode)
	}

	message := body.Message
	for _, secret := range []string{s.apiKey, msg.To} {
		if secret != "" {
			message = strings.ReplaceAll(message, secret, "[redacted]")
		}
	}
	message = urlPattern.ReplaceAllString(message, "[link]")
	if message == "" {
		return fmt.Errorf("resend email: status %d: %s", res.StatusCode, body.Name)
	}
	return fmt.Errorf("resend email: status %d: %s: %s", res.StatusCode, body.Name, message)
}

// describeTransportError reports a send that never got an answer. A url.Error
// would name the request URL, which is harmless, but its inner error is enough.
func describeTransportError(err error, timeout time.Duration) error {
	var urlErr *url.Error
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &urlErr) && urlErr.Timeout()) {
		return fmt.Errorf("resend email: no answer within %s", timeout)
	}
	if errors.As(err, &urlErr) {
		return fmt.Errorf("resend email: request failed: %w", urlErr.Err)
	}
	return fmt.Errorf("resend email: request failed: %w", err)
}
