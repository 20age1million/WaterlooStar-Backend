package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/oapi-codegen/nullable"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/20age1million/WaterlooStar-Backend/internal/apierror"
	"github.com/20age1million/WaterlooStar-Backend/internal/db"
	"github.com/20age1million/WaterlooStar-Backend/internal/db/sqlcgen"
	"github.com/20age1million/WaterlooStar-Backend/internal/httpapi/gen"
)

// Post moderation: every listing and request in any state, and takedown. The
// rules live in internal/db (RemovePost, RestorePost); this file reads, checks
// reasons and says what happened.

func adminPostOwner(id uuid.UUID, username, email string, verified bool, suspendedAt *time.Time) gen.AdminPostOwner {
	o := gen.AdminPostOwner{
		Id: openapi_types.UUID(id), Username: username, Email: openapi_types.Email(email),
		Verified: verified, SuspendedAt: nullable.NewNullNullable[time.Time](),
	}
	if suspendedAt != nil {
		o.SuspendedAt = nullable.NewNullableWithValue(*suspendedAt)
	}
	return o
}

func trimmedOrNil(q *string) *string {
	if q == nil {
		return nil
	}
	if t := strings.TrimSpace(*q); t != "" {
		return &t
	}
	return nil
}

// ListAdminListings returns every listing matching the filters, in any state.
func (s *Server) ListAdminListings(ctx context.Context, request gen.ListAdminListingsRequestObject) (gen.ListAdminListingsResponseObject, error) {
	if _, ok := requireAdmin(ctx); !ok {
		return unrouted{}, nil
	}
	p := request.Params
	page, perPage := paginationFrom(p.Page, p.PerPage)

	filters := sqlcgen.CountListingsForAdminParams{Search: trimmedOrNil(p.Q), Removed: p.Removed}
	if p.Status != nil {
		st := string(*p.Status)
		filters.Status = &st
	}
	if p.OwnerId != nil {
		id := uuid.UUID(*p.OwnerId)
		filters.OwnerID = &id
	}

	total, err := s.queries.CountListingsForAdmin(ctx, filters)
	if err != nil {
		s.log.Error("count listings for admin", slog.String("error", err.Error()))
		return nil, err
	}
	rows, err := s.queries.ListListingsForAdmin(ctx, sqlcgen.ListListingsForAdminParams{
		Search: filters.Search, Status: filters.Status, Removed: filters.Removed, OwnerID: filters.OwnerID,
		Limit: int32(perPage), Offset: int32((page - 1) * perPage),
	})
	if err != nil {
		s.log.Error("list listings for admin", slog.String("error", err.Error()))
		return nil, err
	}

	ids := make([]uuid.UUID, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.Listing.ID)
	}
	photos, err := s.photosByListing(ctx, ids)
	if err != nil {
		s.log.Error("list photos for admin", slog.String("error", err.Error()))
		return nil, err
	}

	data := make([]gen.AdminListing, 0, len(rows))
	for _, r := range rows {
		data = append(data, gen.AdminListing{
			Listing: toListing(r.Listing, gen.ListingOwner{
				Id: openapi_types.UUID(r.Listing.OwnerID), Username: r.OwnerUsername,
				Verified: r.OwnerVerified, AvatarUrl: nullableString(r.OwnerAvatarUrl),
			}, photos[r.Listing.ID]),
			Owner: adminPostOwner(r.Listing.OwnerID, r.OwnerUsername, r.OwnerEmail, r.OwnerVerified, r.OwnerSuspendedAt),
		})
	}
	return gen.ListAdminListings200JSONResponse{Data: data, Meta: pageMeta(page, perPage, total)}, nil
}

// ListAdminRequests returns every request matching the filters, in any state.
func (s *Server) ListAdminRequests(ctx context.Context, request gen.ListAdminRequestsRequestObject) (gen.ListAdminRequestsResponseObject, error) {
	if _, ok := requireAdmin(ctx); !ok {
		return unrouted{}, nil
	}
	p := request.Params
	page, perPage := paginationFrom(p.Page, p.PerPage)

	filters := sqlcgen.CountRequestsForAdminParams{Search: trimmedOrNil(p.Q), Removed: p.Removed}
	if p.Status != nil {
		st := string(*p.Status)
		filters.Status = &st
	}
	if p.OwnerId != nil {
		id := uuid.UUID(*p.OwnerId)
		filters.OwnerID = &id
	}

	total, err := s.queries.CountRequestsForAdmin(ctx, filters)
	if err != nil {
		s.log.Error("count requests for admin", slog.String("error", err.Error()))
		return nil, err
	}
	rows, err := s.queries.ListRequestsForAdmin(ctx, sqlcgen.ListRequestsForAdminParams{
		Search: filters.Search, Status: filters.Status, Removed: filters.Removed, OwnerID: filters.OwnerID,
		Limit: int32(perPage), Offset: int32((page - 1) * perPage),
	})
	if err != nil {
		s.log.Error("list requests for admin", slog.String("error", err.Error()))
		return nil, err
	}

	data := make([]gen.AdminRequest, 0, len(rows))
	for _, r := range rows {
		data = append(data, gen.AdminRequest{
			Request: toRequest(r.HousingRequest, gen.ListingOwner{
				Id: openapi_types.UUID(r.HousingRequest.PosterID), Username: r.PosterUsername,
				Verified: r.PosterVerified, AvatarUrl: nullableString(r.PosterAvatarUrl),
			}, r.OfferCount),
			Owner: adminPostOwner(r.HousingRequest.PosterID, r.PosterUsername, r.PosterEmail, r.PosterVerified, r.PosterSuspendedAt),
		})
	}
	return gen.ListAdminRequests200JSONResponse{Data: data, Meta: pageMeta(page, perPage, total)}, nil
}

