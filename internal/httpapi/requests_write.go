package httpapi

import (
	"context"
	"errors"
	"fmt"
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

// The write half of "Looking for Housing". The same two rules as listings:
//
//   - Only a verified student may post.
//   - Someone else's request answers 404, never 403.

// ------------------------------------------------------------------- create

func (s *Server) CreateRequest(ctx context.Context, request gen.CreateRequestRequestObject) (gen.CreateRequestResponseObject, error) {
	principal, ok := auth.PrincipalFrom(ctx)
	if !ok {
		return gen.CreateRequest401JSONResponse{
			UnauthorizedJSONResponse: gen.UnauthorizedJSONResponse(
				errorBody(apierror.CodeUnauthorized, "Log in to post what you are looking for.")),
		}, nil
	}
	if !principal.Verified {
		return gen.CreateRequest403JSONResponse(errorBody(apierror.CodeForbidden,
			"Confirm your uwaterloo.ca address before posting. That badge is what tells other students who they are dealing with.")), nil
	}
	if request.Body == nil {
		return gen.CreateRequest400JSONResponse{
			BadRequestJSONResponse: gen.BadRequestJSONResponse(
				errorBody(apierror.CodeBadRequest, "A JSON body is required.")),
		}, nil
	}

	in := *request.Body
	params := sqlcgen.CreateRequestParams{
		PosterID:           principal.UserID,
		Title:              strings.TrimSpace(in.Title),
		Body:               strings.TrimSpace(derefString(in.Body)),
		BudgetCents:        int32(in.BudgetCents),
		StartDate:          in.StartDate.Time,
		EndDate:            in.EndDate.Time,
		LeaseMonths:        int32(in.LeaseMonths),
		TermTag:            strings.TrimSpace(in.TermTag),
		Occupants:          1,
		Pets:               derefBool(in.Pets),
		FurnishedPreferred: derefBool(in.FurnishedPreferred),
		ParkingNeeded:      derefBool(in.ParkingNeeded),
		LaundryNeeded:      derefBool(in.LaundryNeeded),
		MaxDistanceM:       int32Ptr(nullableToPtrInt(in.MaxDistanceM)),
		Neighbourhood:      strings.TrimSpace(derefString(in.Neighbourhood)),
		Status:             "published",
		CreatedAt:          time.Now(),
	}
	if in.Occupants != nil {
		params.Occupants = int32(*in.Occupants)
	}
	if in.Status != nil {
		params.Status = string(*in.Status)
	}

	if problems := validateRequest(params); len(problems) > 0 {
		return gen.CreateRequest400JSONResponse{
			BadRequestJSONResponse: gen.BadRequestJSONResponse(errorBodyWithDetails(
				apierror.CodeValidation, "Check the highlighted fields.", problems)),
		}, nil
	}

	row, err := s.queries.CreateRequest(ctx, params)
	if err != nil {
		s.log.Error("create request", slog.String("error", err.Error()))
		return nil, err
	}

	s.log.Info("request created",
		slog.String("request_id", row.ID.String()),
		slog.String("poster_id", principal.UserID.String()))

	return gen.CreateRequest201JSONResponse(s.requestResponse(ctx, row)), nil
}

// ------------------------------------------------------------------- update

func (s *Server) UpdateRequest(ctx context.Context, request gen.UpdateRequestRequestObject) (gen.UpdateRequestResponseObject, error) {
	_, existing, resp := s.ownedRequest(ctx, uuid.UUID(request.Id))
	if resp != nil {
		return updateRequestRefusal(resp), nil
	}

	if request.Body == nil {
		return gen.UpdateRequest400JSONResponse{
			BadRequestJSONResponse: gen.BadRequestJSONResponse(
				errorBody(apierror.CodeBadRequest, "A JSON body is required.")),
		}, nil
	}
	in := *request.Body

	// Validate the request as it would be *after* the edit: moving only the end
	// date can still invert the term.
	merged := existing
	applyRequestUpdate(&merged, in)
	if problems := validateRequestRow(merged); len(problems) > 0 {
		return gen.UpdateRequest400JSONResponse{
			BadRequestJSONResponse: gen.BadRequestJSONResponse(errorBodyWithDetails(
				apierror.CodeValidation, "Check the highlighted fields.", problems)),
		}, nil
	}

	updated, err := s.queries.UpdateRequest(ctx, requestUpdateParams(existing.ID, in))
	if err != nil {
		s.log.Error("update request", slog.String("error", err.Error()))
		return nil, err
	}

	return gen.UpdateRequest200JSONResponse(s.requestResponse(ctx, updated)), nil
}

// ------------------------------------------------------------------- status

func (s *Server) SetRequestStatus(ctx context.Context, request gen.SetRequestStatusRequestObject) (gen.SetRequestStatusResponseObject, error) {
	_, existing, resp := s.ownedRequest(ctx, uuid.UUID(request.Id))
	if resp != nil {
		return requestStatusRefusal(resp), nil
	}
	if request.Body == nil {
		return gen.SetRequestStatus400JSONResponse(
			errorBody(apierror.CodeBadRequest, "A status is required.")), nil
	}

	want := string(request.Body.Status)
	if want == existing.Status {
		// Idempotent: asking for the state it is already in is not an error.
		return gen.SetRequestStatus200JSONResponse(s.requestResponse(ctx, existing)), nil
	}

	// The same lifecycle as listings: archived is terminal.
	if !contains(allowedTransitions[existing.Status], want) {
		message := "A request cannot go from " + existing.Status + " to " + want + "."
		if existing.Status == "archived" {
			message = "This request has been taken down, and that is final. Post a new one instead — by now the term and the budget will have moved on."
		}
		return gen.SetRequestStatus400JSONResponse(
			errorBody(apierror.CodeBadRequest, message)), nil
	}

	updated, err := s.queries.SetRequestStatus(ctx, sqlcgen.SetRequestStatusParams{
		ID: existing.ID, Status: want,
	})
	if err != nil {
		s.log.Error("set request status", slog.String("error", err.Error()))
		return nil, err
	}

	s.log.Info("request status changed",
		slog.String("request_id", existing.ID.String()),
		slog.String("from", existing.Status), slog.String("to", want))

	return gen.SetRequestStatus200JSONResponse(s.requestResponse(ctx, updated)), nil
}

// ------------------------------------------------------------------- delete

func (s *Server) DeleteRequest(ctx context.Context, request gen.DeleteRequestRequestObject) (gen.DeleteRequestResponseObject, error) {
	_, existing, resp := s.ownedRequest(ctx, uuid.UUID(request.Id))
	if resp != nil {
		return deleteRequestRefusal(resp), nil
	}

	// Archive rather than remove, as with listings: offers will reference this
	// row, and deleting it would take them with it.
	if existing.Status != "archived" {
		if _, err := s.queries.SetRequestStatus(ctx, sqlcgen.SetRequestStatusParams{
			ID: existing.ID, Status: "archived",
		}); err != nil {
			s.log.Error("archive request", slog.String("error", err.Error()))
			return nil, err
		}
	}

	return gen.DeleteRequest204Response{}, nil
}

// --------------------------------------------------------------- my requests

func (s *Server) ListMyRequests(ctx context.Context, _ gen.ListMyRequestsRequestObject) (gen.ListMyRequestsResponseObject, error) {
	principal, ok := auth.PrincipalFrom(ctx)
	if !ok {
		return gen.ListMyRequests401JSONResponse{
			UnauthorizedJSONResponse: gen.UnauthorizedJSONResponse(
				errorBody(apierror.CodeUnauthorized, "Log in to see your requests.")),
		}, nil
	}

	rows, err := s.queries.ListRequestsByPoster(ctx, principal.UserID)
	if err != nil {
		s.log.Error("list own requests", slog.String("error", err.Error()))
		return nil, err
	}

	out := make([]gen.HousingRequest, 0, len(rows))
	for _, row := range rows {
		out = append(out, toRequest(row.HousingRequest, gen.ListingOwner{
			Id:        openapi_types.UUID(row.HousingRequest.PosterID),
			Username:  row.PosterUsername,
			AvatarUrl: nullableString(row.PosterAvatarUrl),
			Verified:  row.PosterVerified,
		}, row.OfferCount))
	}

	return gen.ListMyRequests200JSONResponse(out), nil
}

// --------------------------------------------------------------- shared bits

// ownedRequest resolves a request the caller owns, or the refusal to return.
// "Not yours" and "does not exist" are deliberately the same answer.
func (s *Server) ownedRequest(ctx context.Context, id uuid.UUID) (auth.Principal, sqlcgen.HousingRequest, *refusal) {
	principal, ok := auth.PrincipalFrom(ctx)
	if !ok {
		return principal, sqlcgen.HousingRequest{}, &refusal{unauthorised: true, message: "Log in to manage your requests."}
	}

	row, err := s.queries.GetRequestForOwner(ctx, id)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			s.log.Error("load request for owner", slog.String("error", err.Error()))
		}
		return principal, sqlcgen.HousingRequest{}, &refusal{message: "No request with that id."}
	}

	if row.PosterID != principal.UserID {
		return principal, sqlcgen.HousingRequest{}, &refusal{message: "No request with that id."}
	}

	return principal, row, nil
}

