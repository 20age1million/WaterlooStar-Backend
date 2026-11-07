package httpapi_test

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/20age1million/WaterlooStar-Backend/internal/apierror"
	"github.com/20age1million/WaterlooStar-Backend/internal/config"
	"github.com/20age1million/WaterlooStar-Backend/internal/db/sqlcgen"
	"github.com/20age1million/WaterlooStar-Backend/internal/email"
	"github.com/20age1million/WaterlooStar-Backend/internal/httpapi"
	"github.com/20age1million/WaterlooStar-Backend/internal/ratelimit"
)

// Notification emails, through the handlers (Phase 18).
//
// Every test here records email with capturingMailer; nothing reaches Resend.
// The rules tested hardest: each event emails the other side once, a busy
// thread sends one reply email per unread spell, nothing is sent to someone
// who opted out or cannot act on it, the caps skip the email and never the
// request, and no email carries a message's text or the exact address.

// lockedBuffer is a log destination the background sends can write to while
// a test reads it.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) lines(containing string) []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	var out []string
	for _, line := range strings.Split(b.buf.String(), "\n") {
		if strings.Contains(line, containing) {
			out = append(out, line)
		}
	}
	return out
}

// hookedQuerier is the fake with a hook after a message is written, so a test
// can change the world between the handler's checks and its notification -
// as a moderator acting at that moment would.
type hookedQuerier struct {
	*fakeQuerier
	afterMessage func()
}

func (q *hookedQuerier) CreateMessage(ctx context.Context, arg sqlcgen.CreateMessageParams) (sqlcgen.Message, error) {
	m, err := q.fakeQuerier.CreateMessage(ctx, arg)
	if err == nil && q.afterMessage != nil {
		q.afterMessage()
	}
	return m, err
}

// notifyHarness is newHarness with the limiter's clock in the test's hands, the
// log captured, and the fake reachable through a hook.
func notifyHarness(t *testing.T) (*harness, *testClock, *lockedBuffer, *hookedQuerier) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	cfg := config.Config{
		Port:        8080,
		CORSOrigin:  "http://localhost:3000",
		AppURL:      "http://localhost:3000",
		Environment: config.Development,
		JWTSecret:   "a-test-secret-that-is-long-enough-to-pass",
	}
	logs := &lockedBuffer{}
	log := slog.New(slog.NewTextHandler(io.MultiWriter(logs), nil))

	clk := newTestClock()
	fake := newFakeQuerier()
	hooked := &hookedQuerier{fakeQuerier: fake}
	mail := &capturingMailer{}
	api := httpapi.NewServerWithLimiter(cfg, log, hooked, mail, "", ratelimit.New(0, clk.Now))
	srv := httptest.NewServer(httpapi.NewRouter(cfg, log, api))
	t.Cleanup(srv.Close)

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar: %v", err)
	}
	return &harness{server: srv, db: fake, mail: mail, client: &http.Client{Jar: jar}, api: api}, clk, logs, hooked
}

// notifyParties registers an owner with a listing at an exact address and an
// enquirer, each in their own browser, on h.
func notifyParties(t *testing.T, h *harness) (owner, enquirer *harness, listingID string) {
	t.Helper()
	register(t, h, "owner@uwaterloo.ca", "owner")
	res, listing := createListing(t, h, privateListing())
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create listing: %d", res.StatusCode)
	}
	e := anotherBrowser(t, h)
	register(t, e, "enquirer@uwaterloo.ca", "enquirer")
	return h, e, listing["id"].(string)
}

// notifications is every notification email recorded so far; verification
// and reset emails are left out.
func notifications(h *harness) []email.Message {
	h.mail.mu.Lock()
	defer h.mail.mu.Unlock()
	var out []email.Message
	for _, m := range h.mail.sent {
		for _, kind := range []string{"conversation-opened-", "address-shared-", "reply-waiting-"} {
			if strings.HasPrefix(m.ID, kind) {
				out = append(out, m)
			}
		}
	}
	return out
}

// expectNotifications checks that exactly the given number of new
// notifications arrived since before, and returns them.
func expectNotifications(t *testing.T, h *harness, before int, want int, label string) []email.Message {
	t.Helper()
	all := notifications(h)
	got := all[before:]
	if len(got) != want {
		t.Fatalf("%s: %d new notification emails, want %d: %+v", label, len(got), want, got)
	}
	return got
}

func markRead(t *testing.T, h *harness, conversationID string) {
	t.Helper()
	if res := h.do(t, http.MethodPost, "/conversations/"+conversationID+"/read", nil); res.StatusCode != http.StatusNoContent {
		t.Fatalf("mark read: %d", res.StatusCode)
	}
}

