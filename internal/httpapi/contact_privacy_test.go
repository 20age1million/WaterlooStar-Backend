package httpapi_test

import (
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/20age1million/WaterlooStar-Backend/internal/ratelimit"
)

// Contact privacy and address sharing, through the handlers (Phase 16).
//
// Two guards here are regressions waiting to happen, so they read raw JSON
// rather than a struct a new field could slip past:
//
//   - No public read carries the exact address: no address_line key at all,
//     and null coordinates. Nor does any response carry an email or a phone.
//   - Only the owner's share reveals it. Starting, sending, reading and
//     closing never set address_shared_at or fill place.

const (
	exactAddress = "742 Evergreen Terrace"
	crossStreets = "King St N & University Ave"
)

// privateListing is validListing with an exact address and public cross
// streets that share no word with each other or the rest of the listing.
func privateListing() map[string]any {
	body := validListing()
	body["address_line"] = exactAddress
	body["cross_streets"] = crossStreets
	return body
}

// locate gives a listing coordinates in the fake. The contract has no way to
// write them yet, but they are part of the exact address and must already be
// hidden wherever it is.
func locate(t *testing.T, h *harness, listingID string) {
	t.Helper()
	id := uuid.MustParse(listingID)
	h.db.mu.Lock()
	defer h.db.mu.Unlock()
	for i, l := range h.db.listings {
		if l.ID == id {
			lat, lng := 43.4723, -80.5449
			h.db.listings[i].Lat, h.db.listings[i].Lng = &lat, &lng
			return
		}
	}
	t.Fatalf("no listing %s in the fake", listingID)
}

// raw decodes a response body as untyped JSON, so a guard sees every key.
func raw(t *testing.T, res *http.Response, want int) any {
	t.Helper()
	if res.StatusCode != want {
		t.Fatalf("%s %s: status %d, want %d", res.Request.Method, res.Request.URL.Path, res.StatusCode, want)
	}
	return decode[any](t, res)
}

// walk calls visit for every object in v, with a path for messages.
func walk(v any, path string, visit func(path string, obj map[string]any)) {
	switch x := v.(type) {
	case map[string]any:
		visit(path, x)
		for k, child := range x {
			walk(child, path+"."+k, visit)
		}
	case []any:
		for _, child := range x {
			walk(child, path+"[]", visit)
		}
	}
}

// assertNoContactDetails fails on any email or phone field anywhere in v.
func assertNoContactDetails(t *testing.T, label string, v any) {
	t.Helper()
	walk(v, label, func(path string, obj map[string]any) {
		for k := range obj {
			if k == "email" || strings.Contains(k, "phone") {
				t.Errorf("%s carries %q", path, k)
			}
		}
	})
}

// assertPublicShape fails if v carries the exact address anywhere: an
// address_line key, a non-null coordinate, a non-null place, or the address's
// text in any value. A public read must not have the key at all.
func assertPublicShape(t *testing.T, label string, v any) {
	t.Helper()
	assertNoContactDetails(t, label, v)
	walk(v, label, func(path string, obj map[string]any) {
		if _, ok := obj["address_line"]; ok {
			t.Errorf("%s carries address_line", path)
		}
		for _, k := range []string{"lat", "lng", "place"} {
			if val, ok := obj[k]; ok && val != nil {
				t.Errorf("%s.%s = %v, want null", path, k, val)
			}
		}
		for k, val := range obj {
			if s, ok := val.(string); ok && strings.Contains(s, "Evergreen") {
				t.Errorf("%s.%s contains the exact address: %q", path, k, s)
			}
		}
	})
}

// assertExact fails unless listing (a decoded Listing) carries the exact
// address and its coordinates.
func assertExact(t *testing.T, label string, listing any) {
	t.Helper()
	obj, ok := listing.(map[string]any)
	if !ok {
		t.Fatalf("%s is not an object: %v", label, listing)
	}
	if obj["address_line"] != exactAddress {
		t.Errorf("%s.address_line = %v, want the exact address", label, obj["address_line"])
	}
	if obj["lat"] == nil || obj["lng"] == nil {
		t.Errorf("%s coordinates = %v, %v, want them", label, obj["lat"], obj["lng"])
	}
	if obj["cross_streets"] != crossStreets {
		t.Errorf("%s.cross_streets = %v", label, obj["cross_streets"])
	}
	assertNoContactDetails(t, label, listing)
}