// requestResponse attaches the poster summary and the visible offer count to a
// row. The count is queried rather than assumed zero: an edit does not change
// how many owners have answered.
func (s *Server) requestResponse(ctx context.Context, row sqlcgen.HousingRequest) gen.HousingRequest {
	poster := gen.ListingOwner{Id: openapi_types.UUID(row.PosterID)}
	if u, err := s.queries.GetUserByID(ctx, row.PosterID); err == nil {
		poster.Username = u.Username
		poster.Verified = u.Verified
		poster.AvatarUrl = nullableString(u.AvatarUrl)
	}

	offers, err := s.queries.CountOffersForRequest(ctx, row.ID)
	if err != nil {
		// The request itself is fine; report it with no offers rather than
		// failing the write that just succeeded.
		s.log.Error("count offers for request", slog.String("error", err.Error()))
	}

	return toRequest(row, poster, offers)
}

// Each operation has its own generated response types, so the same refusal has
// to be expressed once per operation: no session gives 401, anything else 404.

func updateRequestRefusal(r *refusal) gen.UpdateRequestResponseObject {
	if r.unauthorised {
		return gen.UpdateRequest401JSONResponse{
			UnauthorizedJSONResponse: gen.UnauthorizedJSONResponse(
				errorBody(apierror.CodeUnauthorized, r.message)),
		}
	}
	return gen.UpdateRequest404JSONResponse{
		NotFoundJSONResponse: gen.NotFoundJSONResponse(
			errorBody(apierror.CodeNotFound, r.message)),
	}
}

