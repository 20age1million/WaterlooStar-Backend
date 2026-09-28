package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/20age1million/WaterlooStar-Backend/internal/apierror"
	"github.com/20age1million/WaterlooStar-Backend/internal/db"
	"github.com/20age1million/WaterlooStar-Backend/internal/httpapi/gen"
)

// Account management: suspend, reinstate, verify by hand, change role. The
// rules live in internal/db (Suspend, Reinstate, VerifyByHand, ChangeRole),
// which is also where each change meets its ledger row. This file checks the
// reason, runs the action, and says what happened in the contract's terms.

// minReasonLength is the shortest reason accepted, once trimmed. A ledger of
// "ok" and "spam" answers nothing a month later.
const minReasonLength = 10

// accountOutcome is the result of one action before it is fitted to its
// operation's generated response types.
type accountOutcome struct {
	status int // 200, 400, 404 or 409
	body   gen.Error
	user   gen.AdminUser
}

// accountAction validates the reason, runs act, and maps its result.
// selfMessage is what to say when the admin targets their own account.
func (s *Server) accountAction(
	ctx context.Context,
	id openapi_types.UUID,
	reason *string,
	selfMessage string,
	act func(userID uuid.UUID, reason string) error,
) (accountOutcome, error) {
	if reason == nil {
		return accountOutcome{status: statusBadRequest, body: errorBody(apierror.CodeBadRequest, "A JSON body is required.")}, nil
	}
	trimmed := strings.TrimSpace(*reason)
	switch n := utf8.RuneCountInString(trimmed); {
	case n < minReasonLength:
		return accountOutcome{status: statusBadRequest, body: errorBodyWithDetails(apierror.CodeValidation,
			"Give a reason.", map[string]string{"reason": "At least 10 characters. It is kept in the ledger."})}, nil
	case n > 1000:
		return accountOutcome{status: statusBadRequest, body: errorBodyWithDetails(apierror.CodeValidation,
			"That reason is too long.", map[string]string{"reason": "At most 1,000 characters."})}, nil
	}

	userID := uuid.UUID(id)
	err := act(userID, trimmed)

	conflict := func(msg string) (accountOutcome, error) {
		return accountOutcome{status: statusConflict, body: errorBody(apierror.CodeConflict, msg)}, nil
	}
	switch {
	case err == nil:
	case errors.Is(err, pgx.ErrNoRows):
		return accountOutcome{status: 404, body: errorBody(apierror.CodeNotFound, "No account with that id.")}, nil
	case errors.Is(err, db.ErrSelfAction):
		return conflict(selfMessage)
	case errors.Is(err, db.ErrAlreadySuspended):
		return conflict("That account is already suspended.")
	case errors.Is(err, db.ErrNotSuspended):
		return conflict("That account is not suspended.")
	case errors.Is(err, db.ErrAlreadyVerified):
		return conflict("That account is already verified.")
	case errors.Is(err, db.ErrRoleUnchanged):
		return conflict("That account already has that role.")
	case errors.Is(err, db.ErrLastAdmin):
		return conflict("That is the last admin. Promote someone else first.")
	default:
		s.log.Error("admin account action", slog.String("user", userID.String()), slog.String("error", err.Error()))
		return accountOutcome{}, err
	}

	row, err := s.queries.GetUserForAdmin(ctx, userID)
	if err != nil {
		s.log.Error("reload account after admin action", slog.String("error", err.Error()))
		return accountOutcome{}, err
	}
	return accountOutcome{status: 200, user: toAdminUser(adminUserRow(row))}, nil
}

func reasonFrom(body *gen.AdminReason) *string {
	if body == nil {
		return nil
	}
	return &body.Reason
}

// SuspendAdminUser suspends an account.
func (s *Server) SuspendAdminUser(ctx context.Context, request gen.SuspendAdminUserRequestObject) (gen.SuspendAdminUserResponseObject, error) {
	principal, ok := requireAdmin(ctx)
	if !ok {
		return unrouted{}, nil
	}
	o, err := s.accountAction(ctx, request.Id, reasonFrom(request.Body), "You can't suspend your own account.",
		func(id uuid.UUID, reason string) error {
			_, err := db.Suspend(ctx, s.tx, principal.UserID, id, reason)
			return err
		})
	if err != nil {
		return nil, err
	}
	switch o.status {
	case 200:
		return gen.SuspendAdminUser200JSONResponse(o.user), nil
	case statusBadRequest:
		return gen.SuspendAdminUser400JSONResponse{BadRequestJSONResponse: gen.BadRequestJSONResponse(o.body)}, nil
	case 404:
		return gen.SuspendAdminUser404JSONResponse{NotFoundJSONResponse: gen.NotFoundJSONResponse(o.body)}, nil
	default:
		return gen.SuspendAdminUser409JSONResponse(o.body), nil
	}
}

