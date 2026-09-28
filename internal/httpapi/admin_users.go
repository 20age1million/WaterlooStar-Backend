package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/oapi-codegen/nullable"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/20age1million/WaterlooStar-Backend/internal/apierror"
	"github.com/20age1million/WaterlooStar-Backend/internal/db"
	"github.com/20age1million/WaterlooStar-Backend/internal/db/sqlcgen"
	"github.com/20age1million/WaterlooStar-Backend/internal/httpapi/gen"
)

// Accounts as an operator sees them. Read-only until Phase 10.

// ListAdminUsers returns a page of accounts matching the filters.
func (s *Server) ListAdminUsers(ctx context.Context, request gen.ListAdminUsersRequestObject) (gen.ListAdminUsersResponseObject, error) {
	if _, ok := requireAdmin(ctx); !ok {
		return unrouted{}, nil
	}

	p := request.Params
	page, perPage := paginationFrom(p.Page, p.PerPage)

	filters := sqlcgen.CountUsersForAdminParams{Verified: p.Verified, Suspended: p.Suspended}
	// An empty or whitespace-only q is no search, not a search for "".
	if p.Q != nil {
		if trimmed := strings.TrimSpace(*p.Q); trimmed != "" {
			filters.Search = &trimmed
		}
	}
	if p.Role != nil {
		role := string(*p.Role)
		filters.Role = &role
	}

	total, err := s.queries.CountUsersForAdmin(ctx, filters)
	if err != nil {
		s.log.Error("count users for admin", slog.String("error", err.Error()))
		return nil, err
	}

	rows, err := s.queries.ListUsersForAdmin(ctx, sqlcgen.ListUsersForAdminParams{
		Search:    filters.Search,
		Role:      filters.Role,
		Verified:  filters.Verified,
		Suspended: filters.Suspended,
		Limit:     int32(perPage),
		Offset:    int32((page - 1) * perPage),
	})
	if err != nil {
		s.log.Error("list users for admin", slog.String("error", err.Error()))
		return nil, err
	}

	data := make([]gen.AdminUser, 0, len(rows))
	for _, r := range rows {
		data = append(data, toAdminUser(adminUserRow(r)))
	}
	return gen.ListAdminUsers200JSONResponse{Data: data, Meta: pageMeta(page, perPage, total)}, nil
}

// GetAdminUser returns one account and everything admins have done to it.
func (s *Server) GetAdminUser(ctx context.Context, request gen.GetAdminUserRequestObject) (gen.GetAdminUserResponseObject, error) {
	if _, ok := requireAdmin(ctx); !ok {
		return unrouted{}, nil
	}

	id := uuid.UUID(request.Id)
	row, err := s.queries.GetUserForAdmin(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return gen.GetAdminUser404JSONResponse{NotFoundJSONResponse: gen.NotFoundJSONResponse(
			errorBody(apierror.CodeNotFound, "No account with that id."))}, nil
	}
	if err != nil {
		s.log.Error("get user for admin", slog.String("error", err.Error()))
		return nil, err
	}

	actions, err := s.queries.ListAdminActionsForSubject(ctx, sqlcgen.ListAdminActionsForSubjectParams{
		SubjectType: db.SubjectUser,
		SubjectID:   id,
	})
	if err != nil {
		s.log.Error("list admin actions for user", slog.String("error", err.Error()))
		return nil, err
	}

	out := gen.AdminUserDetail{
		User:    toAdminUser(adminUserRow(row)),
		Actions: make([]gen.AdminAction, 0, len(actions)),
	}
	for _, a := range actions {
		out.Actions = append(out.Actions, s.toAdminAction(a.AdminAction, a.ActorUsername))
	}
	return gen.GetAdminUser200JSONResponse(out), nil
}

// adminUserRow is the shape both admin account queries return. sqlc gives each
// query its own row type even when the columns match, so they are converted to
// this one and mapped once.
type adminUserRow struct {
	ID            uuid.UUID
	Email         string
	Username      string
	Role          string
	Verified      bool
	CreatedAt     time.Time
	SuspendedAt   *time.Time
	SuspendReason *string
	ListingCount  int64
	RequestCount  int64
	OfferCount    int64
}

func toAdminUser(r adminUserRow) gen.AdminUser {
	u := gen.AdminUser{
		Id:            openapi_types.UUID(r.ID),
		Email:         openapi_types.Email(r.Email),
		Username:      r.Username,
		Role:          gen.AdminUserRole(r.Role),
		Verified:      r.Verified,
		CreatedAt:     r.CreatedAt,
		SuspendReason: nullableString(r.SuspendReason),
		SuspendedAt:   nullable.NewNullNullable[time.Time](),
		ListingCount:  int(r.ListingCount),
		RequestCount:  int(r.RequestCount),
		OfferCount:    int(r.OfferCount),
	}
	if r.SuspendedAt != nil {
		u.SuspendedAt = nullable.NewNullableWithValue(*r.SuspendedAt)
	}
	return u
}
