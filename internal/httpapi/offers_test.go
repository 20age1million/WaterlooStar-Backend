package httpapi_test

import (
	"net/http"
	"testing"

	"github.com/20age1million/WaterlooStar-Backend/internal/apierror"
)

// Offers, through the handlers.
//
// The rule tested hardest is visibility: a request's offers belong to the
// student who posted it and to each owner who made one, and to nobody else.
// Getting that wrong turns a request into an auction where everyone sees the
// competition.

// twoAccounts signs a harness in as a student, posts a request, then signs in
// as a second account holding a published listing. The returned harness is
// logged in as the owner; ids for both are returned.
func twoAccounts(t *testing.T) (h *harness, requestID, listingID string) {
	t.Helper()

	h, _ = signedIn(t, true) // poster@uwaterloo.ca, the student
	_, request := postRequest(t, h, validRequest())
	requestID = request["id"].(string)

	// A second verified account in the same harness.
	register(t, h, "owner@uwaterloo.ca", "owner")
	_, listing := createListing(t, h, validListing())
	listingID = listing["id"].(string)

	return h, requestID, listingID
}

// login switches the harness to an account that already exists.
func login(t *testing.T, h *harness, email string) {
	t.Helper()
	if res := h.do(t, http.MethodPost, "/auth/login",
		map[string]string{"email": email, "password": testPassword}); res.StatusCode != http.StatusOK {
		t.Fatalf("login %s: %d", email, res.StatusCode)
	}
}

// register creates and verifies an account, leaving the harness signed in as it.
func register(t *testing.T, h *harness, email, username string) {
	t.Helper()

	if res := h.do(t, http.MethodPost, "/auth/register",
		map[string]string{"email": email, "username": username, "password": testPassword}); res.StatusCode != http.StatusCreated {
		t.Fatalf("register %s: %d", email, res.StatusCode)
	}
	if res := h.do(t, http.MethodPost, "/auth/verify",
		map[string]string{"token": h.mail.token(t)}); res.StatusCode != http.StatusOK {
		t.Fatalf("verify %s: %d", email, res.StatusCode)
	}
	if res := h.do(t, http.MethodPost, "/auth/login",
		map[string]string{"email": email, "password": testPassword}); res.StatusCode != http.StatusOK {
		t.Fatalf("login %s: %d", email, res.StatusCode)
	}
}

type offerBody struct {
	ID        string `json:"id"`
	RequestID string `json:"request_id"`
	Note      string `json:"note"`
	Listing   struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	} `json:"listing"`
	Owner struct {
		Username string `json:"username"`
	} `json:"owner"`
}

func TestOfferingAListingAgainstARequest(t *testing.T) {
	h, requestID, listingID := twoAccounts(t)

	res := h.do(t, http.MethodPost, "/requests/"+requestID+"/offers",
		map[string]any{"listing_id": listingID, "note": "Two doors from the library."})
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201", res.StatusCode)
	}
	offer := decode[offerBody](t, res)
	if offer.Listing.ID != listingID {
		t.Errorf("offer carries listing %q, want %q", offer.Listing.ID, listingID)
	}
	if offer.Owner.Username != "owner" || offer.Note != "Two doors from the library." {
		t.Errorf("offer = %+v", offer)
	}

	// The owner sees their own offer.
	mine := h.do(t, http.MethodGet, "/requests/"+requestID+"/offers", nil)
	if rows := decode[[]offerBody](t, mine); len(rows) != 1 {
		t.Fatalf("the owner sees %d of their own offers, want 1", len(rows))
	}

	// And the student who posted the request sees it, with the place attached.
	login(t, h, "poster@uwaterloo.ca")
	theirs := h.do(t, http.MethodGet, "/requests/"+requestID+"/offers", nil)
	rows := decode[[]offerBody](t, theirs)
	if len(rows) != 1 || rows[0].Listing.Title == "" {
		t.Fatalf("the poster sees %+v", rows)
	}

	// The count on the request reflects it.
	_, page := getJSON[requestPageBody](t, h.server.URL+"/requests")
	if page.Data[0].Offers != 1 {
		t.Errorf("request shows %d offers, want 1", page.Data[0].Offers)
	}
}