func mustSend(t *testing.T, h *harness, conversationID, body string) {
	t.Helper()
	if res := send(t, h, conversationID, body); res.StatusCode != http.StatusCreated {
		t.Fatalf("send: status %d, want 201", res.StatusCode)
	}
}

const listingTitle = "Bright room in a 4-bed student house"

func TestOpeningAConversationEmailsTheOwner(t *testing.T) {
	h, _, _, _ := notifyHarness(t)
	_, enquirer, listingID := notifyParties(t, h)

	c := started(t, enquirer, listingID)

	got := expectNotifications(t, h, 0, 1, "start")[0]
	if got.To != "owner@uwaterloo.ca" {
		t.Errorf("to %q, want the owner", got.To)
	}
	if got.Subject != "enquirer wants to talk about "+listingTitle {
		t.Errorf("subject = %q", got.Subject)
	}
	if !strings.Contains(got.Body, "http://localhost:3000/messages/"+c.ID) {
		t.Errorf("body has no link to the conversation:\n%s", got.Body)
	}
	if want := email.MessageID("conversation-opened", uuidBytes(t, c.ID)); got.ID != want {
		t.Errorf("id = %q, want %q", got.ID, want)
	}
}

// TestOneReplyEmailPerUnreadSpell is the reply rule: an email for the first
// message the recipient has not read, then nothing until they read.
func TestOneReplyEmailPerUnreadSpell(t *testing.T) {
	h, _, _, _ := notifyHarness(t)
	owner, enquirer, listingID := notifyParties(t, h)
	c := started(t, enquirer, listingID)
	n := len(notifications(h)) // the opened email

	// A follow-up before the owner reads: the opening email covers it.
	mustSend(t, enquirer, c.ID, "Also, is parking included?")
	expectNotifications(t, h, n, 0, "a second unread message")

	// The owner replies twice: one email to the enquirer.
	mustSend(t, owner, c.ID, "Yes it is.")
	reply := expectNotifications(t, h, n, 1, "the owner's first reply")[0]
	if reply.To != "enquirer@uwaterloo.ca" || reply.Subject != "owner replied about "+listingTitle {
		t.Errorf("reply email: to %q, subject %q", reply.To, reply.Subject)
	}
	mustSend(t, owner, c.ID, "And the room is furnished.")
	expectNotifications(t, h, n, 1, "the owner's second reply, unread")

	// The enquirer reads; the owner's next message starts a new spell.
	markRead(t, enquirer, c.ID)
	mustSend(t, owner, c.ID, "When would you like to see it?")
	expectNotifications(t, h, n, 2, "a reply after the enquirer read")

	// The owner reads; the enquirer's next message emails the owner.
	markRead(t, owner, c.ID)
	mustSend(t, enquirer, c.ID, "Saturday at noon?")
	got := expectNotifications(t, h, n, 3, "a reply after the owner read")
	if got[2].To != "owner@uwaterloo.ca" {
		t.Errorf("to %q, want the owner", got[2].To)
	}
}

func TestSharingTheAddressEmailsTheEnquirerOnce(t *testing.T) {
	h, _, _, _ := notifyHarness(t)
	owner, enquirer, listingID := notifyParties(t, h)
	c := started(t, enquirer, listingID)
	n := len(notifications(h))

	if res := shareAddress(t, owner, c.ID); res.StatusCode != http.StatusOK {
		t.Fatalf("share: %d", res.StatusCode)
	}
	got := expectNotifications(t, h, n, 1, "share")[0]
	if got.To != "enquirer@uwaterloo.ca" || got.Subject != "owner shared the address of "+listingTitle {
		t.Errorf("share email: to %q, subject %q", got.To, got.Subject)
	}
	if strings.Contains(got.Body, exactAddress) || strings.Contains(got.Subject, exactAddress) {
		t.Errorf("the share email carries the address:\n%s", got.Body)
	}

	// Sharing again changes nothing, and sends nothing.
	if res := shareAddress(t, owner, c.ID); res.StatusCode != http.StatusOK {
		t.Fatalf("second share: %d", res.StatusCode)
	}
	expectNotifications(t, h, n, 1, "a second share")
}

