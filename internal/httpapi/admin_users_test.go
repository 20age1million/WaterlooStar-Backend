package httpapi_test

import (
	"context"
	"net/http"
	"net/http/cookiejar"
	"testing"
)

// Account management over HTTP: suspend, reinstate, verify, change role, and
// what suspension does to the rest of the API.

const goodReason = "Reported for posting a scam listing"

// anotherBrowser is a second client on the same server and data, with its own
// cookies — a second person signed in at the same time.
func anotherBrowser(t *testing.T, h *harness) *harness {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar: %v", err)
	}
	return &harness{server: h.server, db: h.db, mail: h.mail, client: &http.Client{Jar: jar}}
}

// adminBrowser registers admin@uwaterloo.ca in its own browser and signs it in
// as an admin.
func adminBrowser(t *testing.T, h *harness) (*harness, string) {
	t.Helper()
	a := anotherBrowser(t, h)
	register(t, a, "admin@uwaterloo.ca", "operator")
	id := promoteAndRelogin(t, a, "admin@uwaterloo.ca")
	return a, id
}

func userID(t *testing.T, h *harness, email string) string {
	t.Helper()
	u, err := h.db.GetUserByEmail(context.Background(), email)
	if err != nil {
		t.Fatalf("find %s: %v", email, err)
	}
	return u.ID.String()
}

func ledgerFor(t *testing.T, a *harness, id string) []ledgerEntry {
	t.Helper()
	res := a.do(t, http.MethodGet, "/admin/users/"+id, nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /admin/users/%s = %d", id, res.StatusCode)
	}
	return decode[struct {
		Actions []ledgerEntry `json:"actions"`
	}](t, res).Actions
}

func TestSuspensionHidesEverythingAndReinstatingRestoresIt(t *testing.T) {
	// poster@ has a request; owner@ has a listing offered against it.
	h, requestID, listingID := twoAccounts(t)
	if res := h.do(t, http.MethodPost, "/requests/"+requestID+"/offers",
		map[string]any{"listing_id": listingID}); res.StatusCode != http.StatusCreated {
		t.Fatalf("offer: %d", res.StatusCode)
	}
	owner := userID(t, h, "owner@uwaterloo.ca")
	a, _ := adminBrowser(t, h)

	publicCounts := func() (listings, offers int, listingStatus int) {
		t.Helper()
		_, lp := getJSON[listingPageBody](t, h.server.URL+"/listings")
		_, rp := getJSON[requestPageBody](t, h.server.URL+"/requests")
		res, _ := http.Get(h.server.URL + "/listings/" + listingID)
		res.Body.Close()
		return lp.Meta.Total, rp.Data[0].Offers, res.StatusCode
	}
	if l, o, st := publicCounts(); l != 1 || o != 1 || st != http.StatusOK {
		t.Fatalf("before: %d listings, %d offers, listing %d", l, o, st)
	}

	// h is still signed in as owner@: their session is about to be ended.
	res := a.do(t, http.MethodPost, "/admin/users/"+owner+"/suspend", map[string]string{"reason": goodReason})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("suspend = %d", res.StatusCode)
	}
	if body := decode[adminUserBody](t, res); body.SuspendedAt == nil {
		t.Error("the response does not show the suspension")
	}

	if l, o, st := publicCounts(); l != 0 || o != 0 || st != http.StatusNotFound {
		t.Errorf("while suspended: %d listings, %d offers, listing %d — want all hidden", l, o, st)
	}

	// The owner still sees their own posts with the access token they hold:
	// suspension hides content from the public, not from its author.
	if res := h.do(t, http.MethodGet, "/me/listings", nil); res.StatusCode != http.StatusOK ||
		len(decode[[]map[string]any](t, res)) != 1 {
		t.Error("the suspended owner cannot see their own listing")
	}

	// Their refresh token was revoked, and they cannot sign back in.
	if res := h.do(t, http.MethodPost, "/auth/refresh", nil); res.StatusCode != http.StatusUnauthorized {
		t.Errorf("refresh while suspended = %d, want 401", res.StatusCode)
	}
	res = h.do(t, http.MethodPost, "/auth/login", map[string]string{"email": "owner@uwaterloo.ca", "password": testPassword})
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("login while suspended = %d, want 403", res.StatusCode)
	}
	if got := decode[errorEnvelope](t, res); got.Code != "forbidden" {
		t.Errorf("code = %q", got.Code)
	}

	// A wrong password says nothing about the suspension.
	res = h.do(t, http.MethodPost, "/auth/login", map[string]string{"email": "owner@uwaterloo.ca", "password": "not the password"})
	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("wrong password on a suspended account = %d, want the ordinary 401", res.StatusCode)
	}

	// Suspending again changes nothing.
	if res := a.do(t, http.MethodPost, "/admin/users/"+owner+"/suspend", map[string]string{"reason": goodReason}); res.StatusCode != http.StatusConflict {
		t.Errorf("second suspend = %d, want 409", res.StatusCode)
	}

	res = a.do(t, http.MethodPost, "/admin/users/"+owner+"/reinstate", map[string]string{"reason": "Appeal accepted after review"})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("reinstate = %d", res.StatusCode)
	}
	if l, o, st := publicCounts(); l != 1 || o != 1 || st != http.StatusOK {
		t.Errorf("after reinstating: %d listings, %d offers, listing %d — want everything back", l, o, st)
	}
	login(t, h, "owner@uwaterloo.ca")

	if res := a.do(t, http.MethodPost, "/admin/users/"+owner+"/reinstate", map[string]string{"reason": "Appeal accepted after review"}); res.StatusCode != http.StatusConflict {
		t.Errorf("reinstating an active account = %d, want 409", res.StatusCode)
	}

	entries := ledgerFor(t, a, owner)
	if len(entries) != 2 || entries[0].Action != "reinstate" || entries[1].Action != "suspend" {
		t.Fatalf("ledger = %+v, want reinstate then suspend", entries)
	}
	if entries[1].Actor == nil || entries[1].Actor.Username != "operator" || entries[1].Reason != goodReason {
		t.Errorf("suspend entry = %+v", entries[1])
	}
	if entries[0].Detail["suspend_reason"] != goodReason {
		t.Errorf("the reinstate entry lost the reason it lifted: %v", entries[0].Detail)
	}
}

