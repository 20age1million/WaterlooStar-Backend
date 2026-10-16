package httpapi_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/20age1million/WaterlooStar-Backend/internal/apierror"
	"github.com/20age1million/WaterlooStar-Backend/internal/db/sqlcgen"
	"github.com/20age1million/WaterlooStar-Backend/internal/ratelimit"
)

// Conversations and messages, through the handlers (Phase 15).
//
// The rules tested hardest: only the two participants can see a conversation,
// either side writes from the first message and either can close it, a
// conversation whose listing was taken down or whose other side is suspended
// is read-only rather than hidden, and opening one reveals nobody's contact
// details.

type conversationJSON struct {
	ID      string `json:"id"`
	Status  string `json:"status"`
	Listing struct {
		ID        string `json:"id"`
		Title     string `json:"title"`
		Available bool   `json:"available"`
	} `json:"listing"`
	Counterpart struct {
		ID       string `json:"id"`
		Username string `json:"username"`
	} `json:"counterpart"`
	ViewerIsOwner  bool `json:"viewer_is_owner"`
	OpenedByViewer bool `json:"opened_by_viewer"`
	CanSend        bool `json:"can_send"`
	LastMessage    *struct {
		FromViewer bool   `json:"from_viewer"`
		Body       string `json:"body"`
	} `json:"last_message"`
	UnreadCount          int     `json:"unread_count"`
	ClosedAt             *string `json:"closed_at"`
	ClosedByViewer       *bool   `json:"closed_by_viewer"`
	CounterpartAvailable bool    `json:"counterpart_available"`
}

type conversationPageJSON struct {
	Data []conversationJSON `json:"data"`
	Meta struct {
		Total int `json:"total"`
	} `json:"meta"`
}

type messageJSON struct {
	ID         string `json:"id"`
	FromViewer bool   `json:"from_viewer"`
	Body       string `json:"body"`
}

type messagePageJSON struct {
	Data []messageJSON `json:"data"`
	Meta struct {
		Total int `json:"total"`
	} `json:"meta"`
}

type unreadJSON struct {
	Conversations int `json:"conversations"`
	Messages      int `json:"messages"`
}

// conversationParties is a harness signed in as the owner of a published
// listing, and a second browser signed in as a verified enquirer.
func conversationParties(t *testing.T, h *harness) (owner, enquirer *harness, listingID string) {
	t.Helper()
	register(t, h, "owner@uwaterloo.ca", "owner")
	res, listing := createListing(t, h, validListing())
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create listing: %d", res.StatusCode)
	}
	e := anotherBrowser(t, h)
	register(t, e, "enquirer@uwaterloo.ca", "enquirer")
	return h, e, listing["id"].(string)
}

func start(t *testing.T, h *harness, listingID, body string) *http.Response {
	t.Helper()
	return h.do(t, http.MethodPost, "/listings/"+listingID+"/conversations", map[string]string{"body": body})
}

// started opens a conversation and returns it as the enquirer sees it.
func started(t *testing.T, e *harness, listingID string) conversationJSON {
	t.Helper()
	res := start(t, e, listingID, "Is the room still free in January?")
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("start: status %d", res.StatusCode)
	}
	return decode[conversationJSON](t, res)
}

func send(t *testing.T, h *harness, conversationID, body string) *http.Response {
	t.Helper()
	return h.do(t, http.MethodPost, "/conversations/"+conversationID+"/messages", map[string]string{"body": body})
}

func getConversation(t *testing.T, h *harness, id string) conversationJSON {
	t.Helper()
	res := h.do(t, http.MethodGet, "/conversations/"+id, nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /conversations/%s: %d", id, res.StatusCode)
	}
	return decode[conversationJSON](t, res)
}

func unread(t *testing.T, h *harness) unreadJSON {
	t.Helper()
	res := h.do(t, http.MethodGet, "/me/conversations/unread", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("unread: %d", res.StatusCode)
	}
	return decode[unreadJSON](t, res)
}

func envelope(t *testing.T, res *http.Response) apierror.Envelope {
	t.Helper()
	return decode[apierror.Envelope](t, res)
}

