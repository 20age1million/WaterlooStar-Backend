package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/20age1million/WaterlooStar-Backend/internal/apierror"
	"github.com/20age1million/WaterlooStar-Backend/internal/auth"
	"github.com/20age1million/WaterlooStar-Backend/internal/db/sqlcgen"
	"github.com/20age1million/WaterlooStar-Backend/internal/httpapi/gen"
)

// The write half of Housing Available.
//
// Two rules run through all of it:
//
//   - Only a verified student may post. Confirming a uwaterloo.ca address is
//     what the badge on every listing means, so it gates creation.
//   - Someone else's listing answers 404, never 403. A 403 would confirm the
//     listing exists to a person with no business knowing.

const maxConditions = 12

// ------------------------------------------------------------------- create

func (s *Server) CreateListing(ctx context.Context, request gen.CreateListingRequestObject) (gen.CreateListingResponseObject, error) {
	principal, ok := auth.PrincipalFrom(ctx)
	if !ok {
		return gen.CreateListing401JSONResponse{
			UnauthorizedJSONResponse: gen.UnauthorizedJSONResponse(
				errorBody(apierror.CodeUnauthorized, "Log in to post a listing.")),
		}, nil
	}
	if !principal.Verified {
		return gen.CreateListing403JSONResponse(errorBody(apierror.CodeForbidden,
			"Confirm your uwaterloo.ca address before posting. That badge is what tells other students who they are dealing with.")), nil
	}
	if request.Body == nil {
		return gen.CreateListing400JSONResponse{
			BadRequestJSONResponse: gen.BadRequestJSONResponse(
				errorBody(apierror.CodeBadRequest, "A JSON body is required.")),
		}, nil
	}

	in := *request.Body
	params := sqlcgen.CreateListingParams{
		OwnerID:       principal.UserID,
		Title:         strings.TrimSpace(in.Title),
		Body:          strings.TrimSpace(derefString(in.Body)),
		Conditions:    trimAll(derefStrings(in.Conditions)),
		PriceCents:    int32(in.PriceCents),
		DepositCents:  int32Ptr(nullableToPtrInt(in.DepositCents)),
		StartDate:     in.StartDate.Time,
		EndDate:       in.EndDate.Time,
		LeaseMonths:   int32(in.LeaseMonths),
		TermTag:       strings.TrimSpace(in.TermTag),
		UnitType:      string(in.UnitType),
		BedroomsTotal: int32(in.BedroomsTotal),
		BedroomOf:     int32Ptr(nullableToPtrInt(in.BedroomOf)),
		Bathrooms:     float64(in.Bathrooms),
		BathType:      string(in.BathType),
		Furnished:     derefBool(in.Furnished),
		Utilities:     utilityStrings(in.Utilities),
		Parking:       derefBool(in.Parking),
		Pets:          derefBool(in.Pets),
		Laundry:       derefBool(in.Laundry),
		AddressLine:   strings.TrimSpace(in.AddressLine),
		Neighbourhood: strings.TrimSpace(derefString(in.Neighbourhood)),
		DistanceM:     int32Ptr(nullableToPtrInt(in.DistanceM)),
		CommuteMode:   "walk",
		Status:        "published",
		CreatedAt:     time.Now(),
	}
	if in.Status != nil {
		params.Status = string(*in.Status)
	}

	if problems := validateListing(params); len(problems) > 0 {
		return gen.CreateListing400JSONResponse{
			BadRequestJSONResponse: gen.BadRequestJSONResponse(errorBodyWithDetails(
				apierror.CodeValidation, "Check the highlighted fields.", problems)),
		}, nil
	}

	listing, err := s.queries.CreateListing(ctx, params)
	if err != nil {
		s.log.Error("create listing", slog.String("error", err.Error()))
		return nil, err
	}

	s.log.Info("listing created",
		slog.String("listing_id", listing.ID.String()),
		slog.String("owner_id", principal.UserID.String()))

	return gen.CreateListing201JSONResponse(s.listingResponse(ctx, listing)), nil
}

// ------------------------------------------------------------------- update

