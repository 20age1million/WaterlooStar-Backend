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

// The fake's "Looking for Housing" half.
//
// It mirrors what the SQL decides, not what would be convenient here: only
// published rows are public, a budget filter reads as a ceiling, and a request
// with no stated radius passes a distance filter. A looser fake would let a
// handler test pass against behaviour PostgreSQL would reject — which is the
// whole reason internal/db has its own tests.

func (f *fakeQuerier) CreateRequest(_ context.Context, arg sqlcgen.CreateRequestParams) (sqlcgen.HousingRequest, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	now := time.Now()
	row := sqlcgen.HousingRequest{
		ID:                 uuid.New(),
		PosterID:           arg.PosterID,
		Title:              arg.Title,
		Body:               arg.Body,
		BudgetCents:        arg.BudgetCents,
		StartDate:          arg.StartDate,
		EndDate:            arg.EndDate,
		LeaseMonths:        arg.LeaseMonths,
		TermTag:            arg.TermTag,
		Occupants:          arg.Occupants,
		Pets:               arg.Pets,
		FurnishedPreferred: arg.FurnishedPreferred,
		ParkingNeeded:      arg.ParkingNeeded,
		LaundryNeeded:      arg.LaundryNeeded,
		MaxDistanceM:       arg.MaxDistanceM,
		Neighbourhood:      arg.Neighbourhood,
		Status:             arg.Status,
		CreatedAt:          arg.CreatedAt,
		UpdatedAt:          now,
	}
	// Stamped on creation when it goes straight to published, as the SQL does.
	if arg.Status == "published" {
		row.PublishedAt = &now
	}

	f.requests = append(f.requests, row)
	return row, nil
}

func (f *fakeQuerier) requestsMatching(arg sqlcgen.ListRequestsParams) []sqlcgen.HousingRequest {
	out := []sqlcgen.HousingRequest{}
	for _, r := range f.requests {
		if r.Status != "published" {
			continue
		}
		if arg.Search != nil && !strings.Contains(
			strings.ToLower(r.Title+" "+r.Body+" "+r.Neighbourhood),
			strings.ToLower(*arg.Search)) {
			continue
		}
		if arg.StartAfter != nil && r.StartDate.After(*arg.StartAfter) {
			continue
		}
		if arg.EndBefore != nil && r.EndDate.Before(*arg.EndBefore) {
			continue
		}
		if arg.BudgetMin != nil && r.BudgetCents < *arg.BudgetMin {
			continue
		}
		if arg.BudgetMax != nil && r.BudgetCents > *arg.BudgetMax {
			continue
		}
		// A request with no stated radius passes: no preference is not a narrow
		// one. The opposite of how a listing's unknown distance is treated.
		if arg.DistanceMin != nil && r.MaxDistanceM != nil && *r.MaxDistanceM < *arg.DistanceMin {
			continue
		}
		if arg.OccupantsMax != nil && r.Occupants > *arg.OccupantsMax {
			continue
		}
		if arg.Pets != nil && r.Pets != *arg.Pets {
			continue
		}
		if arg.Furnished != nil && r.FurnishedPreferred != *arg.Furnished {
			continue
		}
		if arg.Parking != nil && r.ParkingNeeded != *arg.Parking {
			continue
		}
		if arg.Laundry != nil && r.LaundryNeeded != *arg.Laundry {
			continue
		}
		if arg.VerifiedOnly != nil && *arg.VerifiedOnly && !f.users[r.PosterID].Verified {
			continue
		}
		out = append(out, r)
	}

	sort.SliceStable(out, func(i, j int) bool {
		switch arg.Sort {
		case "budgetAsc":
			return out[i].BudgetCents < out[j].BudgetCents
		case "budgetDesc":
			return out[i].BudgetCents > out[j].BudgetCents
		case "soonest":
			return out[i].StartDate.Before(out[j].StartDate)
		default:
			return out[i].CreatedAt.After(out[j].CreatedAt)
		}
	})
	return out
}

func (f *fakeQuerier) CountRequests(_ context.Context, arg sqlcgen.CountRequestsParams) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	return int64(len(f.requestsMatching(sqlcgen.ListRequestsParams{
		Search: arg.Search, StartAfter: arg.StartAfter, EndBefore: arg.EndBefore,
		BudgetMin: arg.BudgetMin, BudgetMax: arg.BudgetMax, DistanceMin: arg.DistanceMin,
		OccupantsMax: arg.OccupantsMax, Pets: arg.Pets, Furnished: arg.Furnished,
		Parking: arg.Parking, Laundry: arg.Laundry, VerifiedOnly: arg.VerifiedOnly,
	}))), nil
}

