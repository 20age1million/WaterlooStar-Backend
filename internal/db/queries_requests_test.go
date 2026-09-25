package db_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/20age1million/WaterlooStar-Backend/internal/db/dbtest"
	"github.com/20age1million/WaterlooStar-Backend/internal/db/sqlcgen"
)

// The "Looking for Housing" queries.
//
// The filters read from the owner's side and so invert some of the listing
// ones: a budget is a ceiling, a distance is a radius. Those inversions are the
// easiest thing here to get backwards, so each is checked by a case that would
// pass under the opposite reading.

// browseRequests runs the public query and its count, which must agree.
func browseRequests(t *testing.T, q *sqlcgen.Queries, list sqlcgen.ListRequestsParams) ([]sqlcgen.ListRequestsRow, int64) {
	t.Helper()
	ctx := context.Background()

	if list.Sort == "" {
		list.Sort = "new"
	}
	if list.Limit == 0 {
		list.Limit = 50
	}

	rows, err := q.ListRequests(ctx, list)
	if err != nil {
		t.Fatalf("ListRequests: %v", err)
	}

	total, err := q.CountRequests(ctx, sqlcgen.CountRequestsParams{
		Search: list.Search, StartAfter: list.StartAfter, EndBefore: list.EndBefore,
		BudgetMin: list.BudgetMin, BudgetMax: list.BudgetMax, DistanceMin: list.DistanceMin,
		OccupantsMax: list.OccupantsMax, Pets: list.Pets, Furnished: list.Furnished,
		Parking: list.Parking, Laundry: list.Laundry, VerifiedOnly: list.VerifiedOnly,
	})
	if err != nil {
		t.Fatalf("CountRequests: %v", err)
	}

	if int64(len(rows)) != total && int64(len(rows)) < int64(list.Limit) {
		t.Errorf("ListRequests returned %d rows but CountRequests says %d", len(rows), total)
	}
	return rows, total
}

func TestRequestRoundTrip(t *testing.T) {
	t.Parallel()
	q, _ := dbtest.Queries(t)
	ctx := context.Background()

	poster := dbtest.User(t, q, "seeker@uwaterloo.ca", true)
	request := dbtest.Request(t, q, poster.ID, dbtest.RequestOptions{Title: "Room wanted for Winter"})

	// Unlike listings, a request created as published carries published_at.
	if request.PublishedAt == nil {
		t.Error("a request published on creation has no published_at")
	}

	public, err := q.GetPublishedRequest(ctx, request.ID)
	if err != nil {
		t.Fatalf("GetPublishedRequest: %v", err)
	}
	if public.PosterUsername != poster.Username || !public.PosterVerified {
		t.Errorf("poster = %q verified=%v", public.PosterUsername, public.PosterVerified)
	}

	forOwner, err := q.GetRequestForOwner(ctx, request.ID)
	if err != nil || forOwner.ID != request.ID {
		t.Fatalf("GetRequestForOwner: %v", err)
	}

	mine, err := q.ListRequestsByPoster(ctx, poster.ID)
	if err != nil || len(mine) != 1 {
		t.Fatalf("ListRequestsByPoster: %v (%d rows)", err, len(mine))
	}
}

func TestDraftRequestIsPrivateButItsPosterSeesIt(t *testing.T) {
	t.Parallel()
	q, _ := dbtest.Queries(t)
	ctx := context.Background()

	poster := dbtest.User(t, q, "drafter@uwaterloo.ca", true)
	draft := dbtest.Request(t, q, poster.ID, dbtest.RequestOptions{Status: "draft"})

	if draft.PublishedAt != nil {
		t.Error("a draft was stamped as published")
	}
	if _, err := q.GetPublishedRequest(ctx, draft.ID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("a draft was publicly readable: %v", err)
	}

	mine, err := q.ListRequestsByPoster(ctx, poster.ID)
	if err != nil || len(mine) != 1 {
		t.Fatalf("the poster cannot see their own draft: %v (%d rows)", err, len(mine))
	}
}