// privacyParties is an owner with a located listing and a verified enquirer,
// each in their own browser.
func privacyParties(t *testing.T) (owner, enquirer *harness, listingID string) {
	t.Helper()
	owner = newHarness(t)
	register(t, owner, "owner@uwaterloo.ca", "owner")
	res, listing := createListing(t, owner, privateListing())
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create listing: %d", res.StatusCode)
	}
	listingID = listing["id"].(string)
	locate(t, owner, listingID)
	enquirer = anotherBrowser(t, owner)
	register(t, enquirer, "enquirer@uwaterloo.ca", "enquirer")
	return owner, enquirer, listingID
}

func shareAddress(t *testing.T, h *harness, conversationID string) *http.Response {
	t.Helper()
	return h.do(t, http.MethodPost, "/conversations/"+conversationID+"/share-address", nil)
}

// sharedState reads a conversation as JSON and reports its place and
// address_shared_at, after checking it carries no contact details.
func sharedState(t *testing.T, label string, conversation any) (place map[string]any, sharedAt any) {
	t.Helper()
	assertNoContactDetails(t, label, conversation)
	obj := conversation.(map[string]any)
	for _, k := range []string{"place", "address_shared_at", "can_share_address"} {
		if _, ok := obj[k]; !ok {
			t.Fatalf("%s has no %q key", label, k)
		}
	}
	if p, ok := obj["place"].(map[string]any); ok {
		place = p
	}
	return place, obj["address_shared_at"]
}

// ------------------------------------------------------------------- guards

func TestNoPublicReadCarriesTheExactAddress(t *testing.T) {
	owner, enquirer, listingID := privacyParties(t)
	anonymous := anotherBrowser(t, owner)

	// The browse and the detail, signed out and signed in as someone else.
	for _, h := range []*harness{anonymous, enquirer} {
		page := raw(t, h.do(t, http.MethodGet, "/listings", nil), http.StatusOK)
		assertPublicShape(t, "GET /listings", page)
		data := page.(map[string]any)["data"].([]any)
		if len(data) != 1 || data[0].(map[string]any)["cross_streets"] != crossStreets {
			t.Errorf("the browse lost the public cross streets: %v", data)
		}
		detail := raw(t, h.do(t, http.MethodGet, "/listings/"+listingID, nil), http.StatusOK)
		assertPublicShape(t, "GET /listings/{id}", detail)
		if detail.(map[string]any)["cross_streets"] != crossStreets {
			t.Errorf("the detail lost the public cross streets")
		}
	}

	// Search cannot find it by its address, and can by its cross streets.
	for q, want := range map[string]int{"Evergreen": 0, "Terrace": 0, "University": 1} {
		page := raw(t, anonymous.do(t, http.MethodGet, "/listings?q="+q, nil), http.StatusOK)
		if got := len(page.(map[string]any)["data"].([]any)); got != want {
			t.Errorf("q=%s matched %d, want %d", q, got, want)
		}
	}

	// An offer: the create response and both sides' offer lists. The request's
	// poster is the enquirer.
	_, request := postRequest(t, enquirer, validRequest())
	requestID := request["id"].(string)
	offer := raw(t, owner.do(t, http.MethodPost, "/requests/"+requestID+"/offers",
		map[string]any{"listing_id": listingID, "note": "Near the park."}), http.StatusCreated)
	assertPublicShape(t, "POST /requests/{id}/offers", offer)
	for _, h := range []*harness{owner, enquirer} {
		assertPublicShape(t, "GET /requests/{id}/offers",
			raw(t, h.do(t, http.MethodGet, "/requests/"+requestID+"/offers", nil), http.StatusOK))
	}

	// A conversation before the owner shares: the start, the thread, both
	// inboxes, the messages and the unread counts.
	started := raw(t, enquirer.do(t, http.MethodPost, "/listings/"+listingID+"/conversations",
		map[string]string{"body": "Is it still free?"}), http.StatusCreated)
	assertPublicShape(t, "POST /listings/{id}/conversations", started)
	id := started.(map[string]any)["id"].(string)
	for _, h := range []*harness{owner, enquirer} {
		assertPublicShape(t, "GET /conversations/{id}", raw(t, h.do(t, http.MethodGet, "/conversations/"+id, nil), http.StatusOK))
		assertPublicShape(t, "GET /me/conversations", raw(t, h.do(t, http.MethodGet, "/me/conversations", nil), http.StatusOK))
		assertPublicShape(t, "GET /conversations/{id}/messages", raw(t, h.do(t, http.MethodGet, "/conversations/"+id+"/messages", nil), http.StatusOK))
		assertPublicShape(t, "GET /me/conversations/unread", raw(t, h.do(t, http.MethodGet, "/me/conversations/unread", nil), http.StatusOK))
	}
	// The signed-in user's own record carries their own email and nothing
	// else's; it is the one place an email is meant to appear.
	me := raw(t, enquirer.do(t, http.MethodGet, "/me", nil), http.StatusOK).(map[string]any)
	if me["email"] != "enquirer@uwaterloo.ca" {
		t.Errorf("/me email = %v", me["email"])
	}
}