func TestStartingAConversation(t *testing.T) {
	owner, enquirer, listingID := conversationParties(t, newHarness(t))

	res := start(t, enquirer, listingID, "  Is the room still free in January?  ")
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201", res.StatusCode)
	}
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}

	// Opening a conversation reveals nothing about either side.
	for _, key := range []string{`"address_line"`, `"email"`, `"phone"`, `"lat"`, `"lng"`} {
		if strings.Contains(string(raw), key) {
			t.Errorf("the conversation carries %s: %s", key, raw)
		}
	}

	var c conversationJSON
	if err := json.Unmarshal(raw, &c); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if c.Status != "open" || c.ViewerIsOwner || !c.OpenedByViewer || !c.CanSend || !c.CounterpartAvailable {
		t.Errorf("as the enquirer = %+v", c)
	}
	if c.Counterpart.Username != "owner" || c.Listing.ID != listingID || !c.Listing.Available {
		t.Errorf("counterpart %q, listing %+v", c.Counterpart.Username, c.Listing)
	}
	// Stored trimmed; your own message is never unread.
	if c.LastMessage == nil || !c.LastMessage.FromViewer || c.LastMessage.Body != "Is the room still free in January?" || c.UnreadCount != 0 {
		t.Errorf("last message %+v, unread %d", c.LastMessage, c.UnreadCount)
	}
	if c.ClosedAt != nil || c.ClosedByViewer != nil {
		t.Errorf("a new conversation is closed: %v %v", c.ClosedAt, c.ClosedByViewer)
	}

	// The owner sees it in their inbox, unread, from the other side.
	inbox := decode[conversationPageJSON](t, owner.do(t, http.MethodGet, "/me/conversations", nil))
	if inbox.Meta.Total != 1 || len(inbox.Data) != 1 {
		t.Fatalf("owner inbox = %+v", inbox)
	}
	row := inbox.Data[0]
	if !row.ViewerIsOwner || row.OpenedByViewer || row.UnreadCount != 1 || row.Counterpart.Username != "enquirer" ||
		row.LastMessage == nil || row.LastMessage.FromViewer {
		t.Errorf("owner's row = %+v", row)
	}
	if got := unread(t, owner); got != (unreadJSON{Conversations: 1, Messages: 1}) {
		t.Errorf("owner unread = %+v", got)
	}

	messages := decode[messagePageJSON](t, owner.do(t, http.MethodGet, "/conversations/"+c.ID+"/messages", nil))
	if messages.Meta.Total != 1 || len(messages.Data) != 1 || messages.Data[0].FromViewer {
		t.Errorf("owner's messages = %+v", messages)
	}
}