func TestOfferingNeedsAVerifiedStudentAndYourOwnPublishedListing(t *testing.T) {
	t.Run("anonymous is refused", func(t *testing.T) {
		h, requestID, listingID := twoAccounts(t)
		h.do(t, http.MethodPost, "/auth/logout", nil)

		res := h.do(t, http.MethodPost, "/requests/"+requestID+"/offers",
			map[string]any{"listing_id": listingID})
		if res.StatusCode != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", res.StatusCode)
		}
	})

	t.Run("someone else's listing is not found", func(t *testing.T) {
		h, requestID, listingID := twoAccounts(t)

		// A third account offering a listing it does not own. 404 rather than
		// 403: whether that listing exists is not their business.
		register(t, h, "meddler@uwaterloo.ca", "meddler")
		res := h.do(t, http.MethodPost, "/requests/"+requestID+"/offers",
			map[string]any{"listing_id": listingID})
		if res.StatusCode != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", res.StatusCode)
		}
	})

	t.Run("an unpublished listing is refused", func(t *testing.T) {
		h, requestID, listingID := twoAccounts(t)

		if res := h.do(t, http.MethodPost, "/listings/"+listingID+"/status",
			map[string]string{"status": "paused"}); res.StatusCode != http.StatusOK {
			t.Fatalf("pause: %d", res.StatusCode)
		}

		// Offering a place nobody can see asks the student to take it on trust.
		res := h.do(t, http.MethodPost, "/requests/"+requestID+"/offers",
			map[string]any{"listing_id": listingID})
		if res.StatusCode != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", res.StatusCode)
		}
	})

	t.Run("a request that is not published is not found", func(t *testing.T) {
		h, requestID, listingID := twoAccounts(t)

		// The student pauses their request.
		login(t, h, "poster@uwaterloo.ca")
		if res := h.do(t, http.MethodPost, "/requests/"+requestID+"/status",
			map[string]string{"status": "paused"}); res.StatusCode != http.StatusOK {
			t.Fatalf("pause: %d", res.StatusCode)
		}

		login(t, h, "owner@uwaterloo.ca")
		res := h.do(t, http.MethodPost, "/requests/"+requestID+"/offers",
			map[string]any{"listing_id": listingID})
		if res.StatusCode != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", res.StatusCode)
		}
	})
}

func TestTheSameListingCannotBeOfferedTwiceThroughTheAPI(t *testing.T) {
	h, requestID, listingID := twoAccounts(t)
	body := map[string]any{"listing_id": listingID}

	if res := h.do(t, http.MethodPost, "/requests/"+requestID+"/offers", body); res.StatusCode != http.StatusCreated {
		t.Fatalf("first offer: %d", res.StatusCode)
	}
	res := h.do(t, http.MethodPost, "/requests/"+requestID+"/offers", body)
	if res.StatusCode != http.StatusConflict {
		t.Fatalf("second offer: %d, want 409", res.StatusCode)
	}
}

func TestOffersAreNotVisibleToOtherOwners(t *testing.T) {
	h, requestID, listingID := twoAccounts(t)
	if res := h.do(t, http.MethodPost, "/requests/"+requestID+"/offers",
		map[string]any{"listing_id": listingID}); res.StatusCode != http.StatusCreated {
		t.Fatalf("offer: %d", res.StatusCode)
	}

	// A second owner, with their own place and their own offer.
	register(t, h, "rival@uwaterloo.ca", "rival")
	rival := validListing()
	rival["title"] = "A different room entirely"
	_, rivalListing := createListing(t, h, rival)
	if res := h.do(t, http.MethodPost, "/requests/"+requestID+"/offers",
		map[string]any{"listing_id": rivalListing["id"].(string)}); res.StatusCode != http.StatusCreated {
		t.Fatalf("rival offer: %d", res.StatusCode)
	}

	// The rival sees only their own, not the competition.
	rows := decode[[]offerBody](t, h.do(t, http.MethodGet, "/requests/"+requestID+"/offers", nil))
	if len(rows) != 1 || rows[0].Owner.Username != "rival" {
		t.Fatalf("an owner saw %+v", rows)
	}

	// The student sees both.
	login(t, h, "poster@uwaterloo.ca")
	all := decode[[]offerBody](t, h.do(t, http.MethodGet, "/requests/"+requestID+"/offers", nil))
	if len(all) != 2 {
		t.Fatalf("the poster sees %d offers, want 2", len(all))
	}
}

