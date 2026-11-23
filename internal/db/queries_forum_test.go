package db_test

import (
	"context"
	"errors"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/20age1million/WaterlooStar-Backend/internal/db/dbtest"
	"github.com/20age1million/WaterlooStar-Backend/internal/db/sqlcgen"
)

func forumPost(t *testing.T, q *sqlcgen.Queries, author uuid.UUID, title, body string) sqlcgen.ForumPost {
	t.Helper()
	post, err := q.CreateForumPost(context.Background(), sqlcgen.CreateForumPostParams{
		AuthorID: author,
		Title:    title,
		Body:     body,
	})
	if err != nil {
		t.Fatalf("CreateForumPost: %v", err)
	}
	return post
}

func forumBrowse(t *testing.T, q *sqlcgen.Queries, search *string, limit, offset int32) ([]sqlcgen.ListForumPostsRow, int64) {
	t.Helper()
	ctx := context.Background()
	rows, err := q.ListForumPosts(ctx, sqlcgen.ListForumPostsParams{
		Search: search,
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		t.Fatalf("ListForumPosts: %v", err)
	}
	total, err := q.CountForumPosts(ctx, search)
	if err != nil {
		t.Fatalf("CountForumPosts: %v", err)
	}
	return rows, total
}

func TestForumQueriesEnforceOwnershipAndKeepDeletedContent(t *testing.T) {
	t.Parallel()
	q, pool := dbtest.Queries(t)
	ctx := context.Background()
	// A signed-in account may post before confirming its address.
	author := dbtest.User(t, q, "forum-author@uwaterloo.ca", false)
	stranger := dbtest.User(t, q, "forum-stranger@uwaterloo.ca", true)
	post := forumPost(t, q, author.ID, "Quiet study spaces", "Bring a notebook.")

	public, err := q.GetForumPost(ctx, post.ID)
	if err != nil || public.ForumPost.AuthorID != author.ID || public.AuthorUsername != author.Username || public.AuthorVerified {
		t.Fatalf("GetForumPost: %+v, %v", public, err)
	}
	if rows, total := forumBrowse(t, q, nil, 20, 0); total != 1 || len(rows) != 1 || rows[0].ForumPost.ID != post.ID {
		t.Fatalf("public browse = %d rows, total %d", len(rows), total)
	}

	_, err = q.UpdateForumPost(ctx, sqlcgen.UpdateForumPostParams{
		ID: post.ID, AuthorID: stranger.ID, Title: ptrString("Stolen authorship"),
	})
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("another author's update = %v, want no rows", err)
	}
	if _, err := q.DeleteForumPost(ctx, sqlcgen.DeleteForumPostParams{ID: post.ID, AuthorID: stranger.ID}); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("another author's delete = %v, want no rows", err)
	}
	unchanged, err := q.GetForumPost(ctx, post.ID)
	if err != nil || unchanged.ForumPost.Title != post.Title || unchanged.ForumPost.Body != post.Body {
		t.Fatalf("a refused write changed the post: %+v, %v", unchanged, err)
	}

	updated, err := q.UpdateForumPost(ctx, sqlcgen.UpdateForumPostParams{
		ID: post.ID, AuthorID: author.ID, Title: ptrString("A place for reading"),
	})
	if err != nil {
		t.Fatalf("author update: %v", err)
	}
	if updated.Title != "A place for reading" || updated.Body != post.Body || updated.AuthorID != author.ID || updated.UpdatedAt.Before(post.CreatedAt) {
		t.Fatalf("partial update lost existing values: %+v", updated)
	}
	updated, err = q.UpdateForumPost(ctx, sqlcgen.UpdateForumPostParams{
		ID: post.ID, AuthorID: author.ID, Body: ptrString("A thermos of tea is welcome."),
	})
	if err != nil || updated.Title != "A place for reading" {
		t.Fatalf("body-only update: %+v, %v", updated, err)
	}
	if rows, total := forumBrowse(t, q, ptrString("thermos"), 20, 0); total != 1 || len(rows) != 1 {
		t.Fatalf("edited body was not reindexed: %d rows, total %d", len(rows), total)
	}
	if _, total := forumBrowse(t, q, ptrString("notebook"), 20, 0); total != 0 {
		t.Error("the search index retained the old body")
	}

	deleted, err := q.DeleteForumPost(ctx, sqlcgen.DeleteForumPostParams{ID: post.ID, AuthorID: author.ID})
	if err != nil || deleted != post.ID {
		t.Fatalf("author delete: %s, %v", deleted, err)
	}
	if _, err := q.GetForumPost(ctx, post.ID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("deleted post still public: %v", err)
	}
	if rows, total := forumBrowse(t, q, nil, 20, 0); total != 0 || len(rows) != 0 {
		t.Errorf("deleted post still listed: %d rows, total %d", len(rows), total)
	}
	if _, err := q.UpdateForumPost(ctx, sqlcgen.UpdateForumPostParams{ID: post.ID, AuthorID: author.ID, Body: ptrString("Resurrected")}); !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("a deleted post was edited: %v", err)
	}
	if _, err := q.DeleteForumPost(ctx, sqlcgen.DeleteForumPostParams{ID: post.ID, AuthorID: author.ID}); !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("repeat deletion = %v, want no rows", err)
	}
	var retainedBody string
	var deletedAt *time.Time
	if err := pool.QueryRow(ctx, "SELECT body, deleted_at FROM forum_posts WHERE id = $1", post.ID).Scan(&retainedBody, &deletedAt); err != nil || retainedBody != updated.Body || deletedAt == nil {
		t.Fatalf("soft deletion lost content: body %q, deleted_at %v, error %v", retainedBody, deletedAt, err)
	}
	missing := uuid.New()
	if _, err := q.GetForumPost(ctx, missing); !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("unknown post read = %v, want no rows", err)
	}
	if _, err := q.UpdateForumPost(ctx, sqlcgen.UpdateForumPostParams{ID: missing, AuthorID: author.ID, Body: ptrString("A missing post")}); !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("unknown post update = %v, want no rows", err)
	}
	if _, err := q.DeleteForumPost(ctx, sqlcgen.DeleteForumPostParams{ID: missing, AuthorID: author.ID}); !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("unknown post deletion = %v, want no rows", err)
	}
}