func TestStartingNeedsAVerifiedStudentAndAListingThePublicCanSee(t *testing.T) {
	ctx := context.Background()

	t.Run("anonymous is refused", func(t *testing.T) {
		owner, _, listingID := conversationParties(t, newHarness(t))
		anon := anotherBrowser(t, owner)
		if res := start(t, anon, listingID, "Hello"); res.StatusCode != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", res.StatusCode)
		}
	})

	t.Run("an unverified student is refused", func(t *testing.T) {
		owner, _, listingID := conversationParties(t, newHarness(t))
		u := anotherBrowser(t, owner)
		if res := u.do(t, http.MethodPost, "/auth/register",
			map[string]string{"email": "new@uwaterloo.ca", "username": "newcomer", "password": testPassword}); res.StatusCode != http.StatusCreated {
			t.Fatalf("register: %d", res.StatusCode)
		}
		login(t, u, "new@uwaterloo.ca")
		res := start(t, u, listingID, "Hello")
		if res.StatusCode != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", res.StatusCode)
		}
		if env := envelope(t, res); env.Code != apierror.CodeForbidden {
			t.Errorf("code = %s", env.Code)
		}
	})

	t.Run("no such listing", func(t *testing.T) {
		_, enquirer, _ := conversationParties(t, newHarness(t))
		if res := start(t, enquirer, uuid.NewString(), "Hello"); res.StatusCode != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", res.StatusCode)
		}
	})

	t.Run("a paused listing is not found", func(t *testing.T) {
		owner, enquirer, listingID := conversationParties(t, newHarness(t))
		if res := owner.do(t, http.MethodPost, "/listings/"+listingID+"/status",
			map[string]string{"status": "paused"}); res.StatusCode != http.StatusOK {
			t.Fatalf("pause: %d", res.StatusCode)
		}
		if res := start(t, enquirer, listingID, "Hello"); res.StatusCode != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", res.StatusCode)
		}
	})

	t.Run("a taken-down listing is not found", func(t *testing.T) {
		owner, enquirer, listingID := conversationParties(t, newHarness(t))
		if _, err := owner.db.RemoveListing(ctx, sqlcgen.RemoveListingParams{
			ID: uuid.MustParse(listingID), Reason: strp("Scam"),
		}); err != nil {
			t.Fatalf("remove: %v", err)
		}
		if res := start(t, enquirer, listingID, "Hello"); res.StatusCode != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", res.StatusCode)
		}
	})

	t.Run("a suspended owner's listing is not found", func(t *testing.T) {
		owner, enquirer, listingID := conversationParties(t, newHarness(t))
		suspend(t, owner, "owner@uwaterloo.ca")
		if res := start(t, enquirer, listingID, "Hello"); res.StatusCode != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", res.StatusCode)
		}
	})

	t.Run("your own listing is refused", func(t *testing.T) {
		owner, _, listingID := conversationParties(t, newHarness(t))
		if res := start(t, owner, listingID, "Hello"); res.StatusCode != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", res.StatusCode)
		}
	})

	t.Run("an empty or over-long first message is refused", func(t *testing.T) {
		_, enquirer, listingID := conversationParties(t, newHarness(t))
		for _, body := range []string{"", "   \n\t ", strings.Repeat("a", 4001)} {
			res := start(t, enquirer, listingID, body)
			if res.StatusCode != http.StatusBadRequest {
				t.Fatalf("%d characters: status %d, want 400", len(body), res.StatusCode)
			}
			if env := envelope(t, res); env.Code != apierror.CodeValidation || env.Details["body"] == "" {
				t.Errorf("envelope = %+v", env)
			}
		}
		// Nothing was started by the refusals.
		if got := decode[conversationPageJSON](t, enquirer.do(t, http.MethodGet, "/me/conversations", nil)); got.Meta.Total != 0 {
			t.Errorf("a refused start left %d conversations", got.Meta.Total)
		}
		// 4000 multi-byte characters is within the limit: it counts characters.
		if res := start(t, enquirer, listingID, strings.Repeat("é", 4000)); res.StatusCode != http.StatusCreated {
			t.Errorf("4000 characters: status %d, want 201", res.StatusCode)
		}
	})
}

func TestASecondStartNamesTheFirstConversation(t *testing.T) {
	owner, enquirer, listingID := conversationParties(t, newHarness(t))
	first := started(t, enquirer, listingID)

	res := start(t, enquirer, listingID, "Me again")
	if res.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want 409", res.StatusCode)
	}
	if env := envelope(t, res); env.Code != apierror.CodeConflict || env.Details["conversation_id"] != first.ID {
		t.Errorf("open: envelope = %+v, want conversation_id %s", env, first.ID)
	}

	// Closed, it still names it: closing is what stops a second one.
	if res := owner.do(t, http.MethodPost, "/conversations/"+first.ID+"/close", nil); res.StatusCode != http.StatusOK {
		t.Fatalf("close: %d", res.StatusCode)
	}
	res = start(t, enquirer, listingID, "Please?")
	if res.StatusCode != http.StatusConflict {
		t.Fatalf("after closing: status = %d, want 409", res.StatusCode)
	}
	if env := envelope(t, res); env.Details["conversation_id"] != first.ID || !strings.Contains(env.Message, "closed") {
		t.Errorf("closed: envelope = %+v", env)
	}
}