func TestASuspendedPostersRequestLeavesPublicView(t *testing.T) {
	h, requestID, _ := twoAccounts(t)
	poster := userID(t, h, "poster@uwaterloo.ca")
	a, _ := adminBrowser(t, h)

	if res := a.do(t, http.MethodPost, "/admin/users/"+poster+"/suspend", map[string]string{"reason": goodReason}); res.StatusCode != http.StatusOK {
		t.Fatalf("suspend = %d", res.StatusCode)
	}
	if _, page := getJSON[requestPageBody](t, h.server.URL+"/requests"); page.Meta.Total != 0 {
		t.Errorf("public requests = %d, want 0", page.Meta.Total)
	}
	res, err := http.Get(h.server.URL + "/requests/" + requestID)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("GET the request = %d, want 404", res.StatusCode)
	}
}

func TestManualVerificationLetsAnAccountPost(t *testing.T) {
	h, _ := signedIn(t, false) // poster@, unverified
	target := userID(t, h, "poster@uwaterloo.ca")
	a, _ := adminBrowser(t, h)

	if res, _ := createListing(t, h, validListing()); res.StatusCode != http.StatusForbidden {
		t.Fatalf("unverified create = %d, want 403", res.StatusCode)
	}

	res := a.do(t, http.MethodPost, "/admin/users/"+target+"/verify", map[string]string{"reason": "Checked their WatCard in person"})
	if res.StatusCode != http.StatusOK || !decode[adminUserBody](t, res).Verified {
		t.Fatalf("verify = %d", res.StatusCode)
	}

	// The claim is in the token, so it takes a new session to carry it.
	if res := h.do(t, http.MethodPost, "/auth/refresh", nil); res.StatusCode != http.StatusOK {
		t.Fatalf("refresh = %d", res.StatusCode)
	}
	if res, _ := createListing(t, h, validListing()); res.StatusCode != http.StatusCreated {
		t.Errorf("create after manual verification = %d, want 201", res.StatusCode)
	}

	if res := a.do(t, http.MethodPost, "/admin/users/"+target+"/verify", map[string]string{"reason": "Checked their WatCard in person"}); res.StatusCode != http.StatusConflict {
		t.Errorf("verifying twice = %d, want 409", res.StatusCode)
	}
	if entries := ledgerFor(t, a, target); len(entries) != 1 || entries[0].Action != "verify" {
		t.Errorf("ledger = %+v, want one verify", entries)
	}
}

