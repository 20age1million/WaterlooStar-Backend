package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/oapi-codegen/nullable"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/20age1million/WaterlooStar-Backend/internal/db/sqlcgen"
	"github.com/20age1million/WaterlooStar-Backend/internal/httpapi/gen"
)

// The operator's overview and the ledger. Read-only in this phase: accounts are
// changed from Phase 10 and posts from Phase 11, and every change they make
// goes through db.Audited, so it appears here.

// GetAdminOverview returns the counts an operator starts from.
func (s *Server) GetAdminOverview(ctx context.Context, _ gen.GetAdminOverviewRequestObject) (gen.GetAdminOverviewResponseObject, error) {
	if _, ok := requireAdmin(ctx); !ok {
		return unrouted{}, nil
	}

	c, err := s.queries.AdminOverviewCounts(ctx)
	if err != nil {
		s.log.Error("admin overview", slog.String("error", err.Error()))
		return nil, err
	}

	return gen.GetAdminOverview200JSONResponse{
		Users: gen.AdminUserCounts{
			Total:     int(c.UsersTotal),
			Verified:  int(c.UsersVerified),
			Suspended: int(c.UsersSuspended),
			Admins:    int(c.UsersAdmin),
		},
		Listings:        gen.AdminPostCounts{Total: int(c.ListingsTotal), Published: int(c.ListingsPublished)},
		Requests:        gen.AdminPostCounts{Total: int(c.RequestsTotal), Published: int(c.RequestsPublished)},
		ActionsLastWeek: int(c.ActionsLastWeek),
	}, nil
}

// ListAdminActions returns a page of the ledger, newest first.
func (s *Server) ListAdminActions(ctx context.Context, request gen.ListAdminActionsRequestObject) (gen.ListAdminActionsResponseObject, error) {
	if _, ok := requireAdmin(ctx); !ok {
		return unrouted{}, nil
	}

	page, perPage := paginationFrom(request.Params.Page, request.Params.PerPage)

	total, err := s.queries.CountAdminActions(ctx)
	if err != nil {
		s.log.Error("count admin actions", slog.String("error", err.Error()))
		return nil, err
	}

	rows, err := s.queries.ListAdminActions(ctx, sqlcgen.ListAdminActionsParams{
		Limit:  int32(perPage),
		Offset: int32((page - 1) * perPage),
	})
	if err != nil {
		s.log.Error("list admin actions", slog.String("error", err.Error()))
		return nil, err
	}

	data := make([]gen.AdminAction, 0, len(rows))
	for _, row := range rows {
		data = append(data, s.toAdminAction(row.AdminAction, row.ActorUsername))
	}

	return gen.ListAdminActions200JSONResponse{Data: data, Meta: pageMeta(page, perPage, total)}, nil
}

// toAdminAction maps a ledger row. A missing actor — the host, or an account
// since deleted — is an explicit null, not an absent field.
func (s *Server) toAdminAction(row sqlcgen.AdminAction, actorUsername *string) gen.AdminAction {
	out := gen.AdminAction{
		Id:          openapi_types.UUID(row.ID),
		Action:      gen.AdminActionAction(row.Action),
		SubjectType: gen.AdminActionSubjectType(row.SubjectType),
		SubjectId:   openapi_types.UUID(row.SubjectID),
		Reason:      row.Reason,
		Detail:      map[string]interface{}{},
		Actor:       nullable.NewNullNullable[gen.AdminActor](),
		CreatedAt:   row.CreatedAt,
	}

	if len(row.Detail) > 0 {
		if err := json.Unmarshal(row.Detail, &out.Detail); err != nil {
			// The column is jsonb and only Audited writes it, so this should not
			// happen; if it does, the entry is still worth showing without it.
			s.log.Error("decode ledger detail", slog.String("id", row.ID.String()), slog.String("error", err.Error()))
			out.Detail = map[string]interface{}{}
		}
	}

	if row.ActorID != nil && actorUsername != nil {
		out.Actor = nullable.NewNullableWithValue(gen.AdminActor{
			Id:       openapi_types.UUID(*row.ActorID),
			Username: *actorUsername,
		})
	}
	return out
}