func TestBothSidesWriteFromTheFirstMessage(t *testing.T) {
	owner, enquirer, listingID := conversationParties(t, newHarness(t))
	c := started(t, enquirer, listingID)

	// A follow-up before any reply: there is no accept step.
	res := send(t, enquirer, c.ID, "I could view it on Saturday.")
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("follow-up: status %d, want 201", res.StatusCode)
	}
	if m := decode[messageJSON](t, res); !m.FromViewer || m.Body != "I could view it on Saturday." {
		t.Errorf("sent message = %+v", m)
	}
	if got := unread(t, owner); got != (unreadJSON{Conversations: 1, Messages: 2}) {
		t.Errorf("owner unread = %+v, want 1/2", got)
	}

	// The owner reads: their count clears and the enquirer's is untouched.
	if res := owner.do(t, http.MethodPost, "/conversations/"+c.ID+"/read", nil); res.StatusCode != http.StatusNoContent {
		t.Fatalf("read: %d", res.StatusCode)
	}
	if got := unread(t, owner); got != (unreadJSON{}) {
		t.Errorf("owner unread after reading = %+v", got)
	}

	// The owner replies; it is unread for the enquirer, not for the owner.
	if res := send(t, owner, c.ID, "Saturday works."); res.StatusCode != http.StatusCreated {
		t.Fatalf("reply: %d", res.StatusCode)
	}
	if got := unread(t, enquirer); got != (unreadJSON{Conversations: 1, Messages: 1}) {
		t.Errorf("enquirer unread = %+v, want 1/1", got)
	}
	if got := unread(t, owner); got != (unreadJSON{}) {
		t.Errorf("the owner's own reply is unread for them: %+v", got)
	}
	if got := getConversation(t, enquirer, c.ID); got.UnreadCount != 1 || got.LastMessage == nil || got.LastMessage.FromViewer {
		t.Errorf("enquirer's view = %+v", got)
	}
	if res := enquirer.do(t, http.MethodPost, "/conversations/"+c.ID+"/read", nil); res.StatusCode != http.StatusNoContent {
		t.Fatalf("enquirer read: %d", res.StatusCode)
	}
	if got := unread(t, enquirer); got != (unreadJSON{}) {
		t.Errorf("enquirer unread after reading = %+v", got)
	}

	// Newest first, from each side's point of view.
	page := decode[messagePageJSON](t, enquirer.do(t, http.MethodGet, "/conversations/"+c.ID+"/messages?per_page=2", nil))
	if page.Meta.Total != 3 || len(page.Data) != 2 || page.Data[0].Body != "Saturday works." || page.Data[0].FromViewer {
		t.Errorf("messages page = %+v", page)
	}

	// An empty message is refused.
	if res := send(t, enquirer, c.ID, "  "); res.StatusCode != http.StatusBadRequest {
		t.Errorf("empty message: status %d, want 400", res.StatusCode)
	}
}

func TestEitherSideCanCloseAndThenNobodySends(t *testing.T) {
	for _, closer := range []string{"owner", "enquirer"} {
		t.Run(closer+" closes", func(t *testing.T) {
			owner, enquirer, listingID := conversationParties(t, newHarness(t))
			c := started(t, enquirer, listingID)

			who, other := owner, enquirer
			if closer == "enquirer" {
				who, other = enquirer, owner
			}

			res := who.do(t, http.MethodPost, "/conversations/"+c.ID+"/close", nil)
			if res.StatusCode != http.StatusOK {
				t.Fatalf("close: status %d, want 200", res.StatusCode)
			}
			closed := decode[conversationJSON](t, res)
			if closed.Status != "closed" || closed.ClosedAt == nil || closed.ClosedByViewer == nil || !*closed.ClosedByViewer || closed.CanSend {
				t.Errorf("closer's view = %+v", closed)
			}
			theirs := getConversation(t, other, c.ID)
			if theirs.Status != "closed" || theirs.ClosedByViewer == nil || *theirs.ClosedByViewer || theirs.CanSend {
				t.Errorf("other side's view = %+v", theirs)
			}

			for _, h := range []*harness{owner, enquirer} {
				res := send(t, h, c.ID, "One more thing")
				if res.StatusCode != http.StatusConflict {
					t.Fatalf("send after close: status %d, want 409", res.StatusCode)
				}
				if env := envelope(t, res); env.Code != apierror.CodeConflict || env.Message != "This conversation was closed." {
					t.Errorf("envelope = %+v", env)
				}
			}

			// Closing again, by either side, answers 200 unchanged.
			again := decode[conversationJSON](t, other.do(t, http.MethodPost, "/conversations/"+c.ID+"/close", nil))
			if again.ClosedAt == nil || *again.ClosedAt != *closed.ClosedAt || again.ClosedByViewer == nil || *again.ClosedByViewer {
				t.Errorf("second close changed it: %+v", again)
			}

			// It stays readable, and in both inboxes under status=closed.
			for _, h := range []*harness{owner, enquirer} {
				got := decode[conversationPageJSON](t, h.do(t, http.MethodGet, "/me/conversations?status=closed", nil))
				if got.Meta.Total != 1 {
					t.Errorf("status=closed inbox = %+v", got)
				}
			}
		})
	}
}