func TestAnOfferThatOpensAConversationEmailsThePoster(t *testing.T) {
	h, _, _, _ := notifyHarness(t)
	owner, poster, listingID := notifyParties(t, h)
	res, request := postRequest(t, poster, validRequest())
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("post request: %d", res.StatusCode)
	}
	n := len(notifications(h))

	o := offered(t, owner, request["id"].(string), listingID, "It is two minutes from campus.")
	got := expectNotifications(t, h, n, 1, "offer")[0]
	if got.To != "enquirer@uwaterloo.ca" || got.Subject != "owner offered you "+listingTitle {
		t.Errorf("offer email: to %q, subject %q", got.To, got.Subject)
	}
	if !strings.Contains(got.Body, "/messages/"+o.Conversation.ID) {
		t.Errorf("no link to the offer's conversation:\n%s", got.Body)
	}
	// The conversation is opened once, however it was opened.
	if want := email.MessageID("conversation-opened", uuidBytes(t, o.Conversation.ID)); got.ID != want {
		t.Errorf("id = %q, want %q", got.ID, want)
	}
}

// An offer that joins an existing conversation is a message there: its note
// emails the poster only as a first unread reply would, and an offer with no
// note emails nothing.
func TestAnOfferThatJoinsAConversationEmailsOnlyAsAMessage(t *testing.T) {
	h, _, _, _ := notifyHarness(t)
	owner, poster, listingID := notifyParties(t, h)
	c := started(t, poster, listingID) // the owner hears of it
	_, first := postRequest(t, poster, validRequest())
	_, second := postRequest(t, poster, validRequest())
	n := len(notifications(h))

	offered(t, owner, first["id"].(string), listingID, "Here it is as an offer too.")
	got := expectNotifications(t, h, n, 1, "a joining offer with a note")[0]
	if got.To != "enquirer@uwaterloo.ca" || got.Subject != "owner replied about "+listingTitle {
		t.Errorf("joining offer email: to %q, subject %q", got.To, got.Subject)
	}

	markRead(t, poster, c.ID)
	offered(t, owner, second["id"].(string), listingID, "")
	expectNotifications(t, h, n, 1, "a joining offer with no note")
}

func TestOptingOutStopsNotificationsButNotTheInbox(t *testing.T) {
	h, _, _, _ := notifyHarness(t)
	owner, enquirer, listingID := notifyParties(t, h)

	me := decode[map[string]any](t, owner.do(t, http.MethodGet, "/me", nil))
	if me["email_notifications"] != true {
		t.Fatalf("GET /me email_notifications = %v, want true by default", me["email_notifications"])
	}

	res := owner.do(t, http.MethodPatch, "/me/preferences", map[string]bool{"email_notifications": false})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("PATCH /me/preferences: %d", res.StatusCode)
	}
	if body := decode[map[string]any](t, res); body["email_notifications"] != false || body["email"] != "owner@uwaterloo.ca" {
		t.Errorf("PATCH answered %v", body)
	}

	c := started(t, enquirer, listingID)
	expectNotifications(t, h, 0, 0, "start, owner opted out")
	if u := unread(t, owner); u.Messages != 1 {
		t.Errorf("owner's unread = %+v; opting out must not touch the inbox", u)
	}

	// Back on: the next first-unread message emails again.
	if res := owner.do(t, http.MethodPatch, "/me/preferences", map[string]bool{"email_notifications": true}); res.StatusCode != http.StatusOK {
		t.Fatalf("turn back on: %d", res.StatusCode)
	}
	markRead(t, owner, c.ID)
	mustSend(t, enquirer, c.ID, "Any news?")
	expectNotifications(t, h, 0, 1, "after turning back on")
}

func TestNoNotificationToAnUnverifiedOrSuspendedRecipient(t *testing.T) {
	t.Run("unverified", func(t *testing.T) {
		h, _, _, _ := notifyHarness(t)
		owner, enquirer, listingID := notifyParties(t, h)
		// An account cannot lose verification through the API; set it in the
		// fake, as the backstop is for exactly what the API never does.
		setVerified(t, owner, "owner@uwaterloo.ca", false)
		started(t, enquirer, listingID)
		expectNotifications(t, h, 0, 0, "start to an unverified owner")
	})

	t.Run("suspended while the message is sent", func(t *testing.T) {
		h, _, _, hooked := notifyHarness(t)
		owner, enquirer, listingID := notifyParties(t, h)
		c := started(t, enquirer, listingID)
		markRead(t, owner, c.ID)
		n := len(notifications(h))

		// The send's checks pass; the owner is suspended before the email.
		hooked.afterMessage = func() {
			hooked.afterMessage = nil
			suspend(t, h, "owner@uwaterloo.ca")
		}
		mustSend(t, enquirer, c.ID, "Still there?")
		expectNotifications(t, h, n, 0, "a reply to a suspended owner")
	})
}

