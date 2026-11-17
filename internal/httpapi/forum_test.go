package httpapi_test

import (
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/20age1million/WaterlooStar-Backend/internal/apierror"
	"github.com/20age1million/WaterlooStar-Backend/internal/ratelimit"
)

type forumPostJSON struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Author    struct {
		ID       string `json:"id"`
		Username string `json:"username"`
		Verified bool   `json:"verified"`
	} `json:"author"`
}

type forumPageJSON struct {
	Data []forumPostJSON `json:"data"`
	Meta struct {
		Page       int `json:"page"`
		PerPage    int `json:"per_page"`
		Total      int `json:"total"`
		TotalPages int `json:"total_pages"`
	} `json:"meta"`
}

func forumPost(t *testing.T, h *harness, title, body string) forumPostJSON {
	t.Helper()
	res := h.do(t, http.MethodPost, "/forum/posts", map[string]string{"title": title, "body": body})
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create forum post: %d, body %+v", res.StatusCode, decode[map[string]any](t, res))
	}
	return decode[forumPostJSON](t, res)
}

func forumPage(t *testing.T, h *harness, query string) forumPageJSON {
	t.Helper()
	res := h.do(t, http.MethodGet, "/forum/posts"+query, nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("browse forum: %d", res.StatusCode)
	}
	return decode[forumPageJSON](t, res)
}

func TestForumPublicCRUDAndSessionAuthorship(t *testing.T) {
	owner, email := signedIn(t, false) // This scope allows logged-in unverified users.
	anonymous := anotherBrowser(t, owner)
	if empty := forumPage(t, anonymous, ""); empty.Data == nil || len(empty.Data) != 0 || empty.Meta.Total != 0 {
		t.Fatalf("initial empty page = %+v", empty)
	}
	mailCount := owner.mail.count()
	res := owner.do(t, http.MethodPost, "/forum/posts", map[string]any{
		"title": "  A campus question  ", "body": "\nA message to the community.\n",
		"author_id": uuid.NewString(), "author": map[string]string{"id": uuid.NewString(), "username": "someone-else"},
	})
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("unverified signed-in creation: %d", res.StatusCode)
	}
	post := decode[forumPostJSON](t, res)
	if post.Title != "A campus question" || post.Body != "A message to the community." || post.Author.ID != userID(t, owner, email) || post.Author.Verified {
		t.Fatalf("trim/session authorship = %+v", post)
	}
	page := forumPage(t, anonymous, "")
	if page.Meta.Total != 1 || len(page.Data) != 1 || page.Data[0].ID != post.ID {
		t.Fatalf("anonymous page = %+v", page)
	}
	res = anonymous.do(t, http.MethodGet, "/forum/posts/"+post.ID, nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("anonymous detail = %d", res.StatusCode)
	}
	raw := decode[map[string]any](t, res)
	author := raw["author"].(map[string]any)
	for _, private := range []string{"email", "password_hash", "phone", "suspended_at", "role", "email_notifications"} {
		if _, found := author[private]; found {
			t.Errorf("public author leaked %s", private)
		}
	}
	res = owner.do(t, http.MethodPatch, "/forum/posts/"+post.ID, map[string]any{"title": "  A better question  ", "author_id": uuid.NewString()})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("edit title = %d", res.StatusCode)
	}
	edited := decode[forumPostJSON](t, res)
	if edited.Title != "A better question" || edited.Body != post.Body || edited.Author.ID != post.Author.ID || !edited.CreatedAt.Equal(post.CreatedAt) || !edited.UpdatedAt.After(post.UpdatedAt) {
		t.Fatalf("partial edit lost data or authorship: %+v", edited)
	}
	res = owner.do(t, http.MethodPatch, "/forum/posts/"+post.ID, map[string]string{"body": "  An updated message.  "})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("edit body = %d", res.StatusCode)
	}
	edited = decode[forumPostJSON](t, res)
	if edited.Body != "An updated message." || edited.Title != "A better question" {
		t.Fatalf("body-only edit = %+v", edited)
	}
	public := decode[forumPostJSON](t, anonymous.do(t, http.MethodGet, "/forum/posts/"+post.ID, nil))
	if public.Title != edited.Title || public.Body != edited.Body {
		t.Fatalf("public read did not show edit: %+v", public)
	}
	if res = owner.do(t, http.MethodDelete, "/forum/posts/"+post.ID, nil); res.StatusCode != http.StatusNoContent {
		t.Fatalf("delete = %d", res.StatusCode)
	}
	if page = forumPage(t, anonymous, ""); page.Meta.Total != 0 || page.Data == nil || len(page.Data) != 0 {
		t.Fatalf("deleted post remained public: %+v", page)
	}
	for _, method := range []string{http.MethodGet, http.MethodPatch, http.MethodDelete} {
		res = owner.do(t, method, "/forum/posts/"+post.ID, map[string]string{"title": "Cannot restore"})
		if res.StatusCode != http.StatusNotFound {
			t.Errorf("%s deleted post = %d, want 404", method, res.StatusCode)
		}
	}
	owner.db.mu.Lock()
	retained := owner.db.forumPosts[0]
	owner.db.mu.Unlock()
	if retained.DeletedAt == nil || retained.Body != edited.Body {
		t.Fatalf("soft delete did not retain content: %+v", retained)
	}
	if owner.mail.count() != mailCount {
		t.Fatal("forum CRUD sent email")
	}
}

