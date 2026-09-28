package httpapi_test

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/20age1million/WaterlooStar-Backend/internal/db/sqlcgen"
)

// The admin queries, in memory. The filters mirror admin.sql; the query tests
// in internal/db are what prove the SQL itself.

// adminRowFor builds the operator's view of one account, counting its posts in
// every status as the real query does.
func (f *fakeQuerier) adminRowFor(u sqlcgen.User) sqlcgen.GetUserForAdminRow {
	row := sqlcgen.GetUserForAdminRow{
		ID: u.ID, Email: u.Email, Username: u.Username, Role: u.Role,
		Verified: u.Verified, CreatedAt: u.CreatedAt,
		SuspendedAt: u.SuspendedAt, SuspendReason: u.SuspendReason,
	}
	for _, l := range f.listings {
		if l.OwnerID == u.ID {
			row.ListingCount++
		}
	}
	for _, r := range f.requests {
		if r.PosterID == u.ID {
			row.RequestCount++
		}
	}
	for _, o := range f.offers {
		if o.OwnerID == u.ID {
			row.OfferCount++
		}
	}
	return row
}

func (f *fakeQuerier) matchingUsers(search, role *string, verified, suspended *bool) []sqlcgen.User {
	var out []sqlcgen.User
	for _, u := range f.users {
		if search != nil {
			s := strings.ToLower(*search)
			if !strings.Contains(strings.ToLower(u.Email), s) && !strings.Contains(strings.ToLower(u.Username), s) {
				continue
			}
		}
		if role != nil && u.Role != *role {
			continue
		}
		if verified != nil && u.Verified != *verified {
			continue
		}
		if suspended != nil && (u.SuspendedAt != nil) != *suspended {
			continue
		}
		out = append(out, u)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.After(out[j].CreatedAt)
		}
		return out[i].ID.String() < out[j].ID.String()
	})
	return out
}

func (f *fakeQuerier) ListUsersForAdmin(_ context.Context, arg sqlcgen.ListUsersForAdminParams) ([]sqlcgen.ListUsersForAdminRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	all := f.matchingUsers(arg.Search, arg.Role, arg.Verified, arg.Suspended)
	out := []sqlcgen.ListUsersForAdminRow{}
	for i := int(arg.Offset); i < len(all) && len(out) < int(arg.Limit); i++ {
		out = append(out, sqlcgen.ListUsersForAdminRow(f.adminRowFor(all[i])))
	}
	return out, nil
}

func (f *fakeQuerier) CountUsersForAdmin(_ context.Context, arg sqlcgen.CountUsersForAdminParams) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return int64(len(f.matchingUsers(arg.Search, arg.Role, arg.Verified, arg.Suspended))), nil
}

func (f *fakeQuerier) GetUserForAdmin(_ context.Context, id uuid.UUID) (sqlcgen.GetUserForAdminRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	u, ok := f.users[id]
	if !ok {
		return sqlcgen.GetUserForAdminRow{}, pgx.ErrNoRows
	}
	return f.adminRowFor(u), nil
}

func (f *fakeQuerier) CountAdmins(context.Context) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var n int64
	for _, u := range f.users {
		if u.Role == "admin" {
			n++
		}
	}
	return n, nil
}

func (f *fakeQuerier) SetUserRole(_ context.Context, arg sqlcgen.SetUserRoleParams) (sqlcgen.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	u, ok := f.users[arg.ID]
	if !ok {
		return sqlcgen.User{}, pgx.ErrNoRows
	}
	u.Role = arg.Role
	u.UpdatedAt = time.Now()
	f.users[arg.ID] = u
	return u, nil
}

func (f *fakeQuerier) InsertAdminAction(_ context.Context, arg sqlcgen.InsertAdminActionParams) (sqlcgen.AdminAction, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	a := sqlcgen.AdminAction{
		ID: uuid.New(), ActorID: arg.ActorID, Action: arg.Action,
		SubjectType: arg.SubjectType, SubjectID: arg.SubjectID,
		Reason: arg.Reason, Detail: arg.Detail, CreatedAt: time.Now(),
	}
	f.adminActions = append(f.adminActions, a)
	return a, nil
}

// actionsNewestFirst is the ledger in the real query's order. Appended in time
// order, so newest first is simply reversed.
func (f *fakeQuerier) actionsNewestFirst() []sqlcgen.AdminAction {
	out := make([]sqlcgen.AdminAction, len(f.adminActions))
	for i, a := range f.adminActions {
		out[len(out)-1-i] = a
	}
	return out
}