func TestWithdrawingAndTakingTheListingDown(t *testing.T) {
	t.Run("the owner withdraws", func(t *testing.T) {
		h, requestID, listingID := twoAccounts(t)
		res := h.do(t, http.MethodPost, "/requests/"+requestID+"/offers",
			map[string]any{"listing_id": listingID})
		offer := decode[offerBody](t, res)

		if res := h.do(t, http.MethodDelete, "/offers/"+offer.ID, nil); res.StatusCode != http.StatusNoContent {
			t.Fatalf("withdraw: %d", res.StatusCode)
		}

		login(t, h, "poster@uwaterloo.ca")
		rows := decode[[]offerBody](t, h.do(t, http.MethodGet, "/requests/"+requestID+"/offers", nil))
		if len(rows) != 0 {
			t.Errorf("a withdrawn offer was still shown: %+v", rows)
		}
	})

	t.Run("taking the listing down withdraws it", func(t *testing.T) {
		h, requestID, listingID := twoAccounts(t)
		if res := h.do(t, http.MethodPost, "/requests/"+requestID+"/offers",
			map[string]any{"listing_id": listingID}); res.StatusCode != http.StatusCreated {
			t.Fatalf("offer: %d", res.StatusCode)
		}

		// Nothing touches the offer. The place going away is enough.
		if res := h.do(t, http.MethodDelete, "/listings/"+listingID, nil); res.StatusCode != http.StatusNoContent {
			t.Fatalf("take listing down: %d", res.StatusCode)
		}

		login(t, h, "poster@uwaterloo.ca")
		rows := decode[[]offerBody](t, h.do(t, http.MethodGet, "/requests/"+requestID+"/offers", nil))
		if len(rows) != 0 {
			t.Errorf("an offer outlived its listing: %+v", rows)
		}

		_, page := getJSON[requestPageBody](t, h.server.URL+"/requests")
		if page.Data[0].Offers != 0 {
			t.Errorf("request still counts %d offers", page.Data[0].Offers)
		}
	})

	t.Run("someone else's offer is not found", func(t *testing.T) {
		h, requestID, listingID := twoAccounts(t)
		offer := decode[offerBody](t, h.do(t, http.MethodPost, "/requests/"+requestID+"/offers",
			map[string]any{"listing_id": listingID}))

		// Even the student the offer was made to cannot withdraw it.
		login(t, h, "poster@uwaterloo.ca")
		if res := h.do(t, http.MethodDelete, "/offers/"+offer.ID, nil); res.StatusCode != http.StatusNotFound {
			t.Errorf("status = %d, want 404", res.StatusCode)
		}
	})
}

// ------------------------------------------- Phase 17: offers open conversations
//
// An offer opens a conversation with the request's poster about the offered
// listing, or joins the one they already have, and the poster answers it
// there. The rules tested: every row of the table in makeOffer, withdrawal
// leaving the conversation alone, two offers sharing one, and no offer path
// revealing the address.

type offerRefJSON struct {
	ID           string `json:"id"`
	Note         string `json:"note"`
	Conversation *struct {
		ID            string `json:"id"`
		Status        string `json:"status"`
		PosterReplied bool   `json:"poster_replied"`
	} `json:"conversation"`
}

