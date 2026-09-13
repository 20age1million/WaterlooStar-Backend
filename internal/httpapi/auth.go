package httpapi

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/20age1million/waterloostar-api/internal/apierror"
	"github.com/20age1million/waterloostar-api/internal/auth"
	"github.com/20age1million/waterloostar-api/internal/db/sqlcgen"
	"github.com/20age1million/waterloostar-api/internal/email"
	"github.com/20age1million/waterloostar-api/internal/httpapi/gen"
)

// The one rule the whole product rests on: an account belongs to a Waterloo
// student, proved by control of a uwaterloo.ca address. Subdomains count
// (@edu.uwaterloo.ca), so the check is on the suffix after the @.
const uwaterlooSuffix = "uwaterloo.ca"

var usernamePattern = regexp.MustCompile(`^[A-Za-z0-9_.-]{3,32}$`)

// ---------------------------------------------------------------- registration

func (s *Server) Register(ctx context.Context, request gen.RegisterRequestObject) (gen.RegisterResponseObject, error) {
	if request.Body == nil {
		return gen.Register400JSONResponse{
			BadRequestJSONResponse: gen.BadRequestJSONResponse(
				errorBody(apierror.CodeBadRequest, "A JSON body is required.")),
		}, nil
	}

	emailAddr := strings.TrimSpace(string(request.Body.Email))
	username := strings.TrimSpace(request.Body.Username)
	password := request.Body.Password

	if problems := validateRegistration(emailAddr, username, password); len(problems) > 0 {
		return gen.Register400JSONResponse{
			BadRequestJSONResponse: gen.BadRequestJSONResponse(
				errorBodyWithDetails(apierror.CodeValidation, "Check the highlighted fields.", problems)),
		}, nil
	}

	// Checked before insert so the response can be a clean 409, but the unique
	// indexes remain the actual guarantee against a concurrent duplicate.
	counts, err := s.queries.CountUsersByEmailOrUsername(ctx, sqlcgen.CountUsersByEmailOrUsernameParams{
		Lower:   emailAddr,
		Lower_2: username,
	})
	if err != nil {
		s.log.Error("count existing users", slog.String("error", err.Error()))
		return nil, err
	}
	if counts.EmailCount > 0 || counts.UsernameCount > 0 {
		// Deliberately does not say which collided: naming it would turn this
		// endpoint into a way to discover who has an account.
		return gen.Register409JSONResponse(
			errorBody(apierror.CodeConflict, "That email address or username is already taken.")), nil
	}

	hash, err := auth.HashPassword(password)
	if err != nil {
		s.log.Error("hash password", slog.String("error", err.Error()))
		return nil, err
	}

	user, err := s.queries.CreateUser(ctx, sqlcgen.CreateUserParams{
		Email:        emailAddr,
		Username:     username,
		PasswordHash: hash,
	})
	if err != nil {
		// The unique index firing here means someone registered the same
		// identity between the count above and this insert.
		if isUniqueViolation(err) {
			return gen.Register409JSONResponse(
				errorBody(apierror.CodeConflict, "That email address or username is already taken.")), nil
		}
		s.log.Error("create user", slog.String("error", err.Error()))
		return nil, err
	}

	if err := s.issueVerificationEmail(ctx, user); err != nil {
		// The account exists; failing the whole request would leave the user
		// unable to retry. Log it and let them request a new link.
		s.log.Error("send verification email",
			slog.String("user_id", user.ID.String()),
			slog.String("error", err.Error()))
	}

	return gen.Register201JSONResponse{
		User:                 toUser(user, true),
		VerificationRequired: true,
	}, nil
}

func validateRegistration(emailAddr, username, password string) map[string]string {
	problems := map[string]string{}

	at := strings.LastIndex(emailAddr, "@")
	switch {
	case at <= 0 || at == len(emailAddr)-1:
		problems["email"] = "Enter a valid email address."
	case !isUWaterlooDomain(emailAddr[at+1:]):
		problems["email"] = "Use your University of Waterloo address, ending in uwaterloo.ca."
	}

	if !usernamePattern.MatchString(username) {
		problems["username"] = "3 to 32 characters: letters, numbers, dot, dash or underscore."
	}

	if err := auth.ValidatePassword(password); err != nil {
		problems["password"] = err.Error()
	}

	return problems
}