// TestNotificationCapsSkipTheEmailNotTheRequest: the twenty-first email to one
// recipient in a day is skipped and the request still succeeds; the bucket
// refills on the clock.
func TestNotificationCapsSkipTheEmailNotTheRequest(t *testing.T) {
	h, clk, logs, _ := notifyHarness(t)
	owner, enquirer, listingID := notifyParties(t, h)
	c := started(t, enquirer, listingID) // 1 to the owner

	for i := 2; i <= 21; i++ {
		markRead(t, owner, c.ID)
		mustSend(t, enquirer, c.ID, "Another question.") // 201 every time
	}
	toOwner := expectNotifications(t, h, 0, 20, "twenty-one events for one recipient")
	for _, m := range toOwner {
		if m.To != "owner@uwaterloo.ca" {
			t.Fatalf("to %q", m.To)
		}
	}
	if len(logs.lines("notify_per_recipient")) != 1 {
		t.Errorf("the skipped email was not logged once: %v", logs.lines("notification not sent"))
	}

	// About one an hour and twelve minutes refills.
	clk.advance(73 * time.Minute)
	markRead(t, owner, c.ID)
	mustSend(t, enquirer, c.ID, "One more.")
	expectNotifications(t, h, 0, 21, "after the bucket refilled")
}

func TestTheGlobalNotificationCapProtectsAccountMail(t *testing.T) {
	h, clk, logs, _ := notifyHarness(t)
	owner, enquirer, listingID := notifyParties(t, h)
	c := started(t, enquirer, listingID) // 1, to the owner

	// Back and forth, each message the first the other has not read, so no
	// one recipient reaches their own cap of twenty.
	for sent := 1; sent < ratelimit.NotifyGlobal.Burst; sent++ {
		if sent%2 == 1 {
			markRead(t, owner, c.ID)
			mustSend(t, owner, c.ID, "Reply.")
		} else {
			markRead(t, enquirer, c.ID)
			mustSend(t, enquirer, c.ID, "Reply.")
		}
	}
	expectNotifications(t, h, 0, 30, "thirty notifications")

	markRead(t, enquirer, c.ID)
	mustSend(t, enquirer, c.ID, "One too many.") // still 201
	expectNotifications(t, h, 0, 30, "the thirty-first, service-wide")
	if len(logs.lines("notify_global")) != 1 {
		t.Errorf("the global cap was not logged once")
	}

	// Account mail does not share this bucket: a password reset still sends.
	before := h.mail.count()
	if res := h.do(t, http.MethodPost, "/auth/password-reset", map[string]string{"email": "owner@uwaterloo.ca"}); res.StatusCode != http.StatusAccepted {
		t.Fatalf("password reset: %d", res.StatusCode)
	}
	if h.mail.count() != before+1 {
		t.Error("a reset email was not sent while notifications were capped")
	}

	// Thirty a day refills one every forty-eight minutes.
	clk.advance(49 * time.Minute)
	markRead(t, owner, c.ID)
	mustSend(t, owner, c.ID, "Later.")
	expectNotifications(t, h, 0, 31, "after the global bucket refilled")
}

// TestASendFailureChangesNoResponse: with the provider down, every path that
// notifies answers exactly as it would have, and the failure is logged by ids,
// not by address.
func TestASendFailureChangesNoResponse(t *testing.T) {
	h, _, logs, _ := notifyHarness(t)
	owner, enquirer, listingID := notifyParties(t, h)
	_, request := postRequest(t, enquirer, validRequest())
	_, second := createListing(t, owner, privateListing())
	h.mail.failing(true)

	res := start(t, enquirer, listingID, "Hello")
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("start with the provider down: %d", res.StatusCode)
	}
	c := decode[conversationJSON](t, res)
	mustSend(t, owner, c.ID, "Hi!")
	if res := shareAddress(t, owner, c.ID); res.StatusCode != http.StatusOK {
		t.Errorf("share with the provider down: %d", res.StatusCode)
	}
	if res := makeOffer(t, owner, request["id"].(string), second["id"].(string), "An offer"); res.StatusCode != http.StatusCreated {
		t.Errorf("offer with the provider down: %d", res.StatusCode)
	}

	failures := logs.lines("send notification email")
	if len(failures) != 4 {
		t.Fatalf("%d send failures logged, want 4:\n%s", len(failures), strings.Join(failures, "\n"))
	}
	for _, line := range failures {
		if !strings.Contains(line, "conversation_id=") || !strings.Contains(line, "provider unavailable") {
			t.Errorf("failure line lacks the conversation or the error: %s", line)
		}
		if strings.Contains(line, "@uwaterloo.ca") {
			t.Errorf("failure line carries an address: %s", line)
		}
	}
}