// ReinstateAdminUser lifts a suspension.
func (s *Server) ReinstateAdminUser(ctx context.Context, request gen.ReinstateAdminUserRequestObject) (gen.ReinstateAdminUserResponseObject, error) {
	principal, ok := requireAdmin(ctx)
	if !ok {
		return unrouted{}, nil
	}
	o, err := s.accountAction(ctx, request.Id, reasonFrom(request.Body), "",
		func(id uuid.UUID, reason string) error {
			_, err := db.Reinstate(ctx, s.tx, principal.UserID, id, reason)
			return err
		})
	if err != nil {
		return nil, err
	}
	switch o.status {
	case 200:
		return gen.ReinstateAdminUser200JSONResponse(o.user), nil
	case statusBadRequest:
		return gen.ReinstateAdminUser400JSONResponse{BadRequestJSONResponse: gen.BadRequestJSONResponse(o.body)}, nil
	case 404:
		return gen.ReinstateAdminUser404JSONResponse{NotFoundJSONResponse: gen.NotFoundJSONResponse(o.body)}, nil
	default:
		return gen.ReinstateAdminUser409JSONResponse(o.body), nil
	}
}

// VerifyAdminUser verifies an account by hand.
func (s *Server) VerifyAdminUser(ctx context.Context, request gen.VerifyAdminUserRequestObject) (gen.VerifyAdminUserResponseObject, error) {
	principal, ok := requireAdmin(ctx)
	if !ok {
		return unrouted{}, nil
	}
	o, err := s.accountAction(ctx, request.Id, reasonFrom(request.Body), "",
		func(id uuid.UUID, reason string) error {
			_, err := db.VerifyByHand(ctx, s.tx, principal.UserID, id, reason)
			return err
		})
	if err != nil {
		return nil, err
	}
	switch o.status {
	case 200:
		return gen.VerifyAdminUser200JSONResponse(o.user), nil
	case statusBadRequest:
		return gen.VerifyAdminUser400JSONResponse{BadRequestJSONResponse: gen.BadRequestJSONResponse(o.body)}, nil
	case 404:
		return gen.VerifyAdminUser404JSONResponse{NotFoundJSONResponse: gen.NotFoundJSONResponse(o.body)}, nil
	default:
		return gen.VerifyAdminUser409JSONResponse(o.body), nil
	}
}

// SetAdminUserRole moves an account between user and admin.
func (s *Server) SetAdminUserRole(ctx context.Context, request gen.SetAdminUserRoleRequestObject) (gen.SetAdminUserRoleResponseObject, error) {
	principal, ok := requireAdmin(ctx)
	if !ok {
		return unrouted{}, nil
	}

	var reason *string
	if request.Body != nil {
		if !request.Body.Role.Valid() {
			return gen.SetAdminUserRole400JSONResponse{BadRequestJSONResponse: gen.BadRequestJSONResponse(
				errorBodyWithDetails(apierror.CodeValidation, "Choose a role.",
					map[string]string{"role": "Either user or admin."}))}, nil
		}
		reason = &request.Body.Reason
	}

	o, err := s.accountAction(ctx, request.Id, reason, "You can't change your own role.",
		func(id uuid.UUID, reason string) error {
			actor := principal.UserID
			_, err := db.ChangeRole(ctx, s.tx, &actor, id, string(request.Body.Role), reason)
			return err
		})
	if err != nil {
		return nil, err
	}
	switch o.status {
	case 200:
		return gen.SetAdminUserRole200JSONResponse(o.user), nil
	case statusBadRequest:
		return gen.SetAdminUserRole400JSONResponse{BadRequestJSONResponse: gen.BadRequestJSONResponse(o.body)}, nil
	case 404:
		return gen.SetAdminUserRole404JSONResponse{NotFoundJSONResponse: gen.NotFoundJSONResponse(o.body)}, nil
	default:
		return gen.SetAdminUserRole409JSONResponse(o.body), nil
	}
}