// offerParties is an owner with a listing at an exact address and a student
// with a published request, each in their own browser.
func offerParties(t *testing.T) (owner, poster *harness, requestID, listingID string) {
	t.Helper()
	owner, poster, listingID = privacyParties(t)
	res, request := postRequest(t, poster, validRequest())
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("post request: %d", res.StatusCode)
	}
	return owner, poster, request["id"].(string), listingID
}

func makeOffer(t *testing.T, h *harness, requestID, listingID, note string) *http.Response {
	t.Helper()
	body := map[string]any{"listing_id": listingID}
	if note != "" {
		body["note"] = note
	}
	return h.do(t, http.MethodPost, "/requests/"+requestID+"/offers", body)
}

// offered makes an offer that must succeed and must name a conversation.
func offered(t *testing.T, h *harness, requestID, listingID, note string) offerRefJSON {
	t.Helper()
	res := makeOffer(t, h, requestID, listingID, note)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("offer: status %d, want 201", res.StatusCode)
	}
	o := decode[offerRefJSON](t, res)
	if o.Conversation == nil || o.Conversation.ID == "" {
		t.Fatalf("the offer names no conversation: %+v", o)
	}
	return o
}

func offersOn(t *testing.T, h *harness, requestID string) []offerRefJSON {
	t.Helper()
	res := h.do(t, http.MethodGet, "/requests/"+requestID+"/offers", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("list offers: %d", res.StatusCode)
	}
	return decode[[]offerRefJSON](t, res)
}

func messagesIn(t *testing.T, h *harness, conversationID string) messagePageJSON {
	t.Helper()
	res := h.do(t, http.MethodGet, "/conversations/"+conversationID+"/messages", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("messages: %d", res.StatusCode)
	}
	return decode[messagePageJSON](t, res)
}

// assertNothingRevealed fails if the conversation, read by h, has a shared
// address or a place: an offer reveals nothing.
func assertNothingRevealed(t *testing.T, label string, h *harness, conversationID string) {
	t.Helper()
	c := raw(t, h.do(t, http.MethodGet, "/conversations/"+conversationID, nil), http.StatusOK)
	place, sharedAt := sharedState(t, label, c)
	if place != nil || sharedAt != nil {
		t.Errorf("%s: place = %v, address_shared_at = %v, want both null", label, place, sharedAt)
	}
}

func TestAnOfferOpensAConversationWithThePoster(t *testing.T) {
	owner, poster, requestID, listingID := offerParties(t)

	o := offered(t, owner, requestID, listingID, "  Two doors from the library.  ")
	if o.Conversation.Status != "open" || o.Conversation.PosterReplied {
		t.Errorf("new offer's conversation = %+v, want open and not replied", *o.Conversation)
	}
	id := o.Conversation.ID

	// The poster sees it as a conversation the owner opened, with the note as
	// its first, unread message.
	c := getConversation(t, poster, id)
	if c.ViewerIsOwner || c.OpenedByViewer || c.Listing.ID != listingID || c.Counterpart.Username != "owner" {
		t.Errorf("the poster sees %+v", c)
	}
	if c.LastMessage == nil || c.LastMessage.Body != "Two doors from the library." || c.LastMessage.FromViewer {
		t.Errorf("last message = %+v, want the trimmed note from the owner", c.LastMessage)
	}
	if c.UnreadCount != 1 || !c.CanSend {
		t.Errorf("unread = %d, can_send = %v", c.UnreadCount, c.CanSend)
	}
	if mine := getConversation(t, owner, id); !mine.OpenedByViewer || !mine.ViewerIsOwner {
		t.Errorf("the owner sees %+v", mine)
	}
	assertNothingRevealed(t, "after the offer (poster)", poster, id)
	assertNothingRevealed(t, "after the offer (owner)", owner, id)

	// The poster's list shows the same conversation, still waiting on them.
	rows := offersOn(t, poster, requestID)
	if len(rows) != 1 || rows[0].Conversation == nil || rows[0].Conversation.ID != id || rows[0].Conversation.PosterReplied {
		t.Fatalf("the poster's offers = %+v", rows)
	}

	// Replying is the answer.
	if res := send(t, poster, id, "Yes please, can I see it?"); res.StatusCode != http.StatusCreated {
		t.Fatalf("reply: %d", res.StatusCode)
	}
	for who, h := range map[string]*harness{"poster": poster, "owner": owner} {
		rows := offersOn(t, h, requestID)
		if len(rows) != 1 || !rows[0].Conversation.PosterReplied {
			t.Errorf("%s: poster_replied did not turn true: %+v", who, rows)
		}
	}
	// A reply reveals nothing either.
	assertNothingRevealed(t, "after the reply", poster, id)

	// Only the owner's share does.
	if res := shareAddress(t, owner, id); res.StatusCode != http.StatusOK {
		t.Fatalf("share: %d", res.StatusCode)
	}
	shared := raw(t, poster.do(t, http.MethodGet, "/conversations/"+id, nil), http.StatusOK)
	if place, _ := sharedState(t, "after the share", shared); place == nil || place["address_line"] != exactAddress {
		t.Errorf("after the share the poster sees place = %v", place)
	}
}