// adminListing reads one listing as the admin surface shows it.
func (s *Server) adminListing(ctx context.Context, id uuid.UUID) (gen.AdminListing, error) {
	row, err := s.queries.GetListingForOwner(ctx, id)
	if err != nil {
		return gen.AdminListing{}, err
	}
	owner, err := s.queries.GetUserByID(ctx, row.OwnerID)
	if err != nil {
		return gen.AdminListing{}, err
	}
	photos, err := s.queries.ListPhotosForListing(ctx, id)
	if err != nil {
		return gen.AdminListing{}, err
	}
	return gen.AdminListing{
		Listing: toListing(row, gen.ListingOwner{
			Id: openapi_types.UUID(owner.ID), Username: owner.Username,
			Verified: owner.Verified, AvatarUrl: nullableString(owner.AvatarUrl),
		}, photos),
		Owner: adminPostOwner(owner.ID, owner.Username, owner.Email, owner.Verified, owner.SuspendedAt),
	}, nil
}

// adminRequest reads one request as the admin surface shows it.
func (s *Server) adminRequest(ctx context.Context, id uuid.UUID) (gen.AdminRequest, error) {
	row, err := s.queries.GetRequestForOwner(ctx, id)
	if err != nil {
		return gen.AdminRequest{}, err
	}
	poster, err := s.queries.GetUserByID(ctx, row.PosterID)
	if err != nil {
		return gen.AdminRequest{}, err
	}
	offers, err := s.queries.CountOffersForRequest(ctx, id)
	if err != nil {
		return gen.AdminRequest{}, err
	}
	return gen.AdminRequest{
		Request: toRequest(row, gen.ListingOwner{
			Id: openapi_types.UUID(poster.ID), Username: poster.Username,
			Verified: poster.Verified, AvatarUrl: nullableString(poster.AvatarUrl),
		}, offers),
		Owner: adminPostOwner(poster.ID, poster.Username, poster.Email, poster.Verified, poster.SuspendedAt),
	}, nil
}

// postOutcome is a takedown or restore before it is fitted to its operation's
// response types. status is 200, 400, 404 or 409; on 200 the caller reads the
// post back.
type postOutcome struct {
	status int
	body   gen.Error
}

// postAction checks the reason (required to remove, optional to restore), runs
// act and maps its errors.
func (s *Server) postAction(kind string, reason *string, required bool, act func(reason string) error) (postOutcome, error) {
	text := ""
	if reason != nil {
		text = strings.TrimSpace(*reason)
	}
	switch n := utf8.RuneCountInString(text); {
	case required && reason == nil:
		return postOutcome{status: statusBadRequest, body: errorBody(apierror.CodeBadRequest, "A JSON body is required.")}, nil
	case required && n < minReasonLength:
		return postOutcome{status: statusBadRequest, body: errorBodyWithDetails(apierror.CodeValidation,
			"Give a reason. The owner will see it.", map[string]string{"reason": "At least 10 characters."})}, nil
	case n > 1000:
		return postOutcome{status: statusBadRequest, body: errorBodyWithDetails(apierror.CodeValidation,
			"That reason is too long.", map[string]string{"reason": "At most 1,000 characters."})}, nil
	}

	err := act(text)
	switch {
	case err == nil:
		return postOutcome{status: 200}, nil
	case errors.Is(err, pgx.ErrNoRows):
		return postOutcome{status: 404, body: errorBody(apierror.CodeNotFound, "No "+kind+" with that id.")}, nil
	case errors.Is(err, db.ErrAlreadyRemoved):
		return postOutcome{status: statusConflict, body: errorBody(apierror.CodeConflict, "That "+kind+" is already taken down.")}, nil
	case errors.Is(err, db.ErrNotRemoved):
		return postOutcome{status: statusConflict, body: errorBody(apierror.CodeConflict, "That "+kind+" is not taken down.")}, nil
	default:
		s.log.Error("admin post action", slog.String("kind", kind), slog.String("error", err.Error()))
		return postOutcome{}, err
	}
}

func optionalReason(body *gen.AdminOptionalReason) *string {
	if body == nil {
		return nil
	}
	return body.Reason
}

