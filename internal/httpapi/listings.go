package httpapi

import (
	"context"
	"errors"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/20age1million/WaterlooStar-Backend/internal/apierror"
	"github.com/20age1million/WaterlooStar-Backend/internal/db/sqlcgen"
	"github.com/20age1million/WaterlooStar-Backend/internal/httpapi/gen"
)

// Pagination bounds. The maximum is a hard cap rather than an error: a client
// asking for 5,000 rows gets 100, not a rejection, because refusing the request
// helps nobody.
const (
	defaultPerPage = 20
	maxPerPage     = 100
)

// ListListings returns a page of published listings, newest first.
//
// Public: browsing is the first thing a prospective user does, and requiring an
// account to look would defeat the point of the housing hub.
func (s *Server) ListListings(ctx context.Context, request gen.ListListingsRequestObject) (gen.ListListingsResponseObject, error) {
	page, perPage := paginationFrom(request.Params.Page, request.Params.PerPage)

	total, err := s.queries.CountPublishedListings(ctx)
	if err != nil {
		s.log.Error("count listings", slog.String("error", err.Error()))
		return nil, err
	}

	rows, err := s.queries.ListPublishedListings(ctx, sqlcgen.ListPublishedListingsParams{
		Limit:  int32(perPage),
		Offset: int32((page - 1) * perPage),
	})
	if err != nil {
		s.log.Error("list listings", slog.String("error", err.Error()))
		return nil, err
	}

	// One query for every listing's photos rather than one per listing.
	ids := make([]uuid.UUID, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.Listing.ID)
	}
	photos, err := s.photosByListing(ctx, ids)
	if err != nil {
		s.log.Error("list listing photos", slog.String("error", err.Error()))
		return nil, err
	}

	data := make([]gen.Listing, 0, len(rows))
	for _, row := range rows {
		data = append(data, toListing(
			row.Listing,
			gen.ListingOwner{
				Id:        openapi_types.UUID(row.Listing.OwnerID),
				Username:  row.OwnerUsername,
				AvatarUrl: nullableString(row.OwnerAvatarUrl),
				Verified:  row.OwnerVerified,
			},
			photos[row.Listing.ID],
		))
	}

	return gen.ListListings200JSONResponse{
		Data: data,
		Meta: pageMeta(page, perPage, total),
	}, nil
}

// GetListing returns one published listing in full.
func (s *Server) GetListing(ctx context.Context, request gen.GetListingRequestObject) (gen.GetListingResponseObject, error) {
	row, err := s.queries.GetPublishedListing(ctx, uuid.UUID(request.Id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// A paused or archived listing answers the same as one that never
			// existed: whether a post was taken down is nobody else's business.
			return gen.GetListing404JSONResponse{
				NotFoundJSONResponse: gen.NotFoundJSONResponse(
					errorBody(apierror.CodeNotFound, "No listing with that id.")),
			}, nil
		}
		s.log.Error("get listing", slog.String("error", err.Error()))
		return nil, err
	}

	photos, err := s.queries.ListPhotosForListing(ctx, row.Listing.ID)
	if err != nil {
		s.log.Error("list listing photos", slog.String("error", err.Error()))
		return nil, err
	}

	return gen.GetListing200JSONResponse(toListing(
		row.Listing,
		gen.ListingOwner{
			Id:        openapi_types.UUID(row.Listing.OwnerID),
			Username:  row.OwnerUsername,
			AvatarUrl: nullableString(row.OwnerAvatarUrl),
			Verified:  row.OwnerVerified,
		},
		photos,
	)), nil
}

// photosByListing groups photos by their listing, so the list handler can attach
// them without a query each.
func (s *Server) photosByListing(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID][]sqlcgen.ListingPhoto, error) {
	grouped := map[uuid.UUID][]sqlcgen.ListingPhoto{}
	if len(ids) == 0 {
		return grouped, nil
	}

	rows, err := s.queries.ListPhotosForListings(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, photo := range rows {
		grouped[photo.ListingID] = append(grouped[photo.ListingID], photo)
	}
	return grouped, nil
}

// paginationFrom applies the defaults and the hard cap.
func paginationFrom(page, perPage *int) (int, int) {
	p := defaultIfNil(page, 1)
	if p < 1 {
		p = 1
	}

	size := defaultIfNil(perPage, defaultPerPage)
	if size < 1 {
		size = defaultPerPage
	}
	if size > maxPerPage {
		size = maxPerPage
	}

	return p, size
}

func defaultIfNil(value *int, fallback int) int {
	if value == nil {
		return fallback
	}
	return *value
}

// pageMeta reports the page size actually used, not the one requested, so a
// client can tell its value was clamped.
func pageMeta(page, perPage int, total int64) gen.PageMeta {
	totalPages := 0
	if total > 0 {
		totalPages = int((total + int64(perPage) - 1) / int64(perPage))
	}
	return gen.PageMeta{
		Page:       page,
		PerPage:    perPage,
		Total:      int(total),
		TotalPages: totalPages,
	}
}