func TestRoleChangeAndItsGuards(t *testing.T) {
	h, _ := signedIn(t, true)
	target := userID(t, h, "poster@uwaterloo.ca")
	a, self := adminBrowser(t, h)

	role := func(id, to, reason string) *http.Response {
		return a.do(t, http.MethodPost, "/admin/users/"+id+"/role", map[string]string{"role": to, "reason": reason})
	}

	res := role(target, "admin", "Second operator for the winter term")
	if res.StatusCode != http.StatusOK || decode[adminUserBody](t, res).Role != "admin" {
		t.Fatalf("promote = %d", res.StatusCode)
	}
	// The promotion ended their session; the new role arrives with the next login.
	if res := h.do(t, http.MethodPost, "/auth/refresh", nil); res.StatusCode != http.StatusUnauthorized {
		t.Errorf("refresh after a role change = %d, want 401", res.StatusCode)
	}

	if res := role(target, "admin", "Second operator for the winter term"); res.StatusCode != http.StatusConflict {
		t.Errorf("promoting an admin = %d, want 409", res.StatusCode)
	}
	if res := role(self, "user", "Stepping down from operating"); res.StatusCode != http.StatusConflict {
		t.Errorf("demoting yourself = %d, want 409", res.StatusCode)
	}
	if res := role(target, "moderator", "A role this feature does not grant"); res.StatusCode != http.StatusBadRequest {
		t.Errorf("moderator = %d, want 400", res.StatusCode)
	}
	if res := role(target, "user", "Winter term is over now"); res.StatusCode != http.StatusOK {
		t.Errorf("demote = %d", res.StatusCode)
	}

	if entries := ledgerFor(t, a, target); len(entries) != 2 {
		t.Errorf("ledger = %d entries, want 2: refusals are not logged", len(entries))
	}
}

func TestAccountActionRefusals(t *testing.T) {
	h, _ := signedIn(t, true)
	target := userID(t, h, "poster@uwaterloo.ca")
	a, self := adminBrowser(t, h)

	cases := []struct {
		name   string
		path   string
		body   any
		status int
		field  string
	}{
		{"short reason", "/admin/users/" + target + "/suspend", map[string]string{"reason": "  spam  "}, http.StatusBadRequest, "reason"},
		{"no body", "/admin/users/" + target + "/suspend", nil, http.StatusBadRequest, ""},
		{"yourself", "/admin/users/" + self + "/suspend", map[string]string{"reason": goodReason}, http.StatusConflict, ""},
		{"no such account", "/admin/users/3f7c1b2a-9d4e-4c8a-8f1b-2e3d4c5b6a70/suspend", map[string]string{"reason": goodReason}, http.StatusNotFound, ""},
	}
	for _, tc := range cases {
		res := a.do(t, http.MethodPost, tc.path, tc.body)
		if res.StatusCode != tc.status {
			t.Errorf("%s: status %d, want %d", tc.name, res.StatusCode, tc.status)
			continue
		}
		if tc.field != "" {
			body := decode[struct {
				Details map[string]string `json:"details"`
			}](t, res)
			if body.Details[tc.field] == "" {
				t.Errorf("%s: no detail on %q", tc.name, tc.field)
			}
		}
	}

	if entries := ledgerFor(t, a, target); len(entries) != 0 {
		t.Errorf("refusals wrote %d ledger rows", len(entries))
	}
	if n, _ := h.db.CountAdminActions(context.Background()); n != 1 {
		t.Errorf("ledger holds %d rows, want only the admin's own promotion", n)
	}
}

func TestAccountActionsAreInvisibleToNonAdmins(t *testing.T) {
	h, _ := signedIn(t, true)
	target := userID(t, h, "poster@uwaterloo.ca")

	anonymous := anotherBrowser(t, h)
	for _, who := range []*harness{h, anonymous} {
		for _, action := range []string{"suspend", "reinstate", "verify", "role"} {
			path := "/admin/users/" + target + "/" + action
			res := who.do(t, http.MethodPost, path, map[string]string{"reason": goodReason, "role": "admin"})
			if res.StatusCode != http.StatusNotFound {
				t.Errorf("POST %s = %d, want 404", path, res.StatusCode)
				continue
			}
			if got := decode[errorEnvelope](t, res); got.Message != "No endpoint at "+path {
				t.Errorf("POST %s message = %q", path, got.Message)
			}
		}
	}

	// And nothing happened.
	u, _ := h.db.GetUserByEmail(context.Background(), "poster@uwaterloo.ca")
	if u.SuspendedAt != nil || u.Role != "user" {
		t.Errorf("a refused action changed the account: %+v", u)
	}
}
