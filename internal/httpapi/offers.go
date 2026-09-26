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
	"github.com/20age1million/WaterlooStar-Backend/internal/auth"
	"github.com/20age1million/WaterlooStar-Backend/internal/db/sqlcgen"
	"github.com/20age1million/WaterlooStar-Backend/internal/httpapi/gen"
)

// Offers: an owner answering a request with one of their own listings.
//
// Three rules shape this file:
//
//   - An offer points at a listing the offerer owns and has published. It is
//     never free text about a place that may not exist.
//   - The student who posted the request sees every live offer on it; an owner
//     sees only their own. Owners reading each other's answers would turn a
//     request into an auction.
//   - An offer is only as alive as its listing. Taking the place down withdraws
//     the offer, which is why nothing here has to remember to.

const maxNoteLength = 1000

// ------------------------------------------------------------------- create

func (s *Server) CreateOffer(ctx context.Context, request gen.CreateOfferRequestObject) (gen.CreateOfferResponseObject, error) {
	principal, ok := auth.PrincipalFrom(ctx)
	if !ok {
		return gen.CreateOffer401JSONResponse{
			UnauthorizedJSONResponse: gen.UnauthorizedJSONResponse(
				errorBody(apierror.CodeUnauthorized, "Log in to answer a request.")),
		}, nil
	}
	if !principal.Verified {
		return gen.CreateOffer403JSONResponse(errorBody(apierror.CodeForbidden,
			"Confirm your uwaterloo.ca address before answering a request.")), nil
	}
	if request.Body == nil {
		return gen.CreateOffer400JSONResponse{
			BadRequestJSONResponse: gen.BadRequestJSONResponse(
				errorBody(apierror.CodeBadRequest, "A listing to offer is required.")),
		}, nil
	}

	// The request must exist and be open to offers. A draft, paused or
	// taken-down request answers as though it were never there.
	requestRow, err := s.queries.GetPublishedRequest(ctx, uuid.UUID(request.Id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return gen.CreateOffer404JSONResponse{
				NotFoundJSONResponse: gen.NotFoundJSONResponse(
					errorBody(apierror.CodeNotFound, "No request with that id.")),
			}, nil
		}
		s.log.Error("load request for offer", slog.String("error", err.Error()))
		return nil, err
	}

	// The listing must be yours, and published: offering a place nobody can see
	// asks the student to take it on trust.
	listing, err := s.queries.GetListingForOwner(ctx, uuid.UUID(request.Body.ListingId))
	if err != nil || listing.OwnerID != principal.UserID {
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			s.log.Error("load listing for offer", slog.String("error", err.Error()))
			return nil, err
		}
		// 404 rather than 403: whether that listing exists is not the caller's
		// business unless it is theirs.
		return gen.CreateOffer404JSONResponse{
			NotFoundJSONResponse: gen.NotFoundJSONResponse(
				errorBody(apierror.CodeNotFound, "No listing of yours with that id.")),
		}, nil
	}
	if listing.Status != "published" {
		return gen.CreateOffer400JSONResponse{
			BadRequestJSONResponse: gen.BadRequestJSONResponse(errorBody(apierror.CodeValidation,
				"Publish that listing before offering it — the student needs to be able to see the place.")),
		}, nil
	}

	note := ""
	if request.Body.Note != nil {
		note = strings.TrimSpace(*request.Body.Note)
	}
	if len(note) > maxNoteLength {
		return gen.CreateOffer400JSONResponse{
			BadRequestJSONResponse: gen.BadRequestJSONResponse(errorBody(apierror.CodeValidation,
				"Keep the note under 1000 characters. The listing says the rest.")),
		}, nil
	}

	// Said in words rather than as a constraint violation.
	if existing, err := s.queries.GetOfferForListing(ctx, sqlcgen.GetOfferForListingParams{
		RequestID: requestRow.HousingRequest.ID,
		ListingID: listing.ID,
	}); err == nil {
		message := "You have already offered that listing here."
		if existing.WithdrawnAt != nil {
			message = "You offered that listing here before and withdrew it. Offer a different place, or repost this one."
		}
		return gen.CreateOffer409JSONResponse(errorBody(apierror.CodeConflict, message)), nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		s.log.Error("check existing offer", slog.String("error", err.Error()))
		return nil, err
	}

	offer, err := s.queries.CreateOffer(ctx, sqlcgen.CreateOfferParams{
		RequestID: requestRow.HousingRequest.ID,
		ListingID: listing.ID,
		OwnerID:   principal.UserID,
		Note:      note,
	})
	if err != nil {
		s.log.Error("create offer", slog.String("error", err.Error()))
		return nil, err
	}

	s.log.Info("offer made",
		slog.String("offer_id", offer.ID.String()),
		slog.String("request_id", requestRow.HousingRequest.ID.String()),
		slog.String("listing_id", listing.ID.String()))

	return gen.CreateOffer201JSONResponse(s.offerResponse(ctx, offer, listing, principal.UserID)), nil
}