func (s *Server) UpdateListing(ctx context.Context, request gen.UpdateListingRequestObject) (gen.UpdateListingResponseObject, error) {
	_, existing, resp := s.ownedListing(ctx, uuid.UUID(request.Id))
	if resp != nil {
		return updateRefusal(resp), nil
	}

	if request.Body == nil {
		return gen.UpdateListing400JSONResponse{
			BadRequestJSONResponse: gen.BadRequestJSONResponse(
				errorBody(apierror.CodeBadRequest, "A JSON body is required.")),
		}, nil
	}
	in := *request.Body

	// Validate the listing as it would be *after* the edit, not the patch in
	// isolation: moving only the end date can still invert the term.
	merged := existing
	applyUpdate(&merged, in)
	if problems := validateListingRow(merged); len(problems) > 0 {
		return gen.UpdateListing400JSONResponse{
			BadRequestJSONResponse: gen.BadRequestJSONResponse(errorBodyWithDetails(
				apierror.CodeValidation, "Check the highlighted fields.", problems)),
		}, nil
	}

	updated, err := s.queries.UpdateListing(ctx, updateParams(existing.ID, in))
	if err != nil {
		s.log.Error("update listing", slog.String("error", err.Error()))
		return nil, err
	}

	return gen.UpdateListing200JSONResponse(s.listingResponse(ctx, updated)), nil
}

// ------------------------------------------------------------------- status

// allowedTransitions is the lifecycle. archived has no entry: it is terminal,
// because a post that has been taken down stays down, and bringing one back
// means posting again — by which time the term and the rent have changed.
var allowedTransitions = map[string][]string{
	"draft":     {"published", "archived"},
	"published": {"paused", "archived"},
	"paused":    {"published", "archived"},
}

func (s *Server) SetListingStatus(ctx context.Context, request gen.SetListingStatusRequestObject) (gen.SetListingStatusResponseObject, error) {
	_, existing, resp := s.ownedListing(ctx, uuid.UUID(request.Id))
	if resp != nil {
		return statusRefusal(resp), nil
	}
	if request.Body == nil {
		return gen.SetListingStatus400JSONResponse(
			errorBody(apierror.CodeBadRequest, "A status is required.")), nil
	}

	want := string(request.Body.Status)
	if want == existing.Status {
		// Idempotent: asking for the state it is already in is not an error.
		return gen.SetListingStatus200JSONResponse(s.listingResponse(ctx, existing)), nil
	}

	allowed := allowedTransitions[existing.Status]
	if !contains(allowed, want) {
		message := "A listing cannot go from " + existing.Status + " to " + want + "."
		if existing.Status == "archived" {
			message = "This listing is archived, and that is final. Post a new one instead — the term and the rent will have moved on."
		}
		return gen.SetListingStatus400JSONResponse(
			errorBody(apierror.CodeBadRequest, message)), nil
	}

	updated, err := s.queries.SetListingStatus(ctx, sqlcgen.SetListingStatusParams{
		ID: existing.ID, Status: want,
	})
	if err != nil {
		s.log.Error("set listing status", slog.String("error", err.Error()))
		return nil, err
	}

	s.log.Info("listing status changed",
		slog.String("listing_id", existing.ID.String()),
		slog.String("from", existing.Status), slog.String("to", want))

	return gen.SetListingStatus200JSONResponse(s.listingResponse(ctx, updated)), nil
}

// ------------------------------------------------------------------- delete

func (s *Server) DeleteListing(ctx context.Context, request gen.DeleteListingRequestObject) (gen.DeleteListingResponseObject, error) {
	_, existing, resp := s.ownedListing(ctx, uuid.UUID(request.Id))
	if resp != nil {
		return deleteRefusal(resp), nil
	}

	// Archive rather than remove. Questions asked on this listing — and later,
	// saves and conversations — reference the row; deleting it would rewrite
	// other people's history.
	if existing.Status != "archived" {
		if _, err := s.queries.SetListingStatus(ctx, sqlcgen.SetListingStatusParams{
			ID: existing.ID, Status: "archived",
		}); err != nil {
			s.log.Error("archive listing", slog.String("error", err.Error()))
			return nil, err
		}
	}

	return gen.DeleteListing204Response{}, nil
}

// ------------------------------------------------------------- my listings

