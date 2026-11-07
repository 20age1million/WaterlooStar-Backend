package httpapi

import (
	"context"
	"errors"
	"log/slog"

	"github.com/jackc/pgx/v5"

	"github.com/20age1million/WaterlooStar-Backend/internal/apierror"
	"github.com/20age1million/WaterlooStar-Backend/internal/auth"
	"github.com/20age1million/WaterlooStar-Backend/internal/db/sqlcgen"
	"github.com/20age1million/WaterlooStar-Backend/internal/httpapi/gen"
)

// The signed-in user's own settings. GET /me lives with the session handlers
// in auth.go.

// UpdateMyPreferences changes the signed-in user's preferences. Today that is
// one switch, email_notifications, for the messaging notification emails
// (notifications.go). Any signed-in account may change it, verified or not:
// turning email off must never need anything first.
func (s *Server) UpdateMyPreferences(ctx context.Context, request gen.UpdateMyPreferencesRequestObject) (gen.UpdateMyPreferencesResponseObject, error) {
	principal, ok := auth.PrincipalFrom(ctx)
	if !ok {
		return gen.UpdateMyPreferences401JSONResponse{
			UnauthorizedJSONResponse: gen.UnauthorizedJSONResponse(
				errorBody(apierror.CodeUnauthorized, "Log in to change your settings.")),
		}, nil
	}
	if request.Body == nil {
		return gen.UpdateMyPreferences400JSONResponse{
			BadRequestJSONResponse: gen.BadRequestJSONResponse(
				errorBody(apierror.CodeBadRequest, "A JSON body is required.")),
		}, nil
	}

	user, err := s.queries.GetUserByID(ctx, principal.UserID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// The token is valid but the account is gone, as GET /me says.
			return gen.UpdateMyPreferences401JSONResponse{
				UnauthorizedJSONResponse: gen.UnauthorizedJSONResponse(
					errorBody(apierror.CodeUnauthorized, "That account no longer exists.")),
			}, nil
		}
		s.log.Error("load user for preferences", slog.String("error", err.Error()))
		return nil, err
	}

	// A field left out is left as it is, so an empty body changes nothing and
	// answers the user as they are.
	if want := request.Body.EmailNotifications; want != nil && *want != user.EmailNotifications {
		user, err = s.queries.SetEmailNotifications(ctx, sqlcgen.SetEmailNotificationsParams{
			ID: principal.UserID, EmailNotifications: *want,
		})
		if err != nil {
			s.log.Error("set email notifications", slog.String("error", err.Error()))
			return nil, err
		}
		s.log.Info("email notifications changed",
			slog.String("user_id", principal.UserID.String()),
			slog.Bool("email_notifications", *want))
	}

	return gen.UpdateMyPreferences200JSONResponse(toUser(user, true)), nil
}
