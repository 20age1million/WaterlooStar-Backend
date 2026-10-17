package email

import (
	"os"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
	"unicode/utf8"
)

// The messaging notifications (Phase 18) never carry a message's text or an
// exact address. The builders cannot: none of them takes either. These tests
// hold every builder to that shape, and to what each email must say.

// notificationBuilders is every notification template. A new one must be added
// here, or TestEveryNotificationBuilderIsChecked fails.
var notificationBuilders = map[string]func(id, to, from, listingTitle, link string) Message{
	"ConversationOpenedMessage": ConversationOpenedMessage,
	"OfferConversationMessage":  OfferConversationMessage,
	"AddressSharedMessage":      AddressSharedMessage,
	"ReplyWaitingMessage":       ReplyWaitingMessage,
}

// accountMessages are the two account emails, which carry a credential link
// and are not notifications.
var accountMessages = map[string]bool{"VerificationMessage": true, "PasswordResetMessage": true}

const (
	recipient = "enquirer@uwaterloo.ca"
	sender    = "owner"
	title     = "Sunny room near campus"
	link      = "https://waterloostar.com/messages/3f7c1b2a-9d4e-4c8a-8f1b-2e3d4c5b6a70"
)

func TestEveryNotificationBuilderIsChecked(t *testing.T) {
	src, err := os.ReadFile("sender.go")
	if err != nil {
		t.Fatalf("read sender.go: %v", err)
	}
	declared := regexp.MustCompile(`(?m)^func (\w+Message)\(`).FindAllStringSubmatch(string(src), -1)
	var missing []string
	for _, m := range declared {
		name := m[1]
		if accountMessages[name] {
			continue
		}
		if _, ok := notificationBuilders[name]; !ok {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Fatalf("message builders not covered by the notification tests: %v", missing)
	}
	if len(declared) != len(notificationBuilders)+len(accountMessages) {
		t.Fatalf("found %d builders in sender.go, expected %d", len(declared), len(notificationBuilders)+len(accountMessages))
	}
}

// TestNotificationsTakeNoMessageTextOrAddress checks the signature: five
// strings - the id, the recipient, the sender's username, the listing's title
// and the link - and nothing else. A builder that grew a parameter for a
// message body or an address would fail here before it could be called.
func TestNotificationsTakeNoMessageTextOrAddress(t *testing.T) {
	want := reflect.TypeOf(func(id, to, from, listingTitle, link string) Message { return Message{} })
	for name, build := range notificationBuilders {
		if got := reflect.TypeOf(build); got != want {
			t.Errorf("%s has type %v, want %v", name, got, want)
		}
	}
}

func TestNotificationsCarryNoMessageTextOrAddress(t *testing.T) {
	subjects := map[string]string{
		"ConversationOpenedMessage": "owner wants to talk about Sunny room near campus",
		"OfferConversationMessage":  "owner offered you Sunny room near campus",
		"AddressSharedMessage":      "owner shared the address of Sunny room near campus",
		"ReplyWaitingMessage":       "owner replied about Sunny room near campus",
	}
	for name, build := range notificationBuilders {
		t.Run(name, func(t *testing.T) {
			msg := build("kind-0123", recipient, sender, title, link)

			if msg.ID != "kind-0123" || msg.To != recipient {
				t.Errorf("id/to = %q/%q", msg.ID, msg.To)
			}
			if msg.Subject != subjects[name] {
				t.Errorf("subject = %q, want %q", msg.Subject, subjects[name])
			}
			for _, part := range []string{sender, title, link} {
				if !strings.Contains(msg.Body, part) {
					t.Errorf("body lacks %q:\n%s", part, msg.Body)
				}
			}
			// The link is the only URL, and it is the conversation's.
			if urls := regexp.MustCompile(`https?://\S+`).FindAllString(msg.Body, -1); len(urls) != 1 || urls[0] != link {
				t.Errorf("urls in body = %v, want only %s", urls, link)
			}
			// No address of any kind: the recipient's email is in To only,
			// and nothing in the body or subject looks like an email address.
			for _, field := range []string{msg.Subject, msg.Body} {
				if strings.Contains(field, "@") {
					t.Errorf("an email address in %q", field)
				}
			}
			if !strings.Contains(msg.Body, "turn off email notifications") {
				t.Error("body does not say how to opt out")
			}
		})
	}
}

func TestNotificationSubjectsStayOnOneLine(t *testing.T) {
	long := strings.Repeat("A very long title ", 20)
	for name, build := range notificationBuilders {
		msg := build("id", recipient, "own\r\ner", "Room\nBcc: someone\tnear campus "+long, link)
		if strings.ContainsAny(msg.Subject, "\r\n\t") {
			t.Errorf("%s: subject has a line break or tab: %q", name, msg.Subject)
		}
		if !strings.HasPrefix(msg.Subject, "own er ") {
			t.Errorf("%s: subject = %q", name, msg.Subject)
		}
		if n := utf8.RuneCountInString(msg.Subject); n > 140 {
			t.Errorf("%s: subject is %d characters", name, n)
		}
	}
}