// isUWaterlooDomain accepts uwaterloo.ca and its subdomains, and nothing that
// merely ends with the same letters (evil-uwaterloo.ca, uwaterloo.ca.attacker.com).
func isUWaterlooDomain(domain string) bool {
	d := strings.ToLower(strings.TrimSuffix(domain, "."))
	return d == uwaterlooSuffix || strings.HasSuffix(d, "."+uwaterlooSuffix)
}

func (s *Server) issueVerificationEmail(ctx context.Context, user sqlcgen.User) error {
	// Any outstanding link is spent first, so a forwarded older email cannot
	// still be redeemed.
	if err := s.queries.ConsumeAllEmailVerificationTokensForUser(ctx, user.ID); err != nil {
		return fmt.Errorf("consume previous tokens: %w", err)
	}

	raw, hash, err := auth.NewOpaqueToken()
	if err != nil {
		return err
	}

	if err := s.queries.CreateEmailVerificationToken(ctx, sqlcgen.CreateEmailVerificationTokenParams{
		TokenHash: hash,
		UserID:    user.ID,
		ExpiresAt: time.Now().Add(auth.VerificationTokenTTL),
	}); err != nil {
		return fmt.Errorf("store verification token: %w", err)
	}

	link := fmt.Sprintf("%s/verify/%s", s.cfg.AppURL, url.PathEscape(raw))
	return s.mailer.Send(ctx, email.VerificationMessage(user.Email, link))
}

// ---------------------------------------------------------------- verification

func (s *Server) VerifyEmail(ctx context.Context, request gen.VerifyEmailRequestObject) (gen.VerifyEmailResponseObject, error) {
	if request.Body == nil || strings.TrimSpace(request.Body.Token) == "" {
		return gen.VerifyEmail400JSONResponse(
			errorBody(apierror.CodeBadRequest, "A verification token is required.")), nil
	}

	// The query filters on unconsumed and unexpired, so a spent or stale token
	// is simply not found.
	token, err := s.queries.GetLiveEmailVerificationToken(ctx, auth.HashToken(request.Body.Token))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return gen.VerifyEmail400JSONResponse(errorBody(apierror.CodeBadRequest,
				"That confirmation link is invalid or has expired. Request a new one.")), nil
		}
		s.log.Error("look up verification token", slog.String("error", err.Error()))
		return nil, err
	}

	if err := s.queries.ConsumeEmailVerificationToken(ctx, token.TokenHash); err != nil {
		s.log.Error("consume verification token", slog.String("error", err.Error()))
		return nil, err
	}

	user, err := s.queries.MarkUserVerified(ctx, token.UserID)
	if err != nil {
		s.log.Error("mark user verified", slog.String("error", err.Error()))
		return nil, err
	}

	s.log.Info("account verified", slog.String("user_id", user.ID.String()))
	return gen.VerifyEmail200JSONResponse(toUser(user, true)), nil
}

// ----------------------------------------------------------------------- login

func (s *Server) Login(ctx context.Context, request gen.LoginRequestObject) (gen.LoginResponseObject, error) {
	if request.Body == nil {
		return gen.Login400JSONResponse{
			BadRequestJSONResponse: gen.BadRequestJSONResponse(
				errorBody(apierror.CodeBadRequest, "A JSON body is required.")),
		}, nil
	}

	emailAddr := strings.TrimSpace(string(request.Body.Email))
	user, err := s.queries.GetUserByEmail(ctx, emailAddr)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Spend the time a real comparison would, so response timing does
			// not reveal which addresses are registered.
			auth.WasteComparison(request.Body.Password)
			return invalidCredentials(), nil
		}
		s.log.Error("look up user by email", slog.String("error", err.Error()))
		return nil, err
	}

	if err := auth.ComparePassword(user.PasswordHash, request.Body.Password); err != nil {
		return invalidCredentials(), nil
	}

	cookies, err := s.startSession(ctx, user, userAgentFrom(ctx))
	if err != nil {
		s.log.Error("start session", slog.String("error", err.Error()))
		return nil, err
	}

	return loginWithCookies{user: toUser(user, true), cookies: cookies}, nil
}

func invalidCredentials() gen.Login401JSONResponse {
	// One message for both "no such address" and "wrong password".
	return gen.Login401JSONResponse(
		errorBody(apierror.CodeUnauthorized, "That email address and password do not match."))
}

