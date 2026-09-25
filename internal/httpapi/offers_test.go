package httpapi_test

import (
	"net/http"
	"testing"
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
