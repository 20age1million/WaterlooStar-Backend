package httpapi_test

import (
	"context"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/20age1million/WaterlooStar-Backend/internal/db/sqlcgen"
)

// These mirror the SQL visibility and ownership invariants. The real SQL tests
// in internal/db cover PostgreSQL's full-text language and constraints.
func (f *fakeQuerier) visibleForumPost(post sqlcgen.ForumPost, search *string) bool {
	user, exists := f.users[post.AuthorID]
	if !exists || post.DeletedAt != nil || user.SuspendedAt != nil {
		return false
	}
	if search != nil {
		haystack := strings.ToLower(post.Title + " " + post.Body)
		for _, word := range strings.Fields(strings.ToLower(*search)) {
			if !strings.Contains(haystack, strings.Trim(word, `"`)) {
				return false
			}
		}
	}
	return true
}

func (f *fakeQuerier) ListForumPosts(_ context.Context, arg sqlcgen.ListForumPostsParams) ([]sqlcgen.ListForumPostsRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.forumErr != nil {
		return nil, f.forumErr
	}
	rows := make([]sqlcgen.ListForumPostsRow, 0)
	for _, post := range f.forumPosts {
		if !f.visibleForumPost(post, arg.Search) {
			continue
		}
		user := f.users[post.AuthorID]
		rows = append(rows, sqlcgen.ListForumPostsRow{ForumPost: post, AuthorUsername: user.Username, AuthorAvatarUrl: user.AvatarUrl, AuthorVerified: user.Verified})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].ForumPost.CreatedAt.Equal(rows[j].ForumPost.CreatedAt) {
			return rows[i].ForumPost.ID.String() > rows[j].ForumPost.ID.String()
		}
		return rows[i].ForumPost.CreatedAt.After(rows[j].ForumPost.CreatedAt)
	})
	start := int(arg.Offset)
	if start >= len(rows) {
		return []sqlcgen.ListForumPostsRow{}, nil
	}
	end := min(start+int(arg.Limit), len(rows))
	return rows[start:end], nil
}

func (f *fakeQuerier) CountForumPosts(_ context.Context, search *string) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.forumErr != nil {
		return 0, f.forumErr
	}
	var count int64
	for _, post := range f.forumPosts {
		if f.visibleForumPost(post, search) {
			count++
		}
	}
	return count, nil
}

func (f *fakeQuerier) GetForumPost(_ context.Context, id uuid.UUID) (sqlcgen.GetForumPostRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.forumErr != nil {
		return sqlcgen.GetForumPostRow{}, f.forumErr
	}
	for _, post := range f.forumPosts {
		if post.ID == id && f.visibleForumPost(post, nil) {
			user := f.users[post.AuthorID]
			return sqlcgen.GetForumPostRow{ForumPost: post, AuthorUsername: user.Username, AuthorAvatarUrl: user.AvatarUrl, AuthorVerified: user.Verified}, nil
		}
	}
	return sqlcgen.GetForumPostRow{}, pgx.ErrNoRows
}

func (f *fakeQuerier) CreateForumPost(_ context.Context, arg sqlcgen.CreateForumPostParams) (sqlcgen.ForumPost, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.forumErr != nil {
		return sqlcgen.ForumPost{}, f.forumErr
	}
	user, exists := f.users[arg.AuthorID]
	if !exists || user.SuspendedAt != nil {
		return sqlcgen.ForumPost{}, pgx.ErrNoRows
	}
	now := f.tick()
	post := sqlcgen.ForumPost{ID: uuid.New(), AuthorID: arg.AuthorID, Title: arg.Title, Body: arg.Body, CreatedAt: now, UpdatedAt: now}
	f.forumPosts = append(f.forumPosts, post)
	return post, nil
}

func (f *fakeQuerier) UpdateForumPost(_ context.Context, arg sqlcgen.UpdateForumPostParams) (sqlcgen.ForumPost, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.forumErr != nil {
		return sqlcgen.ForumPost{}, f.forumErr
	}
	for i, post := range f.forumPosts {
		if post.ID == arg.ID && post.AuthorID == arg.AuthorID && f.visibleForumPost(post, nil) {
			if arg.Title != nil {
				post.Title = *arg.Title
			}
			if arg.Body != nil {
				post.Body = *arg.Body
			}
			post.UpdatedAt = f.tick()
			f.forumPosts[i] = post
			return post, nil
		}
	}
	return sqlcgen.ForumPost{}, pgx.ErrNoRows
}

func (f *fakeQuerier) DeleteForumPost(_ context.Context, arg sqlcgen.DeleteForumPostParams) (uuid.UUID, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.forumErr != nil {
		return uuid.Nil, f.forumErr
	}
	for i, post := range f.forumPosts {
		if post.ID == arg.ID && post.AuthorID == arg.AuthorID && f.visibleForumPost(post, nil) {
			now := f.tick()
			post.DeletedAt = &now
			post.UpdatedAt = now
			f.forumPosts[i] = post
			return post.ID, nil
		}
	}
	return uuid.Nil, pgx.ErrNoRows
}