func (s *Server) ListMyListings(ctx context.Context, _ gen.ListMyListingsRequestObject) (gen.ListMyListingsResponseObject, error) {
	principal, ok := auth.PrincipalFrom(ctx)
	if !ok {
		return gen.ListMyListings401JSONResponse{
			UnauthorizedJSONResponse: gen.UnauthorizedJSONResponse(
				errorBody(apierror.CodeUnauthorized, "Log in to see your listings.")),
		}, nil
	}

	rows, err := s.queries.ListListingsByOwner(ctx, principal.UserID)
	if err != nil {
		s.log.Error("list own listings", slog.String("error", err.Error()))
		return nil, err
	}

	out := make([]gen.Listing, 0, len(rows))
	for _, row := range rows {
		out = append(out, toListing(row.Listing, gen.ListingOwner{
			Id:        openapi_types.UUID(row.Listing.OwnerID),
			Username:  row.OwnerUsername,
			AvatarUrl: nullableString(row.OwnerAvatarUrl),
			Verified:  row.OwnerVerified,
		}, nil))
	}

	return gen.ListMyListings200JSONResponse(out), nil
}

// --------------------------------------------------------------- shared bits

// refusal says which failure occurred without leaking anything.
type refusal struct {
	unauthorised bool
	message      string
}

// ownedListing resolves a listing the caller owns, or the refusal to return.
//
// "Not yours" and "does not exist" are deliberately the same answer: a 403 on
// someone else's listing would confirm that it exists.
func (s *Server) ownedListing(ctx context.Context, id uuid.UUID) (auth.Principal, sqlcgen.Listing, *refusal) {
	principal, ok := auth.PrincipalFrom(ctx)
	if !ok {
		return principal, sqlcgen.Listing{}, &refusal{unauthorised: true, message: "Log in to manage your listings."}
	}

	listing, err := s.queries.GetListingForOwner(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return principal, sqlcgen.Listing{}, &refusal{message: "No listing with that id."}
		}
		s.log.Error("load listing for owner", slog.String("error", err.Error()))
		return principal, sqlcgen.Listing{}, &refusal{message: "No listing with that id."}
	}

	if listing.OwnerID != principal.UserID {
		return principal, sqlcgen.Listing{}, &refusal{message: "No listing with that id."}
	}

	return principal, listing, nil
}

// listingResponse attaches the owner summary and photos to a row.
func (s *Server) listingResponse(ctx context.Context, row sqlcgen.Listing) gen.Listing {
	owner := gen.ListingOwner{Id: openapi_types.UUID(row.OwnerID)}
	if u, err := s.queries.GetUserByID(ctx, row.OwnerID); err == nil {
		owner.Username = u.Username
		owner.Verified = u.Verified
		owner.AvatarUrl = nullableString(u.AvatarUrl)
	}

	photos, err := s.queries.ListPhotosForListing(ctx, row.ID)
	if err != nil {
		photos = nil
	}
	return toListing(row, owner, photos)
}

func contains(haystack []string, needle string) bool {
	for _, v := range haystack {
		if v == needle {
			return true
		}
	}
	return false
}

// Each operation has its own generated response types, so the same refusal has
// to be expressed once per operation. The mapping is uniform: no session gives
// 401, anything else gives 404 — including "this is not yours".

func updateRefusal(r *refusal) gen.UpdateListingResponseObject {
	if r.unauthorised {
		return gen.UpdateListing401JSONResponse{
			UnauthorizedJSONResponse: gen.UnauthorizedJSONResponse(
				errorBody(apierror.CodeUnauthorized, r.message)),
		}
	}
	return gen.UpdateListing404JSONResponse{
		NotFoundJSONResponse: gen.NotFoundJSONResponse(
			errorBody(apierror.CodeNotFound, r.message)),
	}
}

func statusRefusal(r *refusal) gen.SetListingStatusResponseObject {
	if r.unauthorised {
		return gen.SetListingStatus401JSONResponse{
			UnauthorizedJSONResponse: gen.UnauthorizedJSONResponse(
				errorBody(apierror.CodeUnauthorized, r.message)),
		}
	}
	return gen.SetListingStatus404JSONResponse{
		NotFoundJSONResponse: gen.NotFoundJSONResponse(
			errorBody(apierror.CodeNotFound, r.message)),
	}
}

func deleteRefusal(r *refusal) gen.DeleteListingResponseObject {
	if r.unauthorised {
		return gen.DeleteListing401JSONResponse{
			UnauthorizedJSONResponse: gen.UnauthorizedJSONResponse(
				errorBody(apierror.CodeUnauthorized, r.message)),
		}
	}
	return gen.DeleteListing404JSONResponse{
		NotFoundJSONResponse: gen.NotFoundJSONResponse(
			errorBody(apierror.CodeNotFound, r.message)),
	}
}