func TestSetRequestStatusHidesAndRestores(t *testing.T) {
	t.Parallel()
	q, _ := dbtest.Queries(t)
	ctx := context.Background()

	poster := dbtest.User(t, q, "pauser2@uwaterloo.ca", true)
	request := dbtest.Request(t, q, poster.ID, dbtest.RequestOptions{})
	firstPublished := request.PublishedAt

	paused, err := q.SetRequestStatus(ctx, sqlcgen.SetRequestStatusParams{ID: request.ID, Status: "paused"})
	if err != nil {
		t.Fatalf("SetRequestStatus: %v", err)
	}
	if paused.Status != "paused" {
		t.Fatalf("status = %q, want paused", paused.Status)
	}
	if _, err := q.GetPublishedRequest(ctx, request.ID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("a paused request was still public: %v", err)
	}

	republished, err := q.SetRequestStatus(ctx, sqlcgen.SetRequestStatusParams{ID: request.ID, Status: "published"})
	if err != nil {
		t.Fatalf("SetRequestStatus back to published: %v", err)
	}
	// Stamped once: republishing must not make an old post look new.
	if !republished.PublishedAt.Equal(*firstPublished) {
		t.Error("published_at moved when the request was republished")
	}
	if _, err := q.GetPublishedRequest(ctx, request.ID); err != nil {
		t.Fatalf("a republished request was not public: %v", err)
	}
}

func TestUpdateRequestChangesOnlyWhatIsGiven(t *testing.T) {
	t.Parallel()
	q, _ := dbtest.Queries(t)

	poster := dbtest.User(t, q, "editor2@uwaterloo.ca", true)
	request := dbtest.Request(t, q, poster.ID, dbtest.RequestOptions{
		Title: "Room wanted, quiet street", BudgetCents: 90000, Occupants: 2,
	})

	updated, err := q.UpdateRequest(context.Background(), sqlcgen.UpdateRequestParams{
		ID: request.ID, BudgetCents: ptrInt32(105000),
	})
	if err != nil {
		t.Fatalf("UpdateRequest: %v", err)
	}
	if updated.BudgetCents != 105000 {
		t.Errorf("budget = %d, want 105000", updated.BudgetCents)
	}
	if updated.Title != request.Title || updated.Occupants != request.Occupants {
		t.Error("an untouched column changed")
	}
}

func TestBrowseRequestFiltersReadFromTheOwnersSide(t *testing.T) {
	t.Parallel()
	q, _ := dbtest.Queries(t)

	poster := dbtest.User(t, q, "browse@uwaterloo.ca", true)
	// A modest budget, close to campus only.
	dbtest.Request(t, q, poster.ID, dbtest.RequestOptions{
		Title: "Quiet single room near the plaza", BudgetCents: 70000,
		MaxDistanceM: dbtest.Metres(1000), Occupants: 1, Furnished: true,
	})
	// A larger budget, willing to go further out, two people.
	dbtest.Request(t, q, poster.ID, dbtest.RequestOptions{
		Title: "Two roommates want a two-bedroom", BudgetCents: 160000,
		MaxDistanceM: dbtest.Metres(5000), Occupants: 2, Pets: true,
	})
	// No stated radius at all.
	dbtest.Request(t, q, poster.ID, dbtest.RequestOptions{
		Title: "Anywhere on a bus route", BudgetCents: 100000, Occupants: 1,
	})
	dbtest.Request(t, q, poster.ID, dbtest.RequestOptions{Title: "Paused", Status: "paused"})

	if _, total := browseRequests(t, q, sqlcgen.ListRequestsParams{}); total != 3 {
		t.Fatalf("public total = %d, want 3 — a paused request is not public", total)
	}

	cases := map[string]struct {
		params sqlcgen.ListRequestsParams
		want   int64
	}{
		// An owner asking $900 wants everyone whose ceiling reaches it.
		"can afford $900":    {sqlcgen.ListRequestsParams{BudgetMin: ptrInt32(90000)}, 2},
		"can afford $1600":   {sqlcgen.ListRequestsParams{BudgetMin: ptrInt32(160000)}, 1},
		"budget under $1000": {sqlcgen.ListRequestsParams{BudgetMax: ptrInt32(100000)}, 2},
		// A place 3 km out suits anyone whose radius reaches it — and anyone who
		// stated no radius at all, which is not the same as a narrow one.
		"place 3 km out":  {sqlcgen.ListRequestsParams{DistanceMin: ptrInt32(3000)}, 2},
		"place 500 m out": {sqlcgen.ListRequestsParams{DistanceMin: ptrInt32(500)}, 3},
		"one occupant":    {sqlcgen.ListRequestsParams{OccupantsMax: ptrInt32(1)}, 2},
		"pets":            {sqlcgen.ListRequestsParams{Pets: ptrBool(true)}, 1},
		"furnished":       {sqlcgen.ListRequestsParams{Furnished: ptrBool(true)}, 1},
		"parking":         {sqlcgen.ListRequestsParams{Parking: ptrBool(true)}, 0},
		"laundry":         {sqlcgen.ListRequestsParams{Laundry: ptrBool(true)}, 0},
		"verified only":   {sqlcgen.ListRequestsParams{VerifiedOnly: ptrBool(true)}, 3},
		"search hits one": {sqlcgen.ListRequestsParams{Search: ptrString("roommates")}, 1},
		"search misses":   {sqlcgen.ListRequestsParams{Search: ptrString("submarine")}, 0},
		"term window":     {sqlcgen.ListRequestsParams{StartAfter: ptrTime(date(2027, 1, 1)), EndBefore: ptrTime(date(2027, 4, 30))}, 3},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			rows, total := browseRequests(t, q, tc.params)
			if total != tc.want {
				t.Errorf("total = %d, want %d", total, tc.want)
			}
			if int64(len(rows)) != tc.want {
				t.Errorf("rows = %d, want %d", len(rows), tc.want)
			}
		})
	}
}