func (f *fakeQuerier) actorName(id *uuid.UUID) *string {
	if id == nil {
		return nil
	}
	if u, ok := f.users[*id]; ok {
		name := u.Username
		return &name
	}
	return nil
}

func (f *fakeQuerier) ListAdminActions(_ context.Context, arg sqlcgen.ListAdminActionsParams) ([]sqlcgen.ListAdminActionsRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	all := f.actionsNewestFirst()
	out := []sqlcgen.ListAdminActionsRow{}
	for i := int(arg.Offset); i < len(all) && len(out) < int(arg.Limit); i++ {
		out = append(out, sqlcgen.ListAdminActionsRow{AdminAction: all[i], ActorUsername: f.actorName(all[i].ActorID)})
	}
	return out, nil
}

func (f *fakeQuerier) CountAdminActions(context.Context) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return int64(len(f.adminActions)), nil
}

func (f *fakeQuerier) ListAdminActionsForSubject(_ context.Context, arg sqlcgen.ListAdminActionsForSubjectParams) ([]sqlcgen.ListAdminActionsForSubjectRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []sqlcgen.ListAdminActionsForSubjectRow{}
	for _, a := range f.actionsNewestFirst() {
		if a.SubjectType == arg.SubjectType && a.SubjectID == arg.SubjectID {
			out = append(out, sqlcgen.ListAdminActionsForSubjectRow{AdminAction: a, ActorUsername: f.actorName(a.ActorID)})
		}
	}
	return out, nil
}

func (f *fakeQuerier) AdminOverviewCounts(context.Context) (sqlcgen.AdminOverviewCountsRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var c sqlcgen.AdminOverviewCountsRow
	for _, u := range f.users {
		c.UsersTotal++
		if u.Verified {
			c.UsersVerified++
		}
		if u.SuspendedAt != nil {
			c.UsersSuspended++
		}
		if u.Role == "admin" {
			c.UsersAdmin++
		}
	}
	for _, l := range f.listings {
		c.ListingsTotal++
		if l.Status == "published" {
			c.ListingsPublished++
		}
	}
	for _, r := range f.requests {
		c.RequestsTotal++
		if r.Status == "published" {
			c.RequestsPublished++
		}
	}
	weekAgo := time.Now().Add(-7 * 24 * time.Hour)
	for _, a := range f.adminActions {
		if a.CreatedAt.After(weekAgo) {
			c.ActionsLastWeek++
		}
	}
	return c, nil
}

func (f *fakeQuerier) SuspendUser(_ context.Context, arg sqlcgen.SuspendUserParams) (sqlcgen.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	u, ok := f.users[arg.ID]
	if !ok || u.SuspendedAt != nil {
		return sqlcgen.User{}, pgx.ErrNoRows
	}
	now := time.Now()
	u.SuspendedAt, u.SuspendedBy, u.SuspendReason = &now, arg.ActorID, arg.Reason
	f.users[arg.ID] = u
	return u, nil
}

func (f *fakeQuerier) ReinstateUser(_ context.Context, id uuid.UUID) (sqlcgen.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	u, ok := f.users[id]
	if !ok || u.SuspendedAt == nil {
		return sqlcgen.User{}, pgx.ErrNoRows
	}
	u.SuspendedAt, u.SuspendedBy, u.SuspendReason = nil, nil, nil
	f.users[id] = u
	return u, nil
}

func (f *fakeQuerier) VerifyUserAsAdmin(_ context.Context, id uuid.UUID) (sqlcgen.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	u, ok := f.users[id]
	if !ok || u.Verified {
		return sqlcgen.User{}, pgx.ErrNoRows
	}
	u.Verified = true
	f.users[id] = u
	return u, nil
}

// ------------------------------------------------------ post moderation