func TestAnOfferWithNoNoteOpensAnEmptyConversation(t *testing.T) {
	owner, poster, requestID, listingID := offerParties(t)

	id := offered(t, owner, requestID, listingID, "").Conversation.ID

	if page := messagesIn(t, poster, id); page.Meta.Total != 0 {
		t.Errorf("an offer with no note posted %d messages", page.Meta.Total)
	}
	c := getConversation(t, poster, id)
	if c.LastMessage != nil || c.UnreadCount != 0 || c.OpenedByViewer {
		t.Errorf("empty offer conversation = %+v", c)
	}
	// It is in the poster's inbox all the same, by its created_at.
	inbox := decode[conversationPageJSON](t, poster.do(t, http.MethodGet, "/me/conversations", nil))
	if inbox.Meta.Total != 1 || inbox.Data[0].ID != id {
		t.Errorf("the poster's inbox = %+v", inbox)
	}
	assertNothingRevealed(t, "empty offer conversation", poster, id)
}

func TestAnOfferJoinsAnOpenConversation(t *testing.T) {
	owner, poster, requestID, listingID := offerParties(t)

	// The poster asked about the listing first.
	existing := started(t, poster, listingID)

	o := offered(t, owner, requestID, listingID, "It's still free, and here it is as an offer.")
	if o.Conversation.ID != existing.ID {
		t.Fatalf("the offer opened %s, want it to join %s", o.Conversation.ID, existing.ID)
	}
	// The poster has already written there, so the offer is not waiting on them.
	if !o.Conversation.PosterReplied {
		t.Error("poster_replied is false in a conversation the poster started")
	}

	page := messagesIn(t, poster, existing.ID)
	if page.Meta.Total != 2 || page.Data[0].Body != "It's still free, and here it is as an offer." || page.Data[0].FromViewer {
		t.Errorf("messages = %+v, want the poster's and then the owner's note", page.Data)
	}
	if c := getConversation(t, poster, existing.ID); !c.OpenedByViewer {
		t.Error("joining changed who opened the conversation")
	}
	assertNothingRevealed(t, "joined conversation", poster, existing.ID)
}