func requestStatusRefusal(r *refusal) gen.SetRequestStatusResponseObject {
	if r.unauthorised {
		return gen.SetRequestStatus401JSONResponse{
			UnauthorizedJSONResponse: gen.UnauthorizedJSONResponse(
				errorBody(apierror.CodeUnauthorized, r.message)),
		}
	}
	return gen.SetRequestStatus404JSONResponse{
		NotFoundJSONResponse: gen.NotFoundJSONResponse(
			errorBody(apierror.CodeNotFound, r.message)),
	}
}

func deleteRequestRefusal(r *refusal) gen.DeleteRequestResponseObject {
	if r.unauthorised {
		return gen.DeleteRequest401JSONResponse{
			UnauthorizedJSONResponse: gen.UnauthorizedJSONResponse(
				errorBody(apierror.CodeUnauthorized, r.message)),
		}
	}
	return gen.DeleteRequest404JSONResponse{
		NotFoundJSONResponse: gen.NotFoundJSONResponse(
			errorBody(apierror.CodeNotFound, r.message)),
	}
}

// ---------------------------------------------------------------- validation

// The database has check constraints for all of this, and they are the real
// guarantee. These exist so a person gets the problem against the right field.

func validateRequest(p sqlcgen.CreateRequestParams) map[string]string {
	return validateRequestRow(sqlcgen.HousingRequest{
		Title: p.Title, BudgetCents: p.BudgetCents,
		StartDate: p.StartDate, EndDate: p.EndDate, LeaseMonths: p.LeaseMonths,
		Occupants: p.Occupants, MaxDistanceM: p.MaxDistanceM, Status: p.Status,
	})
}