func TestForumAnonymousAndOtherAuthorWritesAreRefused(t *testing.T) {
	owner, _ := signedIn(t, true)
	post := forumPost(t, owner, "An owner's question", "An owner's message")
	anonymous := anotherBrowser(t, owner)
	other := anotherBrowser(t, owner)
	register(t, other, "forum-other@uwaterloo.ca", "forum-other")
	for _, method := range []string{http.MethodPost, http.MethodPatch, http.MethodDelete} {
		path := "/forum/posts"
		if method != http.MethodPost {
			path += "/" + post.ID
		}
		res := anonymous.do(t, method, path, map[string]string{"title": "A forged edit", "body": "Should not be stored"})
		if res.StatusCode != http.StatusUnauthorized || decode[apierror.Envelope](t, res).Code != apierror.CodeUnauthorized {
			t.Fatalf("anonymous %s = %d, want 401", method, res.StatusCode)
		}
	}
	for _, method := range []string{http.MethodPatch, http.MethodDelete} {
		var first apierror.Envelope
		for i, id := range []string{post.ID, uuid.NewString()} {
			res := other.do(t, method, "/forum/posts/"+id, map[string]any{"title": "A forged edit", "body": "A forged message", "author_id": post.Author.ID})
			if res.StatusCode != http.StatusNotFound {
				t.Fatalf("non-owner %s = %d, want 404", method, res.StatusCode)
			}
			body := decode[apierror.Envelope](t, res)
			if body.Code != apierror.CodeNotFound {
				t.Errorf("non-owner envelope = %+v", body)
			}
			if i == 0 {
				first = body
			} else if body.Message != first.Message {
				t.Errorf("missing and other author's post differ: %q / %q", first.Message, body.Message)
			}
		}
	}
	current := decode[forumPostJSON](t, owner.do(t, http.MethodGet, "/forum/posts/"+post.ID, nil))
	if current.Title != post.Title || current.Body != post.Body || current.Author.ID != post.Author.ID {
		t.Fatalf("another author changed the post: %+v", current)
	}
}

func TestForumInputValidationAndUnicodeBoundaries(t *testing.T) {
	h, _ := signedIn(t, false)
	cases := []struct {
		name  string
		body  any
		field string
	}{
		{"missing title", map[string]any{"body": "A message"}, "title"},
		{"missing body", map[string]any{"title": "A question"}, "body"},
		{"null title", map[string]any{"title": nil, "body": "A message"}, "title"},
		{"blank title", map[string]string{"title": " \n\t\u2003", "body": "A message"}, "title"},
		{"blank body", map[string]string{"title": "A question", "body": " \n\t\u2003"}, "body"},
		{"long title", map[string]string{"title": strings.Repeat("界", 201), "body": "A message"}, "title"},
		{"long body", map[string]string{"title": "A question", "body": strings.Repeat("界", 8001)}, "body"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := h.do(t, http.MethodPost, "/forum/posts", tc.body)
			if res.StatusCode != http.StatusBadRequest {
				t.Fatalf("status = %d", res.StatusCode)
			}
			body := decode[apierror.Envelope](t, res)
			if body.Code != apierror.CodeValidation || body.Details[tc.field] == "" {
				t.Errorf("field error = %+v", body)
			}
		})
	}
	post := forumPost(t, h, "  "+strings.Repeat("界", 200)+"  ", "\n"+strings.Repeat("界", 8000)+"\n")
	if post.Title != strings.Repeat("界", 200) || post.Body != strings.Repeat("界", 8000) {
		t.Fatal("valid Unicode boundary text was changed")
	}
	for _, tc := range []struct {
		name  string
		body  map[string]any
		field string
	}{
		{"empty patch", map[string]any{}, "body"},
		{"unknown-only patch", map[string]any{"author_id": uuid.NewString()}, "body"},
		{"null title", map[string]any{"title": nil}, "title"},
		{"null body", map[string]any{"body": nil}, "body"},
		{"empty title", map[string]any{"title": ""}, "title"},
		{"whitespace body", map[string]any{"body": "\t\n\u2003"}, "body"},
		{"long title", map[string]any{"title": strings.Repeat("界", 201)}, "title"},
		{"long body", map[string]any{"body": strings.Repeat("界", 8001)}, "body"},
	} {
		t.Run("patch "+tc.name, func(t *testing.T) {
			res := h.do(t, http.MethodPatch, "/forum/posts/"+post.ID, tc.body)
			if res.StatusCode != http.StatusBadRequest {
				t.Fatalf("status = %d", res.StatusCode)
			}
			body := decode[apierror.Envelope](t, res)
			if body.Details[tc.field] == "" {
				t.Errorf("field error = %+v", body)
			}
		})
	}
	for _, method := range []string{http.MethodPost, http.MethodPatch} {
		path := "/forum/posts"
		if method == http.MethodPatch {
			path += "/" + post.ID
		}
		for _, body := range []any{nil, map[string]any{"title": 42, "body": "A message"}} {
			res := h.do(t, method, path, body)
			if res.StatusCode != http.StatusBadRequest || decode[apierror.Envelope](t, res).Code != apierror.CodeBadRequest {
				t.Errorf("%s invalid JSON shape = %d, want envelope 400", method, res.StatusCode)
			}
		}
	}
}