// RemoveAdminListing takes a listing down.
func (s *Server) RemoveAdminListing(ctx context.Context, request gen.RemoveAdminListingRequestObject) (gen.RemoveAdminListingResponseObject, error) {
	principal, ok := requireAdmin(ctx)
	if !ok {
		return unrouted{}, nil
	}
	id := uuid.UUID(request.Id)
	o, err := s.postAction("listing", reasonFrom(request.Body), true, func(reason string) error {
		return db.RemovePost(ctx, s.tx, principal.UserID, db.SubjectListing, id, reason)
	})
	if err != nil {
		return nil, err
	}
	switch o.status {
	case 200:
		out, err := s.adminListing(ctx, id)
		if err != nil {
			return nil, err
		}
		return gen.RemoveAdminListing200JSONResponse(out), nil
	case statusBadRequest:
		return gen.RemoveAdminListing400JSONResponse{BadRequestJSONResponse: gen.BadRequestJSONResponse(o.body)}, nil
	case 404:
		return gen.RemoveAdminListing404JSONResponse{NotFoundJSONResponse: gen.NotFoundJSONResponse(o.body)}, nil
	default:
		return gen.RemoveAdminListing409JSONResponse(o.body), nil
	}
}

// RestoreAdminListing lifts a listing's takedown.
func (s *Server) RestoreAdminListing(ctx context.Context, request gen.RestoreAdminListingRequestObject) (gen.RestoreAdminListingResponseObject, error) {
	principal, ok := requireAdmin(ctx)
	if !ok {
		return unrouted{}, nil
	}
	id := uuid.UUID(request.Id)
	o, err := s.postAction("listing", optionalReason(request.Body), false, func(reason string) error {
		return db.RestorePost(ctx, s.tx, principal.UserID, db.SubjectListing, id, reason)
	})
	if err != nil {
		return nil, err
	}
	switch o.status {
	case 200:
		out, err := s.adminListing(ctx, id)
		if err != nil {
			return nil, err
		}
		return gen.RestoreAdminListing200JSONResponse(out), nil
	case statusBadRequest:
		return gen.RestoreAdminListing400JSONResponse{BadRequestJSONResponse: gen.BadRequestJSONResponse(o.body)}, nil
	case 404:
		return gen.RestoreAdminListing404JSONResponse{NotFoundJSONResponse: gen.NotFoundJSONResponse(o.body)}, nil
	default:
		return gen.RestoreAdminListing409JSONResponse(o.body), nil
	}
}

// RemoveAdminRequest takes a request down.
func (s *Server) RemoveAdminRequest(ctx context.Context, request gen.RemoveAdminRequestRequestObject) (gen.RemoveAdminRequestResponseObject, error) {
	principal, ok := requireAdmin(ctx)
	if !ok {
		return unrouted{}, nil
	}
	id := uuid.UUID(request.Id)
	o, err := s.postAction("request", reasonFrom(request.Body), true, func(reason string) error {
		return db.RemovePost(ctx, s.tx, principal.UserID, db.SubjectRequest, id, reason)
	})
	if err != nil {
		return nil, err
	}
	switch o.status {
	case 200:
		out, err := s.adminRequest(ctx, id)
		if err != nil {
			return nil, err
		}
		return gen.RemoveAdminRequest200JSONResponse(out), nil
	case statusBadRequest:
		return gen.RemoveAdminRequest400JSONResponse{BadRequestJSONResponse: gen.BadRequestJSONResponse(o.body)}, nil
	case 404:
		return gen.RemoveAdminRequest404JSONResponse{NotFoundJSONResponse: gen.NotFoundJSONResponse(o.body)}, nil
	default:
		return gen.RemoveAdminRequest409JSONResponse(o.body), nil
	}
}

// RestoreAdminRequest lifts a request's takedown.
func (s *Server) RestoreAdminRequest(ctx context.Context, request gen.RestoreAdminRequestRequestObject) (gen.RestoreAdminRequestResponseObject, error) {
	principal, ok := requireAdmin(ctx)
	if !ok {
		return unrouted{}, nil
	}
	id := uuid.UUID(request.Id)
	o, err := s.postAction("request", optionalReason(request.Body), false, func(reason string) error {
		return db.RestorePost(ctx, s.tx, principal.UserID, db.SubjectRequest, id, reason)
	})
	if err != nil {
		return nil, err
	}
	switch o.status {
	case 200:
		out, err := s.adminRequest(ctx, id)
		if err != nil {
			return nil, err
		}
		return gen.RestoreAdminRequest200JSONResponse(out), nil
	case statusBadRequest:
		return gen.RestoreAdminRequest400JSONResponse{BadRequestJSONResponse: gen.BadRequestJSONResponse(o.body)}, nil
	case 404:
		return gen.RestoreAdminRequest404JSONResponse{NotFoundJSONResponse: gen.NotFoundJSONResponse(o.body)}, nil
	default:
		return gen.RestoreAdminRequest409JSONResponse(o.body), nil
	}
}