func TestAStrangerGetsNotFoundOnEveryConversationRoute(t *testing.T) {
	owner, enquirer, listingID := conversationParties(t, newHarness(t))
	c := started(t, enquirer, listingID)

	stranger := anotherBrowser(t, owner)
	register(t, stranger, "stranger@uwaterloo.ca", "stranger")

	routes := []struct{ method, path string }{
		{http.MethodGet, "/conversations/" + c.ID},
		{http.MethodGet, "/conversations/" + c.ID + "/messages"},
		{http.MethodPost, "/conversations/" + c.ID + "/messages"},
		{http.MethodPost, "/conversations/" + c.ID + "/read"},
		{http.MethodPost, "/conversations/" + c.ID + "/close"},
	}
	for _, r := range routes {
		var body any
		if r.method == http.MethodPost && strings.HasSuffix(r.path, "/messages") {
			body = map[string]string{"body": "Let me in"}
		}
		res := stranger.do(t, r.method, r.path, body)
		if res.StatusCode != http.StatusNotFound {
			t.Errorf("%s %s: status %d, want 404", r.method, r.path, res.StatusCode)
			continue
		}
		if env := envelope(t, res); env.Code != apierror.CodeNotFound {
			t.Errorf("%s %s: code %s", r.method, r.path, env.Code)
		}
	}

	// Nothing the stranger did changed it.
	if got := getConversation(t, enquirer, c.ID); got.Status != "open" {
		t.Errorf("after the stranger: %+v", got)
	}
	if got := decode[conversationPageJSON](t, stranger.do(t, http.MethodGet, "/me/conversations", nil)); got.Meta.Total != 0 {
		t.Errorf("the stranger's inbox holds %d", got.Meta.Total)
	}

	// And anonymous callers are asked to log in.
	anon := anotherBrowser(t, owner)
	for _, path := range []string{"/me/conversations", "/me/conversations/unread", "/conversations/" + c.ID} {
		if res := anon.do(t, http.MethodGet, path, nil); res.StatusCode != http.StatusUnauthorized {
			t.Errorf("anonymous GET %s: %d, want 401", path, res.StatusCode)
		}
	}
}

