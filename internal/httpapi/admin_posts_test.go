package httpapi_test

import (
	"net/http"
	"strings"
	"testing"
)

// Post moderation over HTTP: the admin's view of every post, takedown and
// restore, and what a takedown does to the owner and to everyone else.

type adminListingPage struct {
	Data []struct {
		Listing struct {
			ID      string `json:"id"`
			Status  string `json:"status"`
			Removal *struct {
				Reason string `json:"reason"`
			} `json:"removal"`
		} `json:"listing"`
		Owner struct {
			Email string `json:"email"`
		} `json:"owner"`
	} `json:"data"`
	Meta struct {
		Total int `json:"total"`
	} `json:"meta"`
}

const takedownReason = "Address belongs to someone else's house"

func TestTakingAListingDownAndRestoringIt(t *testing.T) {
	// owner@ holds a published listing, offered against poster@'s request.
	h, requestID, listingID := twoAccounts(t)
	if res := h.do(t, http.MethodPost, "/requests/"+requestID+"/offers",
		map[string]any{"listing_id": listingID}); res.StatusCode != http.StatusCreated {
		t.Fatalf("offer: %d", res.StatusCode)
	}
	a, _ := adminBrowser(t, h)

	visible := func() (listed int, detail int, offers int) {
		t.Helper()
		_, lp := getJSON[listingPageBody](t, h.server.URL+"/listings")
		res, _ := http.Get(h.server.URL + "/listings/" + listingID)
		res.Body.Close()
		_, rp := getJSON[requestPageBody](t, h.server.URL+"/requests")
		return lp.Meta.Total, res.StatusCode, rp.Data[0].Offers
	}

	res := a.do(t, http.MethodPost, "/admin/listings/"+listingID+"/remove", map[string]string{"reason": takedownReason})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("remove = %d", res.StatusCode)
	}
	body := decode[struct {
		Listing struct {
			Status  string `json:"status"`
			Removal *struct {
				Reason string `json:"reason"`
			} `json:"removal"`
		} `json:"listing"`
	}](t, res)
	if body.Listing.Removal == nil || body.Listing.Removal.Reason != takedownReason || body.Listing.Status != "published" {
		t.Errorf("remove response = %+v: the status is the owner's and must be untouched", body.Listing)
	}

	if listed, detail, offers := visible(); listed != 0 || detail != http.StatusNotFound || offers != 0 {
		t.Errorf("while removed: %d listed, detail %d, %d offers — want it gone everywhere", listed, detail, offers)
	}

	// The owner still sees it, with the reason.
	mine := decode[[]struct {
		Removal *struct {
			Reason string `json:"reason"`
		} `json:"removal"`
	}](t, h.do(t, http.MethodGet, "/me/listings", nil))
	if len(mine) != 1 || mine[0].Removal == nil || mine[0].Removal.Reason != takedownReason {
		t.Errorf("the owner's view = %+v, want the listing with its reason", mine)
	}

	// And cannot override the moderator.
	for _, try := range []struct {
		method, path string
		body         any
	}{
		{http.MethodPost, "/listings/" + listingID + "/status", map[string]string{"status": "published"}},
		{http.MethodPatch, "/listings/" + listingID, map[string]any{"title": "Nothing to see here"}},
		{http.MethodDelete, "/listings/" + listingID, nil},
	} {
		res := h.do(t, try.method, try.path, try.body)
		if res.StatusCode != http.StatusConflict {
			t.Errorf("%s %s = %d, want 409", try.method, try.path, res.StatusCode)
			continue
		}
		if msg := decode[errorEnvelope](t, res).Message; !strings.Contains(msg, takedownReason) {
			t.Errorf("%s refusal %q does not explain itself", try.method, msg)
		}
	}

	if res := a.do(t, http.MethodPost, "/admin/listings/"+listingID+"/remove", map[string]string{"reason": "A second reason entirely"}); res.StatusCode != http.StatusConflict {
		t.Errorf("second takedown = %d, want 409", res.StatusCode)
	}

	// Restoring needs no reason.
	if res := a.do(t, http.MethodPost, "/admin/listings/"+listingID+"/restore", nil); res.StatusCode != http.StatusOK {
		t.Fatalf("restore = %d", res.StatusCode)
	}
	if listed, detail, offers := visible(); listed != 1 || detail != http.StatusOK || offers != 1 {
		t.Errorf("after restoring: %d listed, detail %d, %d offers — want everything back", listed, detail, offers)
	}
	if res := a.do(t, http.MethodPost, "/admin/listings/"+listingID+"/restore", nil); res.StatusCode != http.StatusConflict {
		t.Errorf("restoring a visible listing = %d, want 409", res.StatusCode)
	}

	ledger := decode[struct {
		Data []ledgerEntry `json:"data"`
	}](t, a.do(t, http.MethodGet, "/admin/actions", nil)).Data
	if len(ledger) < 2 || ledger[0].Action != "restore_post" || ledger[1].Action != "remove_post" {
		t.Fatalf("ledger = %+v", ledger)
	}
	if ledger[1].Reason != takedownReason || ledger[1].Detail["status"] != "published" || ledger[1].SubjectType != "listing" {
		t.Errorf("remove entry = %+v", ledger[1])
	}
	if ledger[0].Reason == "" || ledger[0].Detail["removed_reason"] != takedownReason {
		t.Errorf("restore entry = %+v", ledger[0])
	}
}