func (f *fakeQuerier) ListRequests(_ context.Context, arg sqlcgen.ListRequestsParams) ([]sqlcgen.ListRequestsRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	all := f.requestsMatching(arg)
	start := min(int(arg.Offset), len(all))
	end := min(start+int(arg.Limit), len(all))

	rows := []sqlcgen.ListRequestsRow{}
	for _, r := range all[start:end] {
		rows = append(rows, f.requestRow(r))
	}
	return rows, nil
}

func (f *fakeQuerier) requestRow(r sqlcgen.HousingRequest) sqlcgen.ListRequestsRow {
	poster := f.users[r.PosterID]
	return sqlcgen.ListRequestsRow{
		HousingRequest:  r,
		PosterUsername:  poster.Username,
		PosterAvatarUrl: poster.AvatarUrl,
		PosterVerified:  poster.Verified,
		// Counted, not stored — the same subquery the SQL runs. Leaving this
		// zero would let a handler test agree with a number no student sees.
		OfferCount: int64(len(f.visibleOffers(r.ID))),
	}
}

func (f *fakeQuerier) GetPublishedRequest(_ context.Context, id uuid.UUID) (sqlcgen.GetPublishedRequestRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	for _, r := range f.requests {
		if r.ID == id && r.Status == "published" {
			row := f.requestRow(r)
			return sqlcgen.GetPublishedRequestRow(row), nil
		}
	}
	return sqlcgen.GetPublishedRequestRow{}, pgx.ErrNoRows
}

func (f *fakeQuerier) GetRequestForOwner(_ context.Context, id uuid.UUID) (sqlcgen.HousingRequest, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	for _, r := range f.requests {
		if r.ID == id {
			return r, nil
		}
	}
	return sqlcgen.HousingRequest{}, pgx.ErrNoRows
}

func (f *fakeQuerier) ListRequestsByPoster(_ context.Context, posterID uuid.UUID) ([]sqlcgen.ListRequestsByPosterRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	// Every status, not just published — that is the point of this query.
	rows := []sqlcgen.ListRequestsByPosterRow{}
	for _, r := range f.requests {
		if r.PosterID == posterID {
			rows = append(rows, sqlcgen.ListRequestsByPosterRow(f.requestRow(r)))
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		return rows[i].HousingRequest.CreatedAt.After(rows[j].HousingRequest.CreatedAt)
	})
	return rows, nil
}

func (f *fakeQuerier) UpdateRequest(_ context.Context, arg sqlcgen.UpdateRequestParams) (sqlcgen.HousingRequest, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	for i, r := range f.requests {
		if r.ID != arg.ID {
			continue
		}
		// coalesce: anything not given keeps its current value.
		if arg.Title != nil {
			r.Title = *arg.Title
		}
		if arg.Body != nil {
			r.Body = *arg.Body
		}
		if arg.BudgetCents != nil {
			r.BudgetCents = *arg.BudgetCents
		}
		if arg.StartDate != nil {
			r.StartDate = *arg.StartDate
		}
		if arg.EndDate != nil {
			r.EndDate = *arg.EndDate
		}
		if arg.LeaseMonths != nil {
			r.LeaseMonths = *arg.LeaseMonths
		}
		if arg.TermTag != nil {
			r.TermTag = *arg.TermTag
		}
		if arg.Occupants != nil {
			r.Occupants = *arg.Occupants
		}
		if arg.Pets != nil {
			r.Pets = *arg.Pets
		}
		if arg.FurnishedPreferred != nil {
			r.FurnishedPreferred = *arg.FurnishedPreferred
		}
		if arg.ParkingNeeded != nil {
			r.ParkingNeeded = *arg.ParkingNeeded
		}
		if arg.LaundryNeeded != nil {
			r.LaundryNeeded = *arg.LaundryNeeded
		}
		if arg.MaxDistanceM != nil {
			r.MaxDistanceM = arg.MaxDistanceM
		}
		if arg.Neighbourhood != nil {
			r.Neighbourhood = *arg.Neighbourhood
		}
		r.UpdatedAt = time.Now()

		f.requests[i] = r
		return r, nil
	}
	return sqlcgen.HousingRequest{}, pgx.ErrNoRows
}

func (f *fakeQuerier) SetRequestStatus(_ context.Context, arg sqlcgen.SetRequestStatusParams) (sqlcgen.HousingRequest, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	for i, r := range f.requests {
		if r.ID != arg.ID {
			continue
		}
		r.Status = arg.Status
		// Stamped the first time it goes live and never moved.
		if arg.Status == "published" && r.PublishedAt == nil {
			now := time.Now()
			r.PublishedAt = &now
		}
		r.UpdatedAt = time.Now()

		f.requests[i] = r
		return r, nil
	}
	return sqlcgen.HousingRequest{}, pgx.ErrNoRows
}

func (f *fakeQuerier) DeleteAllRequests(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = nil
	return nil
}