func TestForumCurrentAccountStateAndCSRF(t *testing.T) {
	h, email := signedIn(t, false)
	post := forumPost(t, h, "A current account's question", "A current account's message")
	for _, method := range []string{http.MethodPost, http.MethodPatch, http.MethodDelete} {
		path := "/forum/posts"
		if method != http.MethodPost {
			path += "/" + post.ID
		}
		req, err := http.NewRequest(method, h.server.URL+path, strings.NewReader(`{"title":"Missing CSRF","body":"Must not be stored"}`))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		res, err := h.client.Do(req) // Cookies, but deliberately no CSRF header.
		if err != nil {
			t.Fatal(err)
		}
		if res.StatusCode != http.StatusForbidden || decode[apierror.Envelope](t, res).Code != apierror.CodeForbidden {
			t.Errorf("missing CSRF %s = %d", method, res.StatusCode)
		}
		res.Body.Close()
	}
	id := uuid.MustParse(userID(t, h, email))
	h.db.mu.Lock()
	user := h.db.users[id]
	now := time.Now()
	user.SuspendedAt = &now
	h.db.users[id] = user
	h.db.mu.Unlock()
	for _, method := range []string{http.MethodPost, http.MethodPatch, http.MethodDelete} {
		path := "/forum/posts"
		if method != http.MethodPost {
			path += "/" + post.ID
		}
		res := h.do(t, method, path, map[string]string{"title": "A stale access token", "body": "Must not be stored"})
		if res.StatusCode != http.StatusForbidden {
			t.Errorf("suspended %s = %d, want 403", method, res.StatusCode)
		}
	}
	if page := forumPage(t, anotherBrowser(t, h), ""); page.Meta.Total != 0 {
		t.Errorf("suspended author's post remained public: %+v", page)
	}
	if res := h.do(t, http.MethodGet, "/forum/posts/"+post.ID, nil); res.StatusCode != http.StatusNotFound {
		t.Errorf("suspended author's detail = %d, want 404", res.StatusCode)
	}
	h.db.mu.Lock()
	delete(h.db.users, id)
	h.db.mu.Unlock()
	if res := h.do(t, http.MethodPost, "/forum/posts", map[string]string{"title": "An absent account", "body": "Must not be stored"}); res.StatusCode != http.StatusUnauthorized {
		t.Errorf("removed account = %d, want 401", res.StatusCode)
	}
}