// --------------------------------------------------------------------- read

func (s *Server) ListOffers(ctx context.Context, request gen.ListOffersRequestObject) (gen.ListOffersResponseObject, error) {
	principal, ok := auth.PrincipalFrom(ctx)
	if !ok {
		return gen.ListOffers401JSONResponse{
			UnauthorizedJSONResponse: gen.UnauthorizedJSONResponse(
				errorBody(apierror.CodeUnauthorized, "Log in to see the offers on a request.")),
		}, nil
	}

	requestRow, err := s.queries.GetRequestForOwner(ctx, uuid.UUID(request.Id))
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			s.log.Error("load request for offers", slog.String("error", err.Error()))
		}
		return gen.ListOffers404JSONResponse{
			NotFoundJSONResponse: gen.NotFoundJSONResponse(
				errorBody(apierror.CodeNotFound, "No request with that id.")),
		}, nil
	}

	rows, err := s.queries.ListOffersForRequest(ctx, requestRow.ID)
	if err != nil {
		s.log.Error("list offers", slog.String("error", err.Error()))
		return nil, err
	}

	// The poster sees every live offer. An owner sees only their own, so the
	// answers to a request stay between the student and each owner.
	poster := requestRow.PosterID == principal.UserID

	out := make([]gen.RequestOffer, 0, len(rows))
	for _, row := range rows {
		if !poster && row.RequestOffer.OwnerID != principal.UserID {
			continue
		}
		out = append(out, gen.RequestOffer{
			Id:        openapi_types.UUID(row.RequestOffer.ID),
			RequestId: openapi_types.UUID(row.RequestOffer.RequestID),
			Note:      row.RequestOffer.Note,
			CreatedAt: row.RequestOffer.CreatedAt,
			Listing: toListing(row.Listing, gen.ListingOwner{
				Id:        openapi_types.UUID(row.RequestOffer.OwnerID),
				Username:  row.OwnerUsername,
				AvatarUrl: nullableString(row.OwnerAvatarUrl),
				Verified:  row.OwnerVerified,
			}, nil),
			Owner: gen.ListingOwner{
				Id:        openapi_types.UUID(row.RequestOffer.OwnerID),
				Username:  row.OwnerUsername,
				AvatarUrl: nullableString(row.OwnerAvatarUrl),
				Verified:  row.OwnerVerified,
			},
		})
	}

	return gen.ListOffers200JSONResponse(out), nil
}

// ----------------------------------------------------------------- withdraw

func (s *Server) WithdrawOffer(ctx context.Context, request gen.WithdrawOfferRequestObject) (gen.WithdrawOfferResponseObject, error) {
	principal, ok := auth.PrincipalFrom(ctx)
	if !ok {
		return gen.WithdrawOffer401JSONResponse{
			UnauthorizedJSONResponse: gen.UnauthorizedJSONResponse(
				errorBody(apierror.CodeUnauthorized, "Log in to withdraw an offer.")),
		}, nil
	}

	offer, err := s.queries.GetOffer(ctx, uuid.UUID(request.Id))
	if err != nil || offer.OwnerID != principal.UserID {
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			s.log.Error("load offer", slog.String("error", err.Error()))
			return nil, err
		}
		// Someone else's offer answers 404, never 403.
		return gen.WithdrawOffer404JSONResponse{
			NotFoundJSONResponse: gen.NotFoundJSONResponse(
				errorBody(apierror.CodeNotFound, "No offer of yours with that id.")),
		}, nil
	}

	// Already withdrawn is success: the caller wanted it gone, and it is.
	if offer.WithdrawnAt == nil {
		if _, err := s.queries.WithdrawOffer(ctx, offer.ID); err != nil {
			s.log.Error("withdraw offer", slog.String("error", err.Error()))
			return nil, err
		}
		s.log.Info("offer withdrawn", slog.String("offer_id", offer.ID.String()))
	}

	return gen.WithdrawOffer204Response{}, nil
}

// --------------------------------------------------------------- shared bits

// offerResponse builds the contract shape for an offer the caller just made.
func (s *Server) offerResponse(ctx context.Context, offer sqlcgen.RequestOffer, listing sqlcgen.Listing, ownerID uuid.UUID) gen.RequestOffer {
	owner := gen.ListingOwner{Id: openapi_types.UUID(ownerID)}
	if u, err := s.queries.GetUserByID(ctx, ownerID); err == nil {
		owner.Username = u.Username
		owner.Verified = u.Verified
		owner.AvatarUrl = nullableString(u.AvatarUrl)
	}

	return gen.RequestOffer{
		Id:        openapi_types.UUID(offer.ID),
		RequestId: openapi_types.UUID(offer.RequestID),
		Note:      offer.Note,
		CreatedAt: offer.CreatedAt,
		Listing:   toListing(listing, owner, nil),
		Owner:     owner,
	}
}