func validateRequestRow(r sqlcgen.HousingRequest) map[string]string {
	problems := map[string]string{}

	if len(strings.TrimSpace(r.Title)) < 8 {
		problems["title"] = "Say what you are looking for, in at least 8 characters."
	}
	if r.BudgetCents <= 0 {
		problems["budget_cents"] = "Your budget must be more than zero."
	}
	if !r.EndDate.After(r.StartDate) {
		problems["end_date"] = "The end of the term must fall after its start."
	}
	if r.LeaseMonths < 1 || r.LeaseMonths > 24 {
		problems["lease_months"] = "A lease runs between 1 and 24 months."
	}
	if r.Occupants < 1 || r.Occupants > 12 {
		problems["occupants"] = "A place is for between 1 and 12 people."
	}
	if r.MaxDistanceM != nil && *r.MaxDistanceM < 1 {
		problems["max_distance_m"] = fmt.Sprintf("A radius of %d m is not a distance.", *r.MaxDistanceM)
	}
	if r.Status != "" && r.Status != "draft" && r.Status != "published" &&
		r.Status != "paused" && r.Status != "archived" {
		problems["status"] = "Unknown status."
	}

	return problems
}

// applyRequestUpdate mirrors the SQL's COALESCE, so validation sees what the
// row will actually become.
func applyRequestUpdate(r *sqlcgen.HousingRequest, in gen.RequestUpdate) {
	if in.Title != nil {
		r.Title = *in.Title
	}
	if in.Body != nil {
		r.Body = *in.Body
	}
	if in.BudgetCents != nil {
		r.BudgetCents = int32(*in.BudgetCents)
	}
	if in.StartDate != nil {
		r.StartDate = in.StartDate.Time
	}
	if in.EndDate != nil {
		r.EndDate = in.EndDate.Time
	}
	if in.LeaseMonths != nil {
		r.LeaseMonths = int32(*in.LeaseMonths)
	}
	if in.Occupants != nil {
		r.Occupants = int32(*in.Occupants)
	}
	if v, ok := nullableValue(in.MaxDistanceM); ok {
		r.MaxDistanceM = int32Ptr(v)
	}
}

// requestUpdateParams maps a partial edit onto the query's parameters. A nil
// stays nil, and the SQL's COALESCE leaves that column as it was.
func requestUpdateParams(id uuid.UUID, in gen.RequestUpdate) sqlcgen.UpdateRequestParams {
	p := sqlcgen.UpdateRequestParams{ID: id}

	p.Title = trimmedPtr(in.Title)
	p.Body = trimmedPtr(in.Body)
	p.BudgetCents = int32From(in.BudgetCents)
	if in.StartDate != nil {
		d := in.StartDate.Time
		p.StartDate = &d
	}
	if in.EndDate != nil {
		d := in.EndDate.Time
		p.EndDate = &d
	}
	p.LeaseMonths = int32From(in.LeaseMonths)
	p.TermTag = trimmedPtr(in.TermTag)
	p.Occupants = int32From(in.Occupants)
	p.Pets = in.Pets
	p.FurnishedPreferred = in.FurnishedPreferred
	p.ParkingNeeded = in.ParkingNeeded
	p.LaundryNeeded = in.LaundryNeeded
	if v, ok := nullableValue(in.MaxDistanceM); ok {
		p.MaxDistanceM = int32Ptr(v)
	}
	p.Neighbourhood = trimmedPtr(in.Neighbourhood)

	return p
}