// TestNotificationsCarryNoMessageTextOrAddress runs every event that sends a
// notification with marked message text, note and address, and checks every
// email for them and for anyone's email address.
func TestNotificationsCarryNoMessageTextOrAddress(t *testing.T) {
	h, _, _, _ := notifyHarness(t)
	owner, enquirer, listingID := notifyParties(t, h)
	_, request := postRequest(t, enquirer, validRequest())
	_, second := createListing(t, owner, privateListing())

	res := start(t, enquirer, listingID, "MARKED-OPENING-TEXT is the room free?")
	c := decode[conversationJSON](t, res)
	mustSend(t, owner, c.ID, "MARKED-REPLY-TEXT yes it is")
	if res := shareAddress(t, owner, c.ID); res.StatusCode != http.StatusOK {
		t.Fatalf("share: %d", res.StatusCode)
	}
	offered(t, owner, request["id"].(string), second["id"].(string), "MARKED-NOTE-TEXT take a look")

	all := notifications(h)
	for _, m := range all {
		for _, field := range []string{m.Subject, m.Body} {
			for _, secret := range []string{"MARKED-", exactAddress, "Evergreen", "@"} {
				if strings.Contains(field, secret) {
					t.Errorf("email %q carries %q:\n%s", m.Subject, secret, field)
				}
			}
		}
	}
	for _, kind := range []string{"enquirer wants to talk", "owner replied", "owner shared", "owner offered"} {
		found := false
		for _, m := range all {
			found = found || strings.HasPrefix(m.Subject, kind)
		}
		if !found {
			t.Errorf("no %q email among the %d sent", kind, len(all))
		}
	}
}

func TestPreferencesEndpoint(t *testing.T) {
	h, clk, _, _ := notifyHarness(t)

	anonymous := h.do(t, http.MethodPatch, "/me/preferences", map[string]bool{"email_notifications": false})
	if anonymous.StatusCode != http.StatusUnauthorized {
		t.Errorf("anonymous: %d, want 401", anonymous.StatusCode)
	}
	if env := envelope(t, anonymous); env.Code != apierror.CodeUnauthorized {
		t.Errorf("anonymous: code %q", env.Code)
	}

	register(t, h, "student@uwaterloo.ca", "student")

	// A field left out is left as it is.
	res := h.do(t, http.MethodPatch, "/me/preferences", map[string]any{})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("empty body: %d", res.StatusCode)
	}
	if body := decode[map[string]any](t, res); body["email_notifications"] != true {
		t.Errorf("empty body changed the switch: %v", body)
	}

	// A wrong type is refused with the envelope.
	bad := h.do(t, http.MethodPatch, "/me/preferences", map[string]any{"email_notifications": "no"})
	if bad.StatusCode != http.StatusBadRequest {
		t.Errorf("a string for the switch: %d, want 400", bad.StatusCode)
	}
	if env := envelope(t, bad); env.Code != apierror.CodeBadRequest {
		t.Errorf("a string for the switch: code %q", env.Code)
	}

	// Rate limited per user: twenty an hour, then 429, then refilled. Two
	// are spent above; the malformed one costs too, as every limited write
	// is charged before its handler runs.
	for i := 0; i < ratelimit.PreferencesPerUser.Burst-2; i++ {
		if res := h.do(t, http.MethodPatch, "/me/preferences", map[string]bool{"email_notifications": i%2 == 0}); res.StatusCode != http.StatusOK {
			t.Fatalf("PATCH %d: %d", i, res.StatusCode)
		}
	}
	limited := h.do(t, http.MethodPatch, "/me/preferences", map[string]bool{"email_notifications": true})
	if limited.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("past the allowance: %d, want 429", limited.StatusCode)
	}
	assertRefusal(t, limited)
	clk.advance(time.Hour)
	if res := h.do(t, http.MethodPatch, "/me/preferences", map[string]bool{"email_notifications": true}); res.StatusCode != http.StatusOK {
		t.Errorf("after an hour: %d", res.StatusCode)
	}
}

// ------------------------------------------------------------- helpers

func uuidBytes(t *testing.T, id string) []byte {
	t.Helper()
	u := uuid.MustParse(id)
	return u[:]
}

func setVerified(t *testing.T, h *harness, addr string, verified bool) {
	t.Helper()
	h.db.mu.Lock()
	defer h.db.mu.Unlock()
	for id, u := range h.db.users {
		if u.Email == addr {
			u.Verified = verified
			h.db.users[id] = u
			return
		}
	}
	t.Fatalf("no user %s in the fake", addr)
}