// startSession mints the access token, stores a fresh refresh token, and returns
// the three cookies that carry a session.
func (s *Server) startSession(ctx context.Context, user sqlcgen.User, userAgent string) ([]*http.Cookie, error) {
	principal := auth.Principal{UserID: user.ID, Role: user.Role, Verified: user.Verified}

	access, accessExpires, err := s.tokens.MintAccessToken(principal)
	if err != nil {
		return nil, err
	}

	refreshRaw, refreshHash, err := auth.NewOpaqueToken()
	if err != nil {
		return nil, err
	}
	refreshExpires := time.Now().Add(auth.RefreshTokenTTL)

	if err := s.queries.CreateRefreshToken(ctx, sqlcgen.CreateRefreshTokenParams{
		TokenHash: refreshHash,
		UserID:    user.ID,
		ExpiresAt: refreshExpires,
		UserAgent: &userAgent,
	}); err != nil {
		return nil, fmt.Errorf("store refresh token: %w", err)
	}

	csrfRaw, _, err := auth.NewOpaqueToken()
	if err != nil {
		return nil, err
	}

	return []*http.Cookie{
		s.cookies.Session(access, accessExpires),
		s.cookies.Refresh(refreshRaw, refreshExpires),
		s.cookies.CSRF(csrfRaw, refreshExpires),
	}, nil
}

// --------------------------------------------------------------------- refresh

func (s *Server) RefreshSession(ctx context.Context, _ gen.RefreshSessionRequestObject) (gen.RefreshSessionResponseObject, error) {
	raw := refreshTokenFrom(ctx)
	if raw == "" {
		return gen.RefreshSession401JSONResponse(
			errorBody(apierror.CodeUnauthorized, "No session to refresh.")), nil
	}

	token, err := s.queries.GetLiveRefreshToken(ctx, auth.HashToken(raw))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return gen.RefreshSession401JSONResponse(
				errorBody(apierror.CodeUnauthorized, "That session has expired. Log in again.")), nil
		}
		s.log.Error("look up refresh token", slog.String("error", err.Error()))
		return nil, err
	}

	user, err := s.queries.GetUserByID(ctx, token.UserID)
	if err != nil {
		s.log.Error("load user for refresh", slog.String("error", err.Error()))
		return nil, err
	}

	// Rotation: the presented token dies as its replacement is born, so a copy
	// taken by an attacker stops working the moment the real user refreshes.
	if err := s.queries.RevokeRefreshToken(ctx, token.TokenHash); err != nil {
		s.log.Error("revoke rotated refresh token", slog.String("error", err.Error()))
		return nil, err
	}

	cookies, err := s.startSession(ctx, user, userAgentFrom(ctx))
	if err != nil {
		s.log.Error("start rotated session", slog.String("error", err.Error()))
		return nil, err
	}

	return refreshWithCookies{user: toUser(user, true), cookies: cookies}, nil
}

// ---------------------------------------------------------------------- logout

func (s *Server) Logout(ctx context.Context, _ gen.LogoutRequestObject) (gen.LogoutResponseObject, error) {
	if raw := refreshTokenFrom(ctx); raw != "" {
		if err := s.queries.RevokeRefreshToken(ctx, auth.HashToken(raw)); err != nil {
			// Clearing the cookies still signs the user out of this browser, so
			// the request succeeds regardless.
			s.log.Error("revoke refresh token on logout", slog.String("error", err.Error()))
		}
	}
	return logoutWithCookies{cookies: s.cookies.Clear()}, nil
}

// -------------------------------------------------------------- password reset

func (s *Server) RequestPasswordReset(ctx context.Context, request gen.RequestPasswordResetRequestObject) (gen.RequestPasswordResetResponseObject, error) {
	if request.Body == nil {
		return gen.RequestPasswordReset400JSONResponse{
			BadRequestJSONResponse: gen.BadRequestJSONResponse(
				errorBody(apierror.CodeBadRequest, "An email address is required.")),
		}, nil
	}

	emailAddr := strings.TrimSpace(string(request.Body.Email))
	user, err := s.queries.GetUserByEmail(ctx, emailAddr)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Same answer as the success path: telling the caller the address is
			// unknown would reveal who has an account.
			s.log.Info("password reset requested for unknown address")
			return gen.RequestPasswordReset202Response{}, nil
		}
		s.log.Error("look up user for reset", slog.String("error", err.Error()))
		return nil, err
	}

	if err := s.issuePasswordResetEmail(ctx, user); err != nil {
		s.log.Error("send password reset email",
			slog.String("user_id", user.ID.String()),
			slog.String("error", err.Error()))
	}

	return gen.RequestPasswordReset202Response{}, nil
}