func TestOwnerAndAdminReadsCarryTheExactAddress(t *testing.T) {
	owner, _, listingID := privacyParties(t)

	mine := raw(t, owner.do(t, http.MethodGet, "/me/listings", nil), http.StatusOK).([]any)
	if len(mine) != 1 {
		t.Fatalf("/me/listings = %d listings", len(mine))
	}
	assertExact(t, "GET /me/listings", mine[0])

	// The owner's own writes answer with the full listing too.
	patched := raw(t, owner.do(t, http.MethodPatch, "/listings/"+listingID,
		map[string]any{"title": "Bright room by the park"}), http.StatusOK)
	assertExact(t, "PATCH /listings/{id}", patched)
	paused := raw(t, owner.do(t, http.MethodPost, "/listings/"+listingID+"/status",
		map[string]string{"status": "paused"}), http.StatusOK)
	assertExact(t, "POST /listings/{id}/status", paused)
	created := raw(t, owner.do(t, http.MethodPost, "/listings", privateListing()), http.StatusCreated)
	if created.(map[string]any)["address_line"] != exactAddress {
		t.Errorf("the create response lost the exact address")
	}

	// The admin surface.
	admin, _ := adminBrowser(t, owner)
	page := raw(t, admin.do(t, http.MethodGet, "/admin/listings", nil), http.StatusOK)
	found := false
	for _, row := range page.(map[string]any)["data"].([]any) {
		l := row.(map[string]any)["listing"].(map[string]any)
		if l["id"] == listingID {
			found = true
			assertExact(t, "GET /admin/listings", l)
		}
	}
	if !found {
		t.Fatal("the admin list does not show the listing")
	}
	// The single admin read answers a takedown and a restore.
	removed := raw(t, admin.do(t, http.MethodPost, "/admin/listings/"+listingID+"/remove",
		map[string]string{"reason": takedownReason}), http.StatusOK)
	assertExact(t, "POST /admin/listings/{id}/remove", removed.(map[string]any)["listing"])
	restored := raw(t, admin.do(t, http.MethodPost, "/admin/listings/"+listingID+"/restore", nil), http.StatusOK)
	assertExact(t, "POST /admin/listings/{id}/restore", restored.(map[string]any)["listing"])
}

