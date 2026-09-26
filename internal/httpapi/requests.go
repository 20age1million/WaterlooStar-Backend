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

// The read half of "Looking for Housing".
//
// Public, like listings: an owner with a room to fill should be able to read
// what students need without an account. A request carries no address, so it
// exposes far less than a listing does.

// ListRequests returns a page of published requests matching the filters.
func (s *Server) ListRequests(ctx context.Context, request gen.ListRequestsRequestObject) (gen.ListRequestsResponseObject, error) {
	page, perPage := paginationFrom(request.Params.Page, request.Params.PerPage)

	filters, problem := requestFiltersFrom(request.Params)
	if problem != "" {
		return gen.ListRequests400JSONResponse{
			BadRequestJSONResponse: gen.BadRequestJSONResponse(
				errorBody(apierror.CodeValidation, problem)),
		}, nil
	}

	// The count applies the same filters, so the total describes the whole
	// matching set rather than the size of the table.
	total, err := s.queries.CountRequests(ctx, filters.count())
	if err != nil {
		s.log.Error("count requests", slog.String("error", err.Error()))
		return nil, err
	}

	filters.Limit = int32(perPage)
	filters.Offset = int32((page - 1) * perPage)

	rows, err := s.queries.ListRequests(ctx, filters.ListRequestsParams)
	if err != nil {
		s.log.Error("list requests", slog.String("error", err.Error()))
		return nil, err
	}

	data := make([]gen.HousingRequest, 0, len(rows))
	for _, row := range rows {
		data = append(data, toRequest(row.HousingRequest, gen.ListingOwner{
			Id:        openapi_types.UUID(row.HousingRequest.PosterID),
			Username:  row.PosterUsername,
			AvatarUrl: nullableString(row.PosterAvatarUrl),
			Verified:  row.PosterVerified,
		}, row.OfferCount))
	}

	return gen.ListRequests200JSONResponse{
		Data: data,
		Meta: pageMeta(page, perPage, total),
	}, nil
}

// requestFilters wraps the generated parameters so the same set can be handed
// to both the list and the count without restating it.
type requestFilters struct {
	sqlcgen.ListRequestsParams
}

func (f requestFilters) count() sqlcgen.CountRequestsParams {
	return sqlcgen.CountRequestsParams{
		Search:       f.Search,
		StartAfter:   f.StartAfter,
		EndBefore:    f.EndBefore,
		BudgetMin:    f.BudgetMin,
		BudgetMax:    f.BudgetMax,
		DistanceMin:  f.DistanceMin,
		OccupantsMax: f.OccupantsMax,
		Pets:         f.Pets,
		Furnished:    f.Furnished,
		Parking:      f.Parking,
		Laundry:      f.Laundry,
		VerifiedOnly: f.VerifiedOnly,
	}
}

// requestFiltersFrom maps query parameters onto the query's, returning a
// message when the request asks for something impossible.
func requestFiltersFrom(p gen.ListRequestsParams) (requestFilters, string) {
	var f requestFilters

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

	f.BudgetMin = int32Ptr(p.BudgetMinCents)
	f.BudgetMax = int32Ptr(p.BudgetMaxCents)
	if f.BudgetMin != nil && f.BudgetMax != nil && *f.BudgetMax < *f.BudgetMin {
		return f, "The maximum budget cannot be below the minimum."
	}

	f.DistanceMin = int32Ptr(p.DistanceMinM)
	f.OccupantsMax = int32Ptr(p.OccupantsMax)
	f.Pets = p.Pets
	f.Furnished = p.Furnished
	f.Parking = p.Parking
	f.Laundry = p.Laundry
	f.VerifiedOnly = p.VerifiedOnly

	// The generated type already constrains this to the enum; default when absent.
	f.Sort = string(gen.ListRequestsParamsSortNew)
	if p.Sort != nil {
		f.Sort = string(*p.Sort)
	}

	return f, ""
}

// GetRequest returns one published request.
func (s *Server) GetRequest(ctx context.Context, request gen.GetRequestRequestObject) (gen.GetRequestResponseObject, error) {
	row, err := s.queries.GetPublishedRequest(ctx, uuid.UUID(request.Id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// A draft, paused or taken-down request answers the same as one that
			// never existed.
			return gen.GetRequest404JSONResponse{
				NotFoundJSONResponse: gen.NotFoundJSONResponse(
					errorBody(apierror.CodeNotFound, "No request with that id.")),
			}, nil
		}
		s.log.Error("get request", slog.String("error", err.Error()))
		return nil, err
	}

	return gen.GetRequest200JSONResponse(toRequest(row.HousingRequest, gen.ListingOwner{
		Id:        openapi_types.UUID(row.HousingRequest.PosterID),
		Username:  row.PosterUsername,
		AvatarUrl: nullableString(row.PosterAvatarUrl),
		Verified:  row.PosterVerified,
	}, row.OfferCount)), nil
}