func TestAnOfferReopensAConversationTheOwnerClosed(t *testing.T) {
	owner, poster, requestID, listingID := offerParties(t)

	existing := started(t, poster, listingID)
	if res := owner.do(t, http.MethodPost, "/conversations/"+existing.ID+"/close", nil); res.StatusCode != http.StatusOK {
		t.Fatalf("close: %d", res.StatusCode)
	}

	o := offered(t, owner, requestID, listingID, "Changed my mind - it's yours if you want it.")
	if o.Conversation.ID != existing.ID || o.Conversation.Status != "open" {
		t.Fatalf("offer conversation = %+v, want %s reopened", *o.Conversation, existing.ID)
	}
	c := getConversation(t, poster, existing.ID)
	if c.Status != "open" || c.ClosedAt != nil || !c.CanSend {
		t.Errorf("the reopened conversation = %+v", c)
	}
	if res := send(t, poster, existing.ID, "Great, thanks."); res.StatusCode != http.StatusCreated {
		t.Errorf("reply in the reopened conversation: %d", res.StatusCode)
	}
	assertNothingRevealed(t, "reopened conversation", poster, existing.ID)
}

func TestAnOfferIsRefusedWhenThePosterClosedTheConversation(t *testing.T) {
	owner, poster, requestID, listingID := offerParties(t)

	existing := started(t, poster, listingID)
	if res := poster.do(t, http.MethodPost, "/conversations/"+existing.ID+"/close", nil); res.StatusCode != http.StatusOK {
		t.Fatalf("close: %d", res.StatusCode)
	}

	res := makeOffer(t, owner, requestID, listingID, "Please reconsider.")
	if res.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want 409", res.StatusCode)
	}
	env := envelope(t, res)
	if env.Code != apierror.CodeConflict || env.Details["conversation_id"] != existing.ID {
		t.Errorf("envelope = %+v", env)
	}

	// Nothing was written: no offer, no message, still closed.
	if rows := offersOn(t, poster, requestID); len(rows) != 0 {
		t.Errorf("a refused offer was recorded: %+v", rows)
	}
	if page := messagesIn(t, poster, existing.ID); page.Meta.Total != 1 {
		t.Errorf("messages = %d, want only the poster's 1", page.Meta.Total)
	}
	if c := getConversation(t, poster, existing.ID); c.Status != "closed" {
		t.Errorf("status = %s, want still closed", c.Status)
	}
	assertNothingRevealed(t, "refused offer", poster, existing.ID)
}

func TestTwoOffersShareOneConversation(t *testing.T) {
	owner, poster, firstRequest, listingID := offerParties(t)
	second := validRequest()
	second["title"] = "Or a spring sublet, if that is easier"
	_, request := postRequest(t, poster, second)
	secondRequest := request["id"].(string)

	a := offered(t, owner, firstRequest, listingID, "For your winter request.")
	b := offered(t, owner, secondRequest, listingID, "And for this one too.")
	if a.Conversation.ID != b.Conversation.ID {
		t.Fatalf("two offers of one listing to one student opened %s and %s", a.Conversation.ID, b.Conversation.ID)
	}
	if page := messagesIn(t, poster, a.Conversation.ID); page.Meta.Total != 2 {
		t.Errorf("messages = %d, want both notes", page.Meta.Total)
	}
	inbox := decode[conversationPageJSON](t, poster.do(t, http.MethodGet, "/me/conversations", nil))
	if inbox.Meta.Total != 1 {
		t.Errorf("the poster's inbox has %d conversations, want 1", inbox.Meta.Total)
	}

	// One reply answers both.
	if res := send(t, poster, a.Conversation.ID, "Either works for me."); res.StatusCode != http.StatusCreated {
		t.Fatalf("reply: %d", res.StatusCode)
	}
	for _, r := range []string{firstRequest, secondRequest} {
		if rows := offersOn(t, poster, r); len(rows) != 1 || !rows[0].Conversation.PosterReplied {
			t.Errorf("request %s offers = %+v", r, rows)
		}
	}
}