func TestForumQueriesFilterSuspendedAuthorsAndRejectTheirWrites(t *testing.T) {
	t.Parallel()
	q, _ := dbtest.Queries(t)
	ctx := context.Background()
	author := dbtest.User(t, q, "forum-suspend@uwaterloo.ca", true)
	admin := dbtest.User(t, q, "forum-admin@uwaterloo.ca", true)
	post := forumPost(t, q, author.ID, "An existing discussion", "Keep the existing content.")
	if _, err := q.SuspendUser(ctx, sqlcgen.SuspendUserParams{ID: author.ID, ActorID: &admin.ID, Reason: ptrString("Forum test suspension")}); err != nil {
		t.Fatalf("suspend author: %v", err)
	}
	if _, err := q.GetForumPost(ctx, post.ID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("a suspended author's post was public: %v", err)
	}
	if rows, total := forumBrowse(t, q, nil, 20, 0); total != 0 || len(rows) != 0 {
		t.Errorf("a suspended author's post was listed: %d rows, total %d", len(rows), total)
	}
	if _, err := q.CreateForumPost(ctx, sqlcgen.CreateForumPostParams{AuthorID: author.ID, Title: "New post", Body: "A suspended account cannot post."}); !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("suspended create = %v, want no rows", err)
	}
	if _, err := q.UpdateForumPost(ctx, sqlcgen.UpdateForumPostParams{ID: post.ID, AuthorID: author.ID, Body: ptrString("A suspended edit")}); !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("suspended update = %v, want no rows", err)
	}
	if _, err := q.DeleteForumPost(ctx, sqlcgen.DeleteForumPostParams{ID: post.ID, AuthorID: author.ID}); !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("suspended delete = %v, want no rows", err)
	}
	if _, err := q.CreateForumPost(ctx, sqlcgen.CreateForumPostParams{AuthorID: uuid.New(), Title: "No account", Body: "A removed account cannot post."}); !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("missing author create = %v, want no rows", err)
	}
	if _, err := q.ReinstateUser(ctx, author.ID); err != nil {
		t.Fatalf("reinstate author: %v", err)
	}
	if restored, err := q.GetForumPost(ctx, post.ID); err != nil || restored.ForumPost.Body != post.Body {
		t.Errorf("reinstatement changed the existing post: %+v, %v", restored, err)
	}
}

