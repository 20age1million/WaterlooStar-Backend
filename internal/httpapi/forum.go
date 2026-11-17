package httpapi

import (
	"context"
	"errors"
	"math"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/20age1million/WaterlooStar-Backend/internal/apierror"
	"github.com/20age1million/WaterlooStar-Backend/internal/auth"
	"github.com/20age1million/WaterlooStar-Backend/internal/db/sqlcgen"
	"github.com/20age1million/WaterlooStar-Backend/internal/httpapi/gen"
)

const (
	maxForumTitle = 200
	maxForumBody  = 8000
	maxForumQuery = 200
)

// The public forum contains plain-text discussions only. Its queries own the
// visibility and ownership filters: deleted posts and suspended authors cannot
// leak through a new handler, and a stale ownership read cannot authorise a write.
func (s *Server) ListForumPosts(ctx context.Context, request gen.ListForumPostsRequestObject) (gen.ListForumPostsResponseObject, error) {
	page, perPage := paginationFrom(request.Params.Page, request.Params.PerPage)
	if int64(page-1) > int64(math.MaxInt32)/int64(perPage) {
		return gen.ListForumPosts400JSONResponse{BadRequestJSONResponse: gen.BadRequestJSONResponse(errorBodyWithDetails(
			apierror.CodeValidation, "Check the highlighted fields.", map[string]string{"page": "Choose an earlier page."}))}, nil
	}
	var search *string
	if request.Params.Q != nil {
		text := strings.TrimSpace(*request.Params.Q)
		if utf8.RuneCountInString(text) > maxForumQuery {
			return gen.ListForumPosts400JSONResponse{BadRequestJSONResponse: gen.BadRequestJSONResponse(errorBodyWithDetails(
				apierror.CodeValidation, "Check the highlighted fields.", map[string]string{"q": "Search must be 200 characters or fewer."}))}, nil
		}
		if text != "" {
			search = &text
		}
	}
	rows, err := s.queries.ListForumPosts(ctx, sqlcgen.ListForumPostsParams{
		Search: search, Limit: int32(perPage), Offset: int32((page - 1) * perPage),
	})
	if err != nil {
		return nil, err
	}
	total, err := s.queries.CountForumPosts(ctx, search)
	if err != nil {
		return nil, err
	}
	posts := make([]gen.ForumPost, 0, len(rows))
	for _, row := range rows {
		posts = append(posts, toForumPost(row.ForumPost, forumAuthor(row.ForumPost.AuthorID, row.AuthorUsername, row.AuthorAvatarUrl, row.AuthorVerified)))
	}
	return gen.ListForumPosts200JSONResponse{Data: posts, Meta: pageMeta(page, perPage, total)}, nil
}

func (s *Server) GetForumPost(ctx context.Context, request gen.GetForumPostRequestObject) (gen.GetForumPostResponseObject, error) {
	row, err := s.queries.GetForumPost(ctx, uuid.UUID(request.Id))
	if errors.Is(err, pgx.ErrNoRows) {
		return gen.GetForumPost404JSONResponse{NotFoundJSONResponse: gen.NotFoundJSONResponse(forumNotFound())}, nil
	}
	if err != nil {
		return nil, err
	}
	return gen.GetForumPost200JSONResponse(toForumPost(row.ForumPost, forumAuthor(row.ForumPost.AuthorID, row.AuthorUsername, row.AuthorAvatarUrl, row.AuthorVerified))), nil
}