func TestOnlyTheOwnersShareRevealsThePlace(t *testing.T) {
	owner, enquirer, listingID := privacyParties(t)
	stranger := anotherBrowser(t, owner)
	register(t, stranger, "stranger@uwaterloo.ca", "stranger")

	// The share-only guard: every other conversation action, by both sides,
	// leaves address_shared_at unset and place null.
	c := started(t, enquirer, listingID)
	steps := []struct {
		label string
		h     *harness
		do    func() *http.Response
		want  int
	}{
		{"enquirer sends", enquirer, func() *http.Response { return send(t, enquirer, c.ID, "When can I see it?") }, http.StatusCreated},
		{"owner replies", owner, func() *http.Response { return send(t, owner, c.ID, "Saturday works.") }, http.StatusCreated},
		{"owner marks read", owner, func() *http.Response { return owner.do(t, http.MethodPost, "/conversations/"+c.ID+"/read", nil) }, http.StatusNoContent},
		{"enquirer marks read", enquirer, func() *http.Response {
			return enquirer.do(t, http.MethodPost, "/conversations/"+c.ID+"/read", nil)
		}, http.StatusNoContent},
	}
	for _, step := range steps {
		if res := step.do(); res.StatusCode != step.want {
			t.Fatalf("%s: status %d, want %d", step.label, res.StatusCode, step.want)
		}
		for _, h := range []*harness{owner, enquirer} {
			place, at := sharedState(t, step.label, raw(t, h.do(t, http.MethodGet, "/conversations/"+c.ID, nil), http.StatusOK))
			if place != nil || at != nil {
				t.Fatalf("after %q: place %v, address_shared_at %v", step.label, place, at)
			}
		}
	}

	// Who may share: the owner, and the conversation says so.
	if v := raw(t, owner.do(t, http.MethodGet, "/conversations/"+c.ID, nil), http.StatusOK).(map[string]any); v["can_share_address"] != true {
		t.Error("the owner's can_share_address is false before sharing")
	}
	if v := raw(t, enquirer.do(t, http.MethodGet, "/conversations/"+c.ID, nil), http.StatusOK).(map[string]any); v["can_share_address"] != false {
		t.Error("the enquirer's can_share_address is true")
	}
	if res := shareAddress(t, enquirer, c.ID); res.StatusCode != http.StatusForbidden {
		t.Errorf("the enquirer sharing: status %d, want 403", res.StatusCode)
	} else if env := envelope(t, res); env.Message != "Only the owner can share the address of their place." {
		t.Errorf("the enquirer's refusal says %q", env.Message)
	}
	if res := shareAddress(t, stranger, c.ID); res.StatusCode != http.StatusNotFound {
		t.Errorf("a stranger sharing: status %d, want 404", res.StatusCode)
	}
	if res := shareAddress(t, anotherBrowser(t, owner), c.ID); res.StatusCode != http.StatusUnauthorized {
		t.Errorf("an anonymous share: status %d, want 401", res.StatusCode)
	}
	if place, at := sharedState(t, "after refused shares", raw(t, enquirer.do(t, http.MethodGet, "/conversations/"+c.ID, nil), http.StatusOK)); place != nil || at != nil {
		t.Fatal("a refused share revealed the place")
	}

	// The owner shares: the place appears, for both participants.
	shared := raw(t, shareAddress(t, owner, c.ID), http.StatusOK)
	place, at := sharedState(t, "the share response", shared)
	if place == nil || place["address_line"] != exactAddress || place["lat"] == nil || place["lng"] == nil || at == nil {
		t.Fatalf("the share response: place %v, address_shared_at %v", place, at)
	}
	if shared.(map[string]any)["can_share_address"] != false {
		t.Error("can_share_address is still true after sharing")
	}
	for _, path := range []string{"/conversations/" + c.ID, "/me/conversations"} {
		got := raw(t, enquirer.do(t, http.MethodGet, path, nil), http.StatusOK)
		if obj, ok := got.(map[string]any); ok && obj["data"] != nil {
			got = obj["data"].([]any)[0]
		}
		if place, _ := sharedState(t, "the enquirer's "+path, got); place == nil || place["address_line"] != exactAddress {
			t.Errorf("the enquirer's %s: place %v", path, place)
		}
	}

	// Sharing again changes nothing.
	again := raw(t, shareAddress(t, owner, c.ID), http.StatusOK)
	if _, at2 := sharedState(t, "a second share", again); at2 != at {
		t.Errorf("a second share moved address_shared_at from %v to %v", at, at2)
	}

	// The public reads still do not carry it.
	assertPublicShape(t, "GET /listings/{id} after a share",
		raw(t, enquirer.do(t, http.MethodGet, "/listings/"+listingID, nil), http.StatusOK))

	// One conversation only: the same enquirer about another of the owner's
	// listings sees nothing.
	res, other := createListing(t, owner, privateListing())
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("second listing: %d", res.StatusCode)
	}
	elsewhere := started(t, enquirer, other["id"].(string))
	if place, at := sharedState(t, "another conversation", raw(t, enquirer.do(t, http.MethodGet, "/conversations/"+elsewhere.ID, nil), http.StatusOK)); place != nil || at != nil {
		t.Errorf("the share leaked into another conversation: %v", place)
	}

	// Closing un-shares nothing.
	if res := enquirer.do(t, http.MethodPost, "/conversations/"+elsewhere.ID+"/close", nil); res.StatusCode != http.StatusOK {
		t.Fatalf("close: %d", res.StatusCode)
	}
	if place, at := sharedState(t, "a closed, unshared conversation", raw(t, owner.do(t, http.MethodGet, "/conversations/"+elsewhere.ID, nil), http.StatusOK)); place != nil || at != nil {
		t.Error("closing revealed the place")
	}
}