func TestForumSearchAndStablePagination(t *testing.T) {
	t.Parallel()
	q, pool := dbtest.Queries(t)
	ctx := context.Background()
	author := dbtest.User(t, q, "unmatchablestudentname@uwaterloo.ca", true)
	posts := []sqlcgen.ForumPost{
		forumPost(t, q, author.ID, "Quiet study spaces", "Bring a notebook."),
		forumPost(t, q, author.ID, "Campus lunch ideas", "The cafeteria has soup."),
		forumPost(t, q, author.ID, "A walk after class", "An afternoon break together."),
	}
	stamp := time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)
	for _, post := range posts {
		if _, err := pool.Exec(ctx, "UPDATE forum_posts SET created_at = $2 WHERE id = $1", post.ID, stamp); err != nil {
			t.Fatalf("set tied creation times: %v", err)
		}
	}
	newest := forumPost(t, q, author.ID, "A newer post", "The latest discussion.")
	if _, err := pool.Exec(ctx, "UPDATE forum_posts SET created_at = $2 WHERE id = $1", newest.ID, stamp.Add(time.Hour)); err != nil {
		t.Fatalf("set newer creation time: %v", err)
	}
	sort.Slice(posts, func(i, j int) bool { return posts[i].ID.String() > posts[j].ID.String() })
	want := []uuid.UUID{newest.ID, posts[0].ID, posts[1].ID, posts[2].ID}
	first, total := forumBrowse(t, q, nil, 2, 0)
	second, secondTotal := forumBrowse(t, q, nil, 2, 2)
	if total != 4 || secondTotal != 4 || len(first) != 2 || len(second) != 2 {
		t.Fatalf("pagination rows %d + %d, totals %d and %d", len(first), len(second), total, secondTotal)
	}
	for index, row := range append(first, second...) {
		if row.ForumPost.ID != want[index] {
			t.Errorf("page position %d = %s, want %s", index, row.ForumPost.ID, want[index])
		}
	}
	if rows, total := forumBrowse(t, q, nil, 2, 10); len(rows) != 0 || total != 4 {
		t.Errorf("page beyond the end: %d rows, total %d", len(rows), total)
	}
	for _, search := range []string{"notebook", "cafeteria", `"quiet study"`} {
		rows, total := forumBrowse(t, q, ptrString(search), 20, 0)
		if total != 1 || len(rows) != 1 {
			t.Errorf("search %q = %d rows, total %d", search, len(rows), total)
		}
	}
	if rows, total := forumBrowse(t, q, ptrString(author.Username), 20, 0); total != 0 || len(rows) != 0 {
		t.Errorf("search matched account data: %d rows, total %d", len(rows), total)
	}
}

func TestForumTextConstraintsCountUnicodeCharacters(t *testing.T) {
	t.Parallel()
	q, _ := dbtest.Queries(t)
	ctx := context.Background()
	author := dbtest.User(t, q, "forum-limits@uwaterloo.ca", false)
	validTitle := strings.Repeat("界", 200)
	validBody := strings.Repeat("界", 8000)
	post := forumPost(t, q, author.ID, validTitle, validBody)
	if post.Title != validTitle || post.Body != validBody {
		t.Fatal("valid Unicode text changed in storage")
	}
	cases := []struct {
		name  string
		title string
		body  string
	}{
		{"blank title", "   ", "A body"},
		{"whitespace title", "\t\r\n", "A body"},
		{"long title", strings.Repeat("界", 201), "A body"},
		{"blank body", "A title", "   "},
		{"whitespace body", "A title", "\t\r\n"},
		{"long body", "A title", strings.Repeat("界", 8001)},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			_, err := q.CreateForumPost(ctx, sqlcgen.CreateForumPostParams{AuthorID: author.ID, Title: test.title, Body: test.body})
			var databaseError *pgconn.PgError
			if !errors.As(err, &databaseError) || (databaseError.Code != "23514" && databaseError.Code != "22001") {
				t.Errorf("invalid text returned %v, want a length/check constraint violation", err)
			}
		})
	}
	if _, err := q.UpdateForumPost(ctx, sqlcgen.UpdateForumPostParams{ID: post.ID, AuthorID: author.ID, Body: ptrString(" ")}); err == nil {
		t.Error("an update bypassed the body constraint")
	}
}

func TestForumMigrationPreservesVersion15Data(t *testing.T) {
	t.Parallel()
	staged := dbtest.NewAt(t, 15)
	ctx := context.Background()
	q := sqlcgen.New(staged.Pool)
	author := staged.User(t, "existing-author@uwaterloo.ca")
	student := staged.User(t, "existing-student@uwaterloo.ca")
	listing := dbtest.Listing(t, q, author.ID, dbtest.ListingOptions{Title: "An existing student home"})
	dbtest.Request(t, q, student.ID, dbtest.RequestOptions{})
	conversation := dbtest.Conversation(t, q, listing, student.ID)
	dbtest.Message(t, q, conversation.ID, student.ID, "An existing private message.")

	tables := []string{"users", "listings", "housing_requests", "conversations", "messages"}
	snapshot := func() map[string]string {
		t.Helper()
		rows := make(map[string]string, len(tables))
		for _, table := range tables {
			var content string
			// Table names are the fixed list above. The snapshot includes every
			// column so a migration cannot silently rewrite private or public data.
			query := "SELECT coalesce(jsonb_agg(to_jsonb(t) ORDER BY id)::text, '[]') FROM " + table + " t"
			if err := staged.Pool.QueryRow(ctx, query).Scan(&content); err != nil {
				t.Fatalf("snapshot %s: %v", table, err)
			}
			rows[table] = content
		}
		return rows
	}
	before := snapshot()
	staged.MigrateTo(t, 16)
	for table, after := range snapshot() {
		if after != before[table] {
			t.Errorf("migration 16 changed existing %s rows", table)
		}
	}
	post := forumPost(t, q, author.ID, "The new forum is available", "Existing accounts can post after migrating.")
	if public, err := q.GetForumPost(ctx, post.ID); err != nil || public.ForumPost.AuthorID != author.ID {
		t.Fatalf("forum after upgrade: %+v, %v", public, err)
	}
}