func TestSuspensionAndTakedownMakeAConversationReadOnly(t *testing.T) {
	ctx := context.Background()

	t.Run("a suspended counterpart", func(t *testing.T) {
		owner, enquirer, listingID := conversationParties(t, newHarness(t))
		c := started(t, enquirer, listingID)

		suspend(t, owner, "owner@uwaterloo.ca")

		// The enquirer still sees it, flagged, and cannot send.
		got := getConversation(t, enquirer, c.ID)
		if got.CounterpartAvailable || got.CanSend || got.Listing.Available {
			t.Errorf("with the owner suspended = %+v", got)
		}
		inbox := decode[conversationPageJSON](t, enquirer.do(t, http.MethodGet, "/me/conversations", nil))
		if inbox.Meta.Total != 1 {
			t.Errorf("the conversation left the enquirer's inbox")
		}
		res := send(t, enquirer, c.ID, "Hello?")
		if res.StatusCode != http.StatusConflict {
			t.Fatalf("send: status %d, want 409", res.StatusCode)
		}
		if env := envelope(t, res); env.Message != "This account is no longer available." {
			t.Errorf("message = %q", env.Message)
		}

		// Reinstating restores it: nothing was rewritten.
		reinstate(t, owner, "owner@uwaterloo.ca")
		if got := getConversation(t, enquirer, c.ID); !got.CanSend || !got.CounterpartAvailable {
			t.Errorf("after reinstating = %+v", got)
		}
		if res := send(t, enquirer, c.ID, "Hello?"); res.StatusCode != http.StatusCreated {
			t.Errorf("send after reinstating: %d", res.StatusCode)
		}
	})

	t.Run("a taken-down listing", func(t *testing.T) {
		owner, enquirer, listingID := conversationParties(t, newHarness(t))
		c := started(t, enquirer, listingID)
		id := uuid.MustParse(listingID)

		if _, err := owner.db.RemoveListing(ctx, sqlcgen.RemoveListingParams{ID: id, Reason: strp("Scam")}); err != nil {
			t.Fatalf("remove: %v", err)
		}
		for _, h := range []*harness{owner, enquirer} {
			got := getConversation(t, h, c.ID)
			if got.CanSend || got.Listing.Available {
				t.Errorf("taken down = %+v", got)
			}
			res := send(t, h, c.ID, "Still there?")
			if res.StatusCode != http.StatusConflict {
				t.Fatalf("send: status %d, want 409", res.StatusCode)
			}
			if env := envelope(t, res); env.Message != "This listing is no longer available." {
				t.Errorf("message = %q", env.Message)
			}
		}

		if _, err := owner.db.RestoreListing(ctx, id); err != nil {
			t.Fatalf("restore: %v", err)
		}
		if res := send(t, enquirer, c.ID, "Still there?"); res.StatusCode != http.StatusCreated {
			t.Errorf("send after restoring: %d", res.StatusCode)
		}
	})

	t.Run("a listing its owner paused carries on", func(t *testing.T) {
		owner, enquirer, listingID := conversationParties(t, newHarness(t))
		c := started(t, enquirer, listingID)

		// An owner archives a place once it is let; the people arranging the
		// lease still need to talk.
		if res := owner.do(t, http.MethodPost, "/listings/"+listingID+"/status",
			map[string]string{"status": "archived"}); res.StatusCode != http.StatusOK {
			t.Fatalf("archive: %d", res.StatusCode)
		}
		got := getConversation(t, enquirer, c.ID)
		if got.Listing.Available || !got.CanSend {
			t.Errorf("archived = %+v", got)
		}
		if res := send(t, owner, c.ID, "The lease is ready to sign."); res.StatusCode != http.StatusCreated {
			t.Errorf("send about an archived listing: %d", res.StatusCode)
		}
	})
}

func TestTheInboxFiltersBySideAndStatus(t *testing.T) {
	owner, enquirer, listingID := conversationParties(t, newHarness(t))
	started(t, enquirer, listingID)

	// The enquirer posts a place too, and the owner asks about it.
	res, theirs := createListing(t, enquirer, validListing())
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("enquirer's listing: %d", res.StatusCode)
	}
	if res := start(t, owner, theirs["id"].(string), "Is yours free?"); res.StatusCode != http.StatusCreated {
		t.Fatalf("second start: %d", res.StatusCode)
	}

	total := func(query string) int {
		t.Helper()
		res := owner.do(t, http.MethodGet, "/me/conversations"+query, nil)
		if res.StatusCode != http.StatusOK {
			t.Fatalf("GET /me/conversations%s: %d", query, res.StatusCode)
		}
		return decode[conversationPageJSON](t, res).Meta.Total
	}
	if got := total(""); got != 2 {
		t.Errorf("all = %d, want 2", got)
	}
	if got := total("?side=owner"); got != 1 {
		t.Errorf("side=owner = %d, want 1", got)
	}
	if got := total("?side=enquirer"); got != 1 {
		t.Errorf("side=enquirer = %d, want 1", got)
	}
	if got := total("?status=open"); got != 2 {
		t.Errorf("status=open = %d, want 2", got)
	}
	if got := total("?status=closed"); got != 0 {
		t.Errorf("status=closed = %d, want 0", got)
	}

	for _, bad := range []string{"?status=pending", "?side=renter"} {
		res := owner.do(t, http.MethodGet, "/me/conversations"+bad, nil)
		if res.StatusCode != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400", bad, res.StatusCode)
		}
	}
}