func (s *Server) issuePasswordResetEmail(ctx context.Context, user sqlcgen.User) error {
	if err := s.queries.ConsumeAllPasswordResetTokensForUser(ctx, user.ID); err != nil {
		return fmt.Errorf("consume previous reset tokens: %w", err)
	}

	raw, hash, err := auth.NewOpaqueToken()
	if err != nil {
		return err
	}

	if err := s.queries.CreatePasswordResetToken(ctx, sqlcgen.CreatePasswordResetTokenParams{
		TokenHash: hash,
		UserID:    user.ID,
		ExpiresAt: time.Now().Add(auth.PasswordResetTTL),
	}); err != nil {
		return fmt.Errorf("store reset token: %w", err)
	}

	link := fmt.Sprintf("%s/password-reset/%s", s.cfg.AppURL, url.PathEscape(raw))
	return s.mailer.Send(ctx, email.PasswordResetMessage(user.Email, link))
}

func (s *Server) ConfirmPasswordReset(ctx context.Context, request gen.ConfirmPasswordResetRequestObject) (gen.ConfirmPasswordResetResponseObject, error) {
	if request.Body == nil || strings.TrimSpace(request.Body.Token) == "" {
		return gen.ConfirmPasswordReset400JSONResponse(
			errorBody(apierror.CodeBadRequest, "A reset token is required.")), nil
	}

	if err := auth.ValidatePassword(request.Body.Password); err != nil {
		return gen.ConfirmPasswordReset400JSONResponse(errorBodyWithDetails(
			apierror.CodeValidation, "Check the highlighted fields.",
			map[string]string{"password": err.Error()})), nil
	}

	token, err := s.queries.GetLivePasswordResetToken(ctx, auth.HashToken(request.Body.Token))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return gen.ConfirmPasswordReset400JSONResponse(errorBody(apierror.CodeBadRequest,
				"That reset link is invalid or has expired. Request a new one.")), nil
		}
		s.log.Error("look up reset token", slog.String("error", err.Error()))
		return nil, err
	}

	hash, err := auth.HashPassword(request.Body.Password)
	if err != nil {
		s.log.Error("hash new password", slog.String("error", err.Error()))
		return nil, err
	}

	if err := s.queries.UpdateUserPassword(ctx, sqlcgen.UpdateUserPasswordParams{
		ID:           token.UserID,
		PasswordHash: hash,
	}); err != nil {
		s.log.Error("update password", slog.String("error", err.Error()))
		return nil, err
	}

	if err := s.queries.ConsumePasswordResetToken(ctx, token.TokenHash); err != nil {
		s.log.Error("consume reset token", slog.String("error", err.Error()))
		return nil, err
	}

	// A reset is usually prompted by a compromise, so it must actually evict
	// whoever is already signed in.
	if err := s.queries.RevokeAllRefreshTokensForUser(ctx, token.UserID); err != nil {
		s.log.Error("revoke sessions after reset", slog.String("error", err.Error()))
		return nil, err
	}

	s.log.Info("password reset completed", slog.String("user_id", token.UserID.String()))
	return gen.ConfirmPasswordReset204Response{}, nil
}

// ------------------------------------------------------------------------- me

func (s *Server) GetMe(ctx context.Context, _ gen.GetMeRequestObject) (gen.GetMeResponseObject, error) {
	principal, ok := auth.PrincipalFrom(ctx)
	if !ok {
		return gen.GetMe401JSONResponse{
			UnauthorizedJSONResponse: gen.UnauthorizedJSONResponse(
				errorBody(apierror.CodeUnauthorized, "Log in to continue.")),
		}, nil
	}

	user, err := s.queries.GetUserByID(ctx, principal.UserID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// The token is valid but the account is gone — deleted while a
			// session was live.
			return gen.GetMe401JSONResponse{
				UnauthorizedJSONResponse: gen.UnauthorizedJSONResponse(
					errorBody(apierror.CodeUnauthorized, "That account no longer exists.")),
			}, nil
		}
		s.log.Error("load current user", slog.String("error", err.Error()))
		return nil, err
	}

	return gen.GetMe200JSONResponse(toUser(user, true)), nil
}