func TestTakingARequestDown(t *testing.T) {
	h, requestID, _ := twoAccounts(t)
	a, _ := adminBrowser(t, h)

	if res := a.do(t, http.MethodPost, "/admin/requests/"+requestID+"/remove", map[string]string{"reason": "Contains a phone number in the text"}); res.StatusCode != http.StatusOK {
		t.Fatalf("remove = %d", res.StatusCode)
	}
	if _, page := getJSON[requestPageBody](t, h.server.URL+"/requests"); page.Meta.Total != 0 {
		t.Errorf("public requests = %d, want 0", page.Meta.Total)
	}

	login(t, h, "poster@uwaterloo.ca")
	mine := decode[[]struct {
		Removal *struct{ Reason string } `json:"removal"`
	}](t, h.do(t, http.MethodGet, "/me/requests", nil))
	if len(mine) != 1 || mine[0].Removal == nil {
		t.Errorf("the poster's view = %+v", mine)
	}
	if res := h.do(t, http.MethodPost, "/requests/"+requestID+"/status", map[string]string{"status": "published"}); res.StatusCode != http.StatusConflict {
		t.Errorf("republishing a removed request = %d, want 409", res.StatusCode)
	}

	if res := a.do(t, http.MethodPost, "/admin/requests/"+requestID+"/restore",
		map[string]string{"reason": "The number was the poster's own"}); res.StatusCode != http.StatusOK {
		t.Fatalf("restore = %d", res.StatusCode)
	}
	if _, page := getJSON[requestPageBody](t, h.server.URL+"/requests"); page.Meta.Total != 1 {
		t.Errorf("after restoring, public requests = %d, want 1", page.Meta.Total)
	}
}

func TestAdminSeesEveryListing(t *testing.T) {
	h, _ := signedIn(t, true)
	_, first := createListing(t, h, validListing())
	_, second := createListing(t, h, validListing())
	pausedID := second["id"].(string)
	if res := h.do(t, http.MethodPost, "/listings/"+pausedID+"/status", map[string]string{"status": "paused"}); res.StatusCode != http.StatusOK {
		t.Fatalf("pause: %d", res.StatusCode)
	}
	a, _ := adminBrowser(t, h)
	if res := a.do(t, http.MethodPost, "/admin/listings/"+first["id"].(string)+"/remove", map[string]string{"reason": takedownReason}); res.StatusCode != http.StatusOK {
		t.Fatalf("remove: %d", res.StatusCode)
	}
	owner := userID(t, h, "poster@uwaterloo.ca")

	cases := map[string]int{
		"":                   2, // the public sees neither
		"?status=paused":     1,
		"?removed=true":      1,
		"?removed=false":     1,
		"?owner_id=" + owner: 2,
		"?owner_id=3f7c1b2a-9d4e-4c8a-8f1b-2e3d4c5b6a70": 0,
	}
	for query, want := range cases {
		res := a.do(t, http.MethodGet, "/admin/listings"+query, nil)
		if res.StatusCode != http.StatusOK {
			t.Fatalf("GET /admin/listings%s = %d", query, res.StatusCode)
		}
		page := decode[adminListingPage](t, res)
		if page.Meta.Total != want || len(page.Data) != want {
			t.Errorf("GET /admin/listings%s = %d, want %d", query, page.Meta.Total, want)
		}
		for _, row := range page.Data {
			if row.Owner.Email != "poster@uwaterloo.ca" {
				t.Errorf("owner email = %q", row.Owner.Email)
			}
		}
	}
	if _, page := getJSON[listingPageBody](t, h.server.URL+"/listings"); page.Meta.Total != 0 {
		t.Errorf("public listings = %d, want 0", page.Meta.Total)
	}
}

func TestTakedownRefusals(t *testing.T) {
	h, _ := signedIn(t, true)
	_, l := createListing(t, h, validListing())
	id := l["id"].(string)
	a, _ := adminBrowser(t, h)

	for name, tc := range map[string]struct {
		path   string
		body   any
		status int
	}{
		"short reason":  {"/admin/listings/" + id + "/remove", map[string]string{"reason": "spam"}, http.StatusBadRequest},
		"no body":       {"/admin/listings/" + id + "/remove", nil, http.StatusBadRequest},
		"no such thing": {"/admin/listings/3f7c1b2a-9d4e-4c8a-8f1b-2e3d4c5b6a70/remove", map[string]string{"reason": takedownReason}, http.StatusNotFound},
		"not removed":   {"/admin/requests/3f7c1b2a-9d4e-4c8a-8f1b-2e3d4c5b6a70/restore", nil, http.StatusNotFound},
	} {
		if res := a.do(t, http.MethodPost, tc.path, tc.body); res.StatusCode != tc.status {
			t.Errorf("%s: %d, want %d", name, res.StatusCode, tc.status)
		}
	}

	// Ordinary users and anonymous callers find nothing there.
	anonymous := anotherBrowser(t, h)
	for _, who := range []*harness{h, anonymous} {
		for _, req := range []struct{ method, path string }{
			{http.MethodGet, "/admin/listings"}, {http.MethodGet, "/admin/requests"},
			{http.MethodPost, "/admin/listings/" + id + "/remove"}, {http.MethodPost, "/admin/listings/" + id + "/restore"},
		} {
			res := who.do(t, req.method, req.path, map[string]string{"reason": takedownReason})
			if res.StatusCode != http.StatusNotFound || decode[errorEnvelope](t, res).Message != "No endpoint at "+req.path {
				t.Errorf("%s %s by a non-admin = %d", req.method, req.path, res.StatusCode)
			}
		}
	}
	if _, page := getJSON[listingPageBody](t, h.server.URL+"/listings"); page.Meta.Total != 1 {
		t.Error("a refused takedown took the listing down")
	}
}