func TestModerationWithholdsASharedPlaceWithoutUnsharingIt(t *testing.T) {
	owner, enquirer, listingID := privacyParties(t)
	c := started(t, enquirer, listingID)
	raw(t, shareAddress(t, owner, c.ID), http.StatusOK)
	admin, _ := adminBrowser(t, owner)

	placeFor := func(label string) (map[string]any, any) {
		t.Helper()
		return sharedState(t, label, raw(t, enquirer.do(t, http.MethodGet, "/conversations/"+c.ID, nil), http.StatusOK))
	}

	if res := admin.do(t, http.MethodPost, "/admin/listings/"+listingID+"/remove", map[string]string{"reason": takedownReason}); res.StatusCode != http.StatusOK {
		t.Fatalf("take down: %d", res.StatusCode)
	}
	if place, at := placeFor("taken down"); place != nil || at == nil {
		t.Errorf("taken down: place %v, address_shared_at %v; want withheld, still shared", place, at)
	}
	if res := admin.do(t, http.MethodPost, "/admin/listings/"+listingID+"/restore", nil); res.StatusCode != http.StatusOK {
		t.Fatalf("restore: %d", res.StatusCode)
	}
	if place, _ := placeFor("restored"); place == nil {
		t.Error("restoring the listing did not restore the place")
	}

	suspend(t, owner, "owner@uwaterloo.ca")
	if place, at := placeFor("owner suspended"); place != nil || at == nil {
		t.Errorf("owner suspended: place %v, address_shared_at %v", place, at)
	}
	reinstate(t, owner, "owner@uwaterloo.ca")
	if place, _ := placeFor("reinstated"); place == nil {
		t.Error("reinstating the owner did not restore the place")
	}
}

func TestShareAddressRefusals(t *testing.T) {
	refuse := func(t *testing.T, res *http.Response, want int, message string) {
		t.Helper()
		if res.StatusCode != want {
			t.Fatalf("status %d, want %d", res.StatusCode, want)
		}
		if env := envelope(t, res); env.Message != message {
			t.Errorf("message %q, want %q", env.Message, message)
		}
	}

	t.Run("closed", func(t *testing.T) {
		owner, enquirer, listingID := privacyParties(t)
		c := started(t, enquirer, listingID)
		enquirer.do(t, http.MethodPost, "/conversations/"+c.ID+"/close", nil)
		if v := getConversation(t, owner, c.ID); v.Status != "closed" {
			t.Fatal("not closed")
		}
		refuse(t, shareAddress(t, owner, c.ID), http.StatusConflict, "This conversation was closed.")
	})

	t.Run("listing paused", func(t *testing.T) {
		owner, enquirer, listingID := privacyParties(t)
		c := started(t, enquirer, listingID)
		owner.do(t, http.MethodPost, "/listings/"+listingID+"/status", map[string]string{"status": "paused"})
		if v := raw(t, owner.do(t, http.MethodGet, "/conversations/"+c.ID, nil), http.StatusOK).(map[string]any); v["can_share_address"] != false {
			t.Error("can_share_address is true on a paused listing")
		}
		refuse(t, shareAddress(t, owner, c.ID), http.StatusConflict, "This listing is not available, so its address cannot be shared.")
	})

	t.Run("enquirer suspended", func(t *testing.T) {
		owner, enquirer, listingID := privacyParties(t)
		c := started(t, enquirer, listingID)
		suspend(t, owner, "enquirer@uwaterloo.ca")
		refuse(t, shareAddress(t, owner, c.ID), http.StatusConflict, "This account is no longer available.")
	})

	t.Run("no address", func(t *testing.T) {
		owner, enquirer, listingID := privacyParties(t)
		c := started(t, enquirer, listingID)
		id := uuid.MustParse(listingID)
		owner.db.mu.Lock()
		for i, l := range owner.db.listings {
			if l.ID == id {
				owner.db.listings[i].AddressLine = ""
			}
		}
		owner.db.mu.Unlock()
		refuse(t, shareAddress(t, owner, c.ID), http.StatusConflict, "Add the exact address to your listing before sharing it.")
	})

	// Nothing refused was shared.
	t.Run("a refusal shares nothing", func(t *testing.T) {
		owner, enquirer, listingID := privacyParties(t)
		c := started(t, enquirer, listingID)
		suspend(t, owner, "enquirer@uwaterloo.ca")
		shareAddress(t, owner, c.ID)
		reinstate(t, owner, "enquirer@uwaterloo.ca")
		for _, conv := range owner.db.conversations {
			if conv.AddressSharedAt != nil {
				t.Fatal("a refused share set address_shared_at")
			}
		}
	})
}