// ------------------------------------------------------------ rate limits

func TestStartingConversationsIsThrottledPerUser(t *testing.T) {
	h, clk := limitedHarness(t)
	_, enquirer, listingID := conversationParties(t, h)

	// Every attempt is charged, including the ones that find the conversation
	// already exists, so one listing is enough to spend the allowance.
	for i := range ratelimit.ConversationStartPerUser.Burst {
		want := http.StatusConflict
		if i == 0 {
			want = http.StatusCreated
		}
		if res := start(t, enquirer, listingID, "Hello"); res.StatusCode != want {
			t.Fatalf("attempt %d: status %d, want %d", i+1, res.StatusCode, want)
		}
	}
	assertRefusal(t, start(t, enquirer, listingID, "Hello"))

	clk.advance(ratelimit.ConversationStartPerUser.Window + time.Second)
	if res := start(t, enquirer, listingID, "Hello"); res.StatusCode != http.StatusConflict {
		t.Fatalf("after the window: status %d, want 409", res.StatusCode)
	}
}

func TestSendingMessagesIsThrottledPerUser(t *testing.T) {
	h, clk := limitedHarness(t)
	_, enquirer, listingID := conversationParties(t, h)
	c := started(t, enquirer, listingID)

	for i := range ratelimit.MessagePerUser.Burst {
		if res := send(t, enquirer, c.ID, "Another thought"); res.StatusCode != http.StatusCreated {
			t.Fatalf("message %d: status %d, want 201", i+1, res.StatusCode)
		}
	}
	assertRefusal(t, send(t, enquirer, c.ID, "Another thought"))

	// Reading, marking read and closing are never limited.
	for range 5 {
		if res := enquirer.do(t, http.MethodGet, "/conversations/"+c.ID+"/messages", nil); res.StatusCode != http.StatusOK {
			t.Fatalf("read while throttled: %d", res.StatusCode)
		}
		if res := enquirer.do(t, http.MethodPost, "/conversations/"+c.ID+"/read", nil); res.StatusCode != http.StatusNoContent {
			t.Fatalf("mark read while throttled: %d", res.StatusCode)
		}
	}

	clk.advance(ratelimit.MessagePerUser.Window + time.Second)
	if res := send(t, enquirer, c.ID, "Back again"); res.StatusCode != http.StatusCreated {
		t.Fatalf("after the window: status %d, want 201", res.StatusCode)
	}
}

// Messages are append-only: no query may edit or delete one, so what a
// conversation shows can be trusted if someone reports it. The same guard as
// the admin ledger's, over the generated queries.
func TestMessagesHaveNoUpdateOrDelete(t *testing.T) {
	dir := filepath.Join("..", "db", "sqlcgen")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}

	rewrite := regexp.MustCompile(`(?is)\b(update\s+messages|delete\s+from\s+messages)\b`)
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		src, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		if m := rewrite.Find(src); m != nil {
			t.Errorf("%s contains %q. Messages are append-only: they cannot be edited or deleted.", e.Name(), m)
		}
	}
}

// ------------------------------------------------------------- helpers

func strp(s string) *string { return &s }

// suspend and reinstate act on the fake directly: the admin surface has its
// own tests, and these are about what suspension does to a conversation.
func suspend(t *testing.T, h *harness, email string) {
	t.Helper()
	u, err := h.db.GetUserByEmail(context.Background(), email)
	if err != nil {
		t.Fatalf("find %s: %v", email, err)
	}
	if _, err := h.db.SuspendUser(context.Background(), sqlcgen.SuspendUserParams{ID: u.ID, Reason: strp(goodReason)}); err != nil {
		t.Fatalf("suspend %s: %v", email, err)
	}
}

func reinstate(t *testing.T, h *harness, email string) {
	t.Helper()
	u, err := h.db.GetUserByEmail(context.Background(), email)
	if err != nil {
		t.Fatalf("find %s: %v", email, err)
	}
	if _, err := h.db.ReinstateUser(context.Background(), u.ID); err != nil {
		t.Fatalf("reinstate %s: %v", email, err)
	}
}