func TestBrowseRequestSorts(t *testing.T) {
	t.Parallel()
	q, _ := dbtest.Queries(t)

	poster := dbtest.User(t, q, "sorter2@uwaterloo.ca", true)
	dbtest.Request(t, q, poster.ID, dbtest.RequestOptions{
		Title: "Modest", BudgetCents: 70000, StartDate: date(2027, 5, 1), EndDate: date(2027, 8, 31),
	})
	dbtest.Request(t, q, poster.ID, dbtest.RequestOptions{
		Title: "Generous", BudgetCents: 160000, StartDate: date(2027, 1, 1), EndDate: date(2027, 4, 30),
	})

	// Each sort must be a value PostgreSQL accepts in the ORDER BY CASE.
	for _, sort := range []string{"new", "match", "budgetAsc", "budgetDesc", "soonest"} {
		t.Run(sort, func(t *testing.T) {
			rows, _ := browseRequests(t, q, sqlcgen.ListRequestsParams{Sort: sort})
			if len(rows) != 2 {
				t.Fatalf("rows = %d, want 2", len(rows))
			}
			switch sort {
			case "budgetAsc":
				if rows[0].HousingRequest.BudgetCents != 70000 {
					t.Error("budgetAsc did not put the smallest budget first")
				}
			case "budgetDesc":
				if rows[0].HousingRequest.BudgetCents != 160000 {
					t.Error("budgetDesc did not put the largest budget first")
				}
			case "soonest":
				if !rows[0].HousingRequest.StartDate.Equal(date(2027, 1, 1)) {
					t.Error("soonest did not put the earliest start first")
				}
			}
		})
	}
}

func TestDeleteAllRequestsEmptiesTheTable(t *testing.T) {
	t.Parallel()
	q, _ := dbtest.Queries(t)

	poster := dbtest.User(t, q, "reseed2@uwaterloo.ca", true)
	dbtest.Request(t, q, poster.ID, dbtest.RequestOptions{})

	if err := q.DeleteAllRequests(context.Background()); err != nil {
		t.Fatalf("DeleteAllRequests: %v", err)
	}
	if _, total := browseRequests(t, q, sqlcgen.ListRequestsParams{}); total != 0 {
		t.Errorf("total = %d after deleting everything", total)
	}
}
