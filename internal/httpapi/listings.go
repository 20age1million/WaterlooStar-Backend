package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"strings"

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

// ListListings returns a page of published listings matching the filters.
//
// Public: browsing is the first thing a prospective user does, and requiring an
// account to look would defeat the point of the housing hub.
func (s *Server) ListListings(ctx context.Context, request gen.ListListingsRequestObject) (gen.ListListingsResponseObject, error) {
	page, perPage := paginationFrom(request.Params.Page, request.Params.PerPage)

	filters, problem := filtersFrom(request.Params)
	if problem != "" {
		return gen.ListListings400JSONResponse{
			BadRequestJSONResponse: gen.BadRequestJSONResponse(
				errorBody(apierror.CodeValidation, problem)),
		}, nil
	}

	// The count applies the same filters, so the total is the number to show a
	// user rather than the size of the table.
	total, err := s.queries.CountListings(ctx, filters.count())
	if err != nil {
		s.log.Error("count listings", slog.String("error", err.Error()))
		return nil, err
	}

	filters.Limit = int32(perPage)
	filters.Offset = int32((page - 1) * perPage)

	rows, err := s.queries.ListListings(ctx, filters.ListListingsParams)
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

// listingFilters wraps the generated parameters so the same set can be handed to
// both the list and the count without restating it.
type listingFilters struct {
	sqlcgen.ListListingsParams
}

func (f listingFilters) count() sqlcgen.CountListingsParams {
	return sqlcgen.CountListingsParams{
		Search:       f.Search,
		StartAfter:   f.StartAfter,
		EndBefore:    f.EndBefore,
		PriceMin:     f.PriceMin,
		PriceMax:     f.PriceMax,
		DistanceMax:  f.DistanceMax,
		BedroomsMin:  f.BedroomsMin,
		Furnished:    f.Furnished,
		Parking:      f.Parking,
		Pets:         f.Pets,
		Laundry:      f.Laundry,
		Utilities:    f.Utilities,
		VerifiedOnly: f.VerifiedOnly,
	}
}

// filtersFrom maps query parameters onto the query's, returning a message when
// the request asks for something impossible.
func filtersFrom(p gen.ListListingsParams) (listingFilters, string) {
	var f listingFilters

	// An empty or whitespace-only q is no search at all, not a search for "".
	if p.Q != nil {
		if trimmed := strings.TrimSpace(*p.Q); trimmed != "" {
			f.Search = &trimmed
		}
	}

	if p.StartAfter != nil {
		d := p.StartAfter.Time
		f.StartAfter = &d
	}
	if p.EndBefore != nil {
		d := p.EndBefore.Time
		f.EndBefore = &d
	}
	if f.StartAfter != nil && f.EndBefore != nil && f.EndBefore.Before(*f.StartAfter) {
		return f, "The end of the term cannot fall before its start."
	}

	f.PriceMin = int32Ptr(p.PriceMinCents)
	f.PriceMax = int32Ptr(p.PriceMaxCents)
	if f.PriceMin != nil && f.PriceMax != nil && *f.PriceMax < *f.PriceMin {
		return f, "The maximum rent cannot be below the minimum."
	}

	f.DistanceMax = int32Ptr(p.DistanceMaxM)
	f.BedroomsMin = int32Ptr(p.BedroomsMin)
	f.Furnished = p.Furnished
	f.Parking = p.Parking
	f.Pets = p.Pets
	f.Laundry = p.Laundry
	f.VerifiedOnly = p.VerifiedOnly

	if p.Utilities != nil && len(*p.Utilities) > 0 {
		utilities := make([]string, 0, len(*p.Utilities))
		for _, u := range *p.Utilities {
			utilities = append(utilities, string(u))
		}
		f.Utilities = utilities
	}

	// The generated type already constrains this to the enum; default when absent.
	f.Sort = string(gen.New)
	if p.Sort != nil {
		f.Sort = string(*p.Sort)
	}

	return f, ""
}

func int32Ptr(v *int) *int32 {
	if v == nil {
		return nil
	}
	n := int32(*v)
	return &n
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