func (s *Server) CreateForumPost(ctx context.Context, request gen.CreateForumPostRequestObject) (gen.CreateForumPostResponseObject, error) {
	user, refusal, err := s.forumUser(ctx)
	if err != nil {
		return nil, err
	}
	if refusal != nil {
		if refusal.Code == gen.ErrorCodeUnauthorized {
			return gen.CreateForumPost401JSONResponse{UnauthorizedJSONResponse: gen.UnauthorizedJSONResponse(*refusal)}, nil
		}
		return gen.CreateForumPost403JSONResponse(*refusal), nil
	}
	if request.Body == nil {
		return gen.CreateForumPost400JSONResponse{BadRequestJSONResponse: gen.BadRequestJSONResponse(errorBody(apierror.CodeBadRequest, "A JSON body is required."))}, nil
	}
	title, body := strings.TrimSpace(request.Body.Title), strings.TrimSpace(request.Body.Body)
	problems := map[string]string{}
	validateForumText("title", title, maxForumTitle, problems)
	validateForumText("body", body, maxForumBody, problems)
	if len(problems) > 0 {
		return gen.CreateForumPost400JSONResponse{BadRequestJSONResponse: gen.BadRequestJSONResponse(errorBodyWithDetails(apierror.CodeValidation, "Check the highlighted fields.", problems))}, nil
	}
	post, err := s.queries.CreateForumPost(ctx, sqlcgen.CreateForumPostParams{AuthorID: user.ID, Title: title, Body: body})
	if errors.Is(err, pgx.ErrNoRows) {
		// The SQL repeats the active-user condition, so suspension or removal
		// between the check above and this insert cannot create a post.
		return gen.CreateForumPost403JSONResponse(errorBody(apierror.CodeForbidden, "This account is no longer available to post.")), nil
	}
	if err != nil {
		return nil, err
	}
	return gen.CreateForumPost201JSONResponse(toForumPost(post, forumAuthor(user.ID, user.Username, user.AvatarUrl, user.Verified))), nil
}

func (s *Server) UpdateForumPost(ctx context.Context, request gen.UpdateForumPostRequestObject) (gen.UpdateForumPostResponseObject, error) {
	user, refusal, err := s.forumUser(ctx)
	if err != nil {
		return nil, err
	}
	if refusal != nil {
		if refusal.Code == gen.ErrorCodeUnauthorized {
			return gen.UpdateForumPost401JSONResponse{UnauthorizedJSONResponse: gen.UnauthorizedJSONResponse(*refusal)}, nil
		}
		return gen.UpdateForumPost403JSONResponse(*refusal), nil
	}
	// Refuse someone else's post before checking its submitted fields. The
	// update still includes author_id, deleted_at and active user in SQL.
	existing, err := s.queries.GetForumPost(ctx, uuid.UUID(request.Id))
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && existing.ForumPost.AuthorID != user.ID) {
		return gen.UpdateForumPost404JSONResponse{NotFoundJSONResponse: gen.NotFoundJSONResponse(forumNotFound())}, nil
	}
	if err != nil {
		return nil, err
	}
	if request.Body == nil {
		return gen.UpdateForumPost400JSONResponse{BadRequestJSONResponse: gen.BadRequestJSONResponse(errorBody(apierror.CodeBadRequest, "A JSON body is required."))}, nil
	}
	params := sqlcgen.UpdateForumPostParams{ID: uuid.UUID(request.Id), AuthorID: user.ID}
	problems := map[string]string{}
	if request.Body.Title.IsSpecified() {
		if request.Body.Title.IsNull() {
			problems["title"] = "Enter a title."
		} else {
			value, _ := request.Body.Title.Get()
			value = strings.TrimSpace(value)
			validateForumText("title", value, maxForumTitle, problems)
			params.Title = &value
		}
	}
	if request.Body.Body.IsSpecified() {
		if request.Body.Body.IsNull() {
			problems["body"] = "Write something for your post."
		} else {
			value, _ := request.Body.Body.Get()
			value = strings.TrimSpace(value)
			validateForumText("body", value, maxForumBody, problems)
			params.Body = &value
		}
	}
	if !request.Body.Title.IsSpecified() && !request.Body.Body.IsSpecified() {
		problems["body"] = "Supply a title or body to change."
	}
	if len(problems) > 0 {
		return gen.UpdateForumPost400JSONResponse{BadRequestJSONResponse: gen.BadRequestJSONResponse(errorBodyWithDetails(apierror.CodeValidation, "Check the highlighted fields.", problems))}, nil
	}
	post, err := s.queries.UpdateForumPost(ctx, params)
	if errors.Is(err, pgx.ErrNoRows) {
		return gen.UpdateForumPost404JSONResponse{NotFoundJSONResponse: gen.NotFoundJSONResponse(forumNotFound())}, nil
	}
	if err != nil {
		return nil, err
	}
	return gen.UpdateForumPost200JSONResponse(toForumPost(post, forumAuthor(user.ID, user.Username, user.AvatarUrl, user.Verified))), nil
}