func (f *fakeQuerier) adminListings(search, status *string, removed *bool, owner *uuid.UUID) []sqlcgen.Listing {
	var out []sqlcgen.Listing
	for _, l := range f.listings {
		if search != nil && !strings.Contains(strings.ToLower(l.Title+" "+l.Body+" "+l.Neighbourhood), strings.ToLower(*search)) {
			continue
		}
		if status != nil && l.Status != *status {
			continue
		}
		if removed != nil && (l.RemovedAt != nil) != *removed {
			continue
		}
		if owner != nil && l.OwnerID != *owner {
			continue
		}
		out = append(out, l)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

func (f *fakeQuerier) ListListingsForAdmin(_ context.Context, arg sqlcgen.ListListingsForAdminParams) ([]sqlcgen.ListListingsForAdminRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	all := f.adminListings(arg.Search, arg.Status, arg.Removed, arg.OwnerID)
	out := []sqlcgen.ListListingsForAdminRow{}
	for i := int(arg.Offset); i < len(all) && len(out) < int(arg.Limit); i++ {
		u := f.users[all[i].OwnerID]
		out = append(out, sqlcgen.ListListingsForAdminRow{
			Listing: all[i], OwnerUsername: u.Username, OwnerEmail: u.Email,
			OwnerAvatarUrl: u.AvatarUrl, OwnerVerified: u.Verified, OwnerSuspendedAt: u.SuspendedAt,
		})
	}
	return out, nil
}

func (f *fakeQuerier) CountListingsForAdmin(_ context.Context, arg sqlcgen.CountListingsForAdminParams) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return int64(len(f.adminListings(arg.Search, arg.Status, arg.Removed, arg.OwnerID))), nil
}

func (f *fakeQuerier) adminRequests(search, status *string, removed *bool, owner *uuid.UUID) []sqlcgen.HousingRequest {
	var out []sqlcgen.HousingRequest
	for _, r := range f.requests {
		if search != nil && !strings.Contains(strings.ToLower(r.Title+" "+r.Body+" "+r.Neighbourhood), strings.ToLower(*search)) {
			continue
		}
		if status != nil && r.Status != *status {
			continue
		}
		if removed != nil && (r.RemovedAt != nil) != *removed {
			continue
		}
		if owner != nil && r.PosterID != *owner {
			continue
		}
		out = append(out, r)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

func (f *fakeQuerier) ListRequestsForAdmin(_ context.Context, arg sqlcgen.ListRequestsForAdminParams) ([]sqlcgen.ListRequestsForAdminRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	all := f.adminRequests(arg.Search, arg.Status, arg.Removed, arg.OwnerID)
	out := []sqlcgen.ListRequestsForAdminRow{}
	for i := int(arg.Offset); i < len(all) && len(out) < int(arg.Limit); i++ {
		u := f.users[all[i].PosterID]
		out = append(out, sqlcgen.ListRequestsForAdminRow{
			HousingRequest: all[i], PosterUsername: u.Username, PosterEmail: u.Email,
			PosterAvatarUrl: u.AvatarUrl, PosterVerified: u.Verified, PosterSuspendedAt: u.SuspendedAt,
			OfferCount: int64(len(f.visibleOffers(all[i].ID))),
		})
	}
	return out, nil
}

func (f *fakeQuerier) CountRequestsForAdmin(_ context.Context, arg sqlcgen.CountRequestsForAdminParams) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return int64(len(f.adminRequests(arg.Search, arg.Status, arg.Removed, arg.OwnerID))), nil
}

func (f *fakeQuerier) RemoveListing(_ context.Context, arg sqlcgen.RemoveListingParams) (sqlcgen.Listing, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, l := range f.listings {
		if l.ID == arg.ID && l.RemovedAt == nil {
			now := time.Now()
			l.RemovedAt, l.RemovedBy, l.RemovedReason = &now, arg.ActorID, arg.Reason
			f.listings[i] = l
			return l, nil
		}
	}
	return sqlcgen.Listing{}, pgx.ErrNoRows
}

func (f *fakeQuerier) RestoreListing(_ context.Context, id uuid.UUID) (sqlcgen.Listing, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, l := range f.listings {
		if l.ID == id && l.RemovedAt != nil {
			l.RemovedAt, l.RemovedBy, l.RemovedReason = nil, nil, nil
			f.listings[i] = l
			return l, nil
		}
	}
	return sqlcgen.Listing{}, pgx.ErrNoRows
}

func (f *fakeQuerier) RemoveRequest(_ context.Context, arg sqlcgen.RemoveRequestParams) (sqlcgen.HousingRequest, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, r := range f.requests {
		if r.ID == arg.ID && r.RemovedAt == nil {
			now := time.Now()
			r.RemovedAt, r.RemovedBy, r.RemovedReason = &now, arg.ActorID, arg.Reason
			f.requests[i] = r
			return r, nil
		}
	}
	return sqlcgen.HousingRequest{}, pgx.ErrNoRows
}

func (f *fakeQuerier) RestoreRequest(_ context.Context, id uuid.UUID) (sqlcgen.HousingRequest, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, r := range f.requests {
		if r.ID == id && r.RemovedAt != nil {
			r.RemovedAt, r.RemovedBy, r.RemovedReason = nil, nil, nil
			f.requests[i] = r
			return r, nil
		}
	}
	return sqlcgen.HousingRequest{}, pgx.ErrNoRows
}