func TestWithdrawingAnOfferLeavesItsConversation(t *testing.T) {
	t.Run("not shared", func(t *testing.T) {
		owner, poster, requestID, listingID := offerParties(t)
		o := offered(t, owner, requestID, listingID, "Here you go.")

		if res := owner.do(t, http.MethodDelete, "/offers/"+o.ID, nil); res.StatusCode != http.StatusNoContent {
			t.Fatalf("withdraw: %d", res.StatusCode)
		}

		if rows := offersOn(t, poster, requestID); len(rows) != 0 {
			t.Errorf("a withdrawn offer was still listed: %+v", rows)
		}
		c := getConversation(t, poster, o.Conversation.ID)
		if c.Status != "open" || !c.CanSend {
			t.Errorf("withdrawal changed the conversation: %+v", c)
		}
		if page := messagesIn(t, poster, o.Conversation.ID); page.Meta.Total != 1 {
			t.Errorf("messages = %d, want the note kept", page.Meta.Total)
		}
		assertNothingRevealed(t, "after withdrawal", poster, o.Conversation.ID)
		if res := send(t, poster, o.Conversation.ID, "Oh - is it gone?"); res.StatusCode != http.StatusCreated {
			t.Errorf("reply after withdrawal: %d", res.StatusCode)
		}
	})

	t.Run("shared", func(t *testing.T) {
		owner, poster, requestID, listingID := offerParties(t)
		o := offered(t, owner, requestID, listingID, "Here you go.")
		if res := shareAddress(t, owner, o.Conversation.ID); res.StatusCode != http.StatusOK {
			t.Fatalf("share: %d", res.StatusCode)
		}

		if res := owner.do(t, http.MethodDelete, "/offers/"+o.ID, nil); res.StatusCode != http.StatusNoContent {
			t.Fatalf("withdraw: %d", res.StatusCode)
		}

		// The offer revealed nothing, so withdrawing it takes nothing back.
		after := raw(t, poster.do(t, http.MethodGet, "/conversations/"+o.Conversation.ID, nil), http.StatusOK)
		if place, sharedAt := sharedState(t, "shared, then withdrawn", after); place == nil || sharedAt == nil {
			t.Errorf("withdrawal un-shared the address: place = %v, address_shared_at = %v", place, sharedAt)
		}
	})
}

func TestOfferConversationsStayBetweenThePosterAndTheOwner(t *testing.T) {
	owner, poster, requestID, listingID := offerParties(t)
	o := offered(t, owner, requestID, listingID, "Here you go.")

	// A rival owner, with an offer of their own on the same request.
	rival := anotherBrowser(t, owner)
	register(t, rival, "rival@uwaterloo.ca", "rival")
	other := validListing()
	other["title"] = "A different room entirely"
	_, rivalListing := createListing(t, rival, other)
	r := offered(t, rival, requestID, rivalListing["id"].(string), "Mine is closer.")

	// Each owner sees only their own offer, and only their own conversation.
	if rows := offersOn(t, rival, requestID); len(rows) != 1 || rows[0].Conversation.ID != r.Conversation.ID {
		t.Errorf("the rival sees %+v", rows)
	}
	for _, path := range []string{"/conversations/" + o.Conversation.ID, "/conversations/" + o.Conversation.ID + "/messages"} {
		if res := rival.do(t, http.MethodGet, path, nil); res.StatusCode != http.StatusNotFound {
			t.Errorf("rival GET %s: %d, want 404", path, res.StatusCode)
		}
	}
	if res := owner.do(t, http.MethodGet, "/conversations/"+r.Conversation.ID, nil); res.StatusCode != http.StatusNotFound {
		t.Errorf("owner reading the rival's conversation: %d, want 404", res.StatusCode)
	}

	// The poster sees both offers, each with its own conversation.
	if rows := offersOn(t, poster, requestID); len(rows) != 2 || rows[0].Conversation.ID == rows[1].Conversation.ID {
		t.Errorf("the poster sees %+v", rows)
	}
}

func TestYouCannotOfferAgainstYourOwnRequest(t *testing.T) {
	owner, _, _, listingID := offerParties(t)
	_, own := postRequest(t, owner, validRequest())

	res := makeOffer(t, owner, own["id"].(string), listingID, "")
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", res.StatusCode)
	}
	if env := envelope(t, res); env.Code != apierror.CodeValidation {
		t.Errorf("code = %s", env.Code)
	}
}