func TestForumSearchPaginationAndBindingErrors(t *testing.T) {
	h, _ := signedIn(t, false)
	first := forumPost(t, h, "Campus study spots", "A quiet place to read")
	forumPost(t, h, "A warm drink", "A campus coffee break")
	latest := forumPost(t, h, "A winter walk", "A useful pair of gloves")
	anonymous := anotherBrowser(t, h)
	page := forumPage(t, anonymous, "?per_page=2")
	if page.Meta.PerPage != 2 || page.Meta.Total != 3 || page.Meta.TotalPages != 2 || len(page.Data) != 2 || page.Data[0].ID != latest.ID {
		t.Fatalf("first page = %+v", page)
	}
	page = forumPage(t, anonymous, "?per_page=2&page=2")
	if len(page.Data) != 1 || page.Data[0].ID != first.ID {
		t.Fatalf("second page = %+v", page)
	}
	if page = forumPage(t, anonymous, "?q=campus"); page.Meta.Total != 2 || len(page.Data) != 2 {
		t.Errorf("title/body search = %+v", page)
	}
	if page = forumPage(t, anonymous, "?q=poster"); page.Meta.Total != 0 {
		t.Errorf("search matched private or author information: %+v", page)
	}
	if page = forumPage(t, anonymous, "?q=%20%20&per_page=10000&page=-1"); page.Meta.Total != 3 || page.Meta.PerPage != 100 || page.Meta.Page != 1 {
		t.Errorf("default and clamped page = %+v", page)
	}
	if page = forumPage(t, anonymous, "?page=99"); page.Meta.Total != 3 || page.Data == nil || len(page.Data) != 0 {
		t.Errorf("past-end page = %+v", page)
	}
	for _, path := range []string{"/forum/posts/not-a-uuid", "/forum/posts?page=wrong", "/forum/posts?page=2147483647", "/forum/posts?q=" + url.QueryEscape(strings.Repeat("x", 201))} {
		res := anonymous.do(t, http.MethodGet, path, nil)
		if res.StatusCode != http.StatusBadRequest {
			t.Errorf("%s = %d, want 400", path, res.StatusCode)
		}
		body := decode[apierror.Envelope](t, res)
		if body.Code != apierror.CodeBadRequest && body.Code != apierror.CodeValidation {
			t.Errorf("binding/validation envelope = %+v", body)
		}
	}
}

func TestForumDatabaseErrorsUseSanitisedEnvelope(t *testing.T) {
	h, _ := signedIn(t, false)
	post := forumPost(t, h, "An ordinary title", "An ordinary body")
	h.db.mu.Lock()
	h.db.forumErr = errors.New("private database connection or table detail")
	h.db.mu.Unlock()
	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/forum/posts"},
		{http.MethodGet, "/forum/posts/" + post.ID},
		{http.MethodPost, "/forum/posts"},
		{http.MethodPatch, "/forum/posts/" + post.ID},
		{http.MethodDelete, "/forum/posts/" + post.ID},
	} {
		res := h.do(t, tc.method, tc.path, map[string]string{"title": "An ordinary title", "body": "An ordinary body"})
		if res.StatusCode != http.StatusInternalServerError {
			t.Errorf("%s %s = %d, want 500", tc.method, tc.path, res.StatusCode)
		}
		body, err := io.ReadAll(res.Body)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(body), "private database") || !strings.Contains(string(body), string(apierror.CodeInternal)) {
			t.Errorf("internal error leaked or lost envelope: %s", body)
		}
	}
}

func TestForumCreationUsesTheExistingPostingAllowance(t *testing.T) {
	h, clock := limitedHarness(t)
	register(t, h, "forum-limit@uwaterloo.ca", "forum-limit")
	var first forumPostJSON
	for i := range ratelimit.WritePerUser.Burst - 1 {
		post := forumPost(t, h, "A question for the forum", "A message for the forum")
		if i == 0 {
			first = post
		}
	}
	// The same rule name means the same user's existing posting allowance,
	// rather than another full allowance for each newly introduced surface.
	if res, _ := createListing(t, h, validListing()); res.StatusCode != http.StatusCreated {
		t.Fatalf("last shared posting token: %d", res.StatusCode)
	}
	envelope := assertRefusal(t, h.do(t, http.MethodPost, "/forum/posts", map[string]string{"title": "One post too many", "body": "Must wait for the allowance"}))
	if !strings.HasPrefix(envelope.Message, "You have posted a lot in a short time.") {
		t.Errorf("posting refusal = %q", envelope.Message)
	}
	// Existing owner edits/deletes and public reads stay outside this rule.
	if res := h.do(t, http.MethodPatch, "/forum/posts/"+first.ID, map[string]string{"title": "An edit after posting"}); res.StatusCode != http.StatusOK {
		t.Errorf("edit after exhausting creation allowance = %d", res.StatusCode)
	}
	if res := h.do(t, http.MethodGet, "/forum/posts/"+first.ID, nil); res.StatusCode != http.StatusOK {
		t.Errorf("detail after exhausting creation allowance = %d", res.StatusCode)
	}
	if res := h.do(t, http.MethodDelete, "/forum/posts/"+first.ID, nil); res.StatusCode != http.StatusNoContent {
		t.Errorf("delete after exhausting creation allowance = %d", res.StatusCode)
	}
	other := anotherBrowser(t, h)
	register(t, other, "forum-other-limit@uwaterloo.ca", "forum-other-limit")
	forumPost(t, other, "A different student's post", "The posting allowance belongs to the student")
	clock.advance(ratelimit.WritePerUser.Window + time.Second)
	forumPost(t, h, "A post after the allowance refills", "The existing policy refills without a real-time wait")
}