func (s *Server) DeleteForumPost(ctx context.Context, request gen.DeleteForumPostRequestObject) (gen.DeleteForumPostResponseObject, error) {
	user, refusal, err := s.forumUser(ctx)
	if err != nil {
		return nil, err
	}
	if refusal != nil {
		if refusal.Code == gen.ErrorCodeUnauthorized {
			return gen.DeleteForumPost401JSONResponse{UnauthorizedJSONResponse: gen.UnauthorizedJSONResponse(*refusal)}, nil
		}
		return gen.DeleteForumPost403JSONResponse(*refusal), nil
	}
	_, err = s.queries.DeleteForumPost(ctx, sqlcgen.DeleteForumPostParams{ID: uuid.UUID(request.Id), AuthorID: user.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return gen.DeleteForumPost404JSONResponse{NotFoundJSONResponse: gen.NotFoundJSONResponse(forumNotFound())}, nil
	}
	if err != nil {
		return nil, err
	}
	return gen.DeleteForumPost204Response{}, nil
}

// A valid JWT is necessary, but current account state is authoritative. In
// particular an already-issued token must not keep a suspended author posting.
// Verification is deliberately not required for the forum's initial scope.
func (s *Server) forumUser(ctx context.Context) (sqlcgen.User, *gen.Error, error) {
	principal, ok := auth.PrincipalFrom(ctx)
	if !ok {
		body := errorBody(apierror.CodeUnauthorized, "Log in to manage your forum posts.")
		return sqlcgen.User{}, &body, nil
	}
	user, err := s.queries.GetUserByID(ctx, principal.UserID)
	if errors.Is(err, pgx.ErrNoRows) {
		body := errorBody(apierror.CodeUnauthorized, "That account no longer exists. Log in again.")
		return sqlcgen.User{}, &body, nil
	}
	if err != nil {
		return sqlcgen.User{}, nil, err
	}
	if user.SuspendedAt != nil {
		body := errorBody(apierror.CodeForbidden, "This account is suspended and cannot manage forum posts.")
		return sqlcgen.User{}, &body, nil
	}
	return user, nil, nil
}

func validateForumText(field, value string, maximum int, problems map[string]string) {
	length := utf8.RuneCountInString(value)
	if length == 0 {
		if field == "title" {
			problems[field] = "Enter a title."
		} else {
			problems[field] = "Write something for your post."
		}
	} else if length > maximum {
		if field == "title" {
			problems[field] = "Keep the title to 200 characters or fewer."
		} else {
			problems[field] = "Keep the post to 8,000 characters or fewer."
		}
	}
}

func forumNotFound() gen.Error {
	return errorBody(apierror.CodeNotFound, "No forum post with that id.")
}

func forumAuthor(id uuid.UUID, username string, avatar *string, verified bool) gen.ListingOwner {
	return gen.ListingOwner{Id: openapi_types.UUID(id), Username: username, AvatarUrl: nullableString(avatar), Verified: verified}
}

func toForumPost(post sqlcgen.ForumPost, author gen.ListingOwner) gen.ForumPost {
	return gen.ForumPost{Id: openapi_types.UUID(post.ID), Title: post.Title, Body: post.Body, Author: author, CreatedAt: post.CreatedAt, UpdatedAt: post.UpdatedAt}
}