func TestCrossStreetsAreValidatedAndEditable(t *testing.T) {
	owner, _, listingID := privacyParties(t)

	long := privateListing()
	long["cross_streets"] = strings.Repeat("x", 81)
	res, body := createListing(t, owner, long)
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("81 characters of cross streets: status %d, want 400", res.StatusCode)
	}
	if details, _ := body["details"].(map[string]any); details["cross_streets"] == nil {
		t.Errorf("the refusal does not name cross_streets: %v", body)
	}

	edited := raw(t, owner.do(t, http.MethodPatch, "/listings/"+listingID,
		map[string]any{"cross_streets": "  Columbia St & Phillip St  "}), http.StatusOK).(map[string]any)
	if edited["cross_streets"] != "Columbia St & Phillip St" {
		t.Errorf("edited cross streets = %v, want trimmed", edited["cross_streets"])
	}
	cleared := raw(t, owner.do(t, http.MethodPatch, "/listings/"+listingID,
		map[string]any{"cross_streets": ""}), http.StatusOK).(map[string]any)
	if cleared["cross_streets"] != "" || cleared["address_line"] != exactAddress {
		t.Errorf("after clearing: cross_streets %v, address_line %v", cleared["cross_streets"], cleared["address_line"])
	}
}

func TestSharingAnAddressIsThrottledPerUser(t *testing.T) {
	h, clk := limitedHarness(t)
	owner, enquirer, listingID := conversationParties(t, h)
	c := started(t, enquirer, listingID)

	// Sharing again is a no-op that still costs.
	for i := range ratelimit.ShareAddressPerUser.Burst {
		if res := shareAddress(t, owner, c.ID); res.StatusCode != http.StatusOK {
			t.Fatalf("share %d: status %d, want 200", i+1, res.StatusCode)
		}
	}
	assertRefusal(t, shareAddress(t, owner, c.ID))

	clk.advance(ratelimit.ShareAddressPerUser.Window + time.Second)
	if res := shareAddress(t, owner, c.ID); res.StatusCode != http.StatusOK {
		t.Fatalf("after the window: status %d, want 200", res.StatusCode)
	}
}

// The guard on the write itself: in every generated query, only
// ShareConversationAddress assigns address_shared_at.
func TestOnlyTheShareQuerySetsAddressSharedAt(t *testing.T) {
	writes := sqlcWritesTo(t, "address_shared_at")
	if len(writes) != 1 || writes[0] != "ShareConversationAddress" {
		t.Errorf("queries that set address_shared_at: %v, want only ShareConversationAddress", writes)
	}
}

// sqlcWritesTo names every query in internal/db/queries that writes column:
// assigns it in a SET, or names it in an INSERT's column list.
func sqlcWritesTo(t *testing.T, column string) []string {
	t.Helper()
	dir := filepath.Join("..", "db", "queries")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	name := regexp.MustCompile(`(?m)^-- name: (\w+)`)
	assign := regexp.MustCompile(`(?i)\b` + column + `\s*=[^=]`)
	insert := regexp.MustCompile(`(?is)\binsert\s+into\s+\w+\s*\([^)]*\b` + column + `\b`)
	var writes []string
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		src, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		text := string(src)
		bounds := name.FindAllStringSubmatchIndex(text, -1)
		for i, b := range bounds {
			end := len(text)
			if i+1 < len(bounds) {
				end = bounds[i+1][0]
			}
			// Comments say what may set the column; only the SQL counts.
			var body []string
			for _, line := range strings.Split(text[b[1]:end], "\n") {
				if !strings.HasPrefix(strings.TrimSpace(line), "--") {
					body = append(body, line)
				}
			}
			query := strings.Join(body, "\n")
			if assign.MatchString(query) || insert.MatchString(query) {
				writes = append(writes, text[b[2]:b[3]])
			}
		}
	}
	return writes
}
