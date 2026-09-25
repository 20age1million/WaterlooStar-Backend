package dbtest

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/20age1million/WaterlooStar-Backend/internal/db/sqlcgen"
)

// Fixtures go through the real queries rather than raw INSERTs. A fixture that
// bypassed CreateUser would keep working after CreateUser broke.

// User inserts a student. Verified, because most tests are about what a
// verified student can do; pass false when the point is that they cannot.
func User(t *testing.T, q *sqlcgen.Queries, email string, verified bool) sqlcgen.User {
	t.Helper()
	ctx := context.Background()

	user, err := q.CreateUser(ctx, sqlcgen.CreateUserParams{
		Email:    email,
		Username: strings.TrimSuffix(email, "@uwaterloo.ca"),
		// Not a real hash: no test here checks a password, and bcrypt is slow
		// enough to notice when every fixture pays for it.
		PasswordHash: "$2a$12$fixture.not.a.real.hash.only.a.placeholder.value.aa",
	})
	if err != nil {
		t.Fatalf("fixture user %s: %v", email, err)
	}

	if verified {
		user, err = q.MarkUserVerified(ctx, user.ID)
		if err != nil {
			t.Fatalf("fixture verify %s: %v", email, err)
		}
	}
	return user
}

// ListingOptions is what a test might reasonably vary. Anything left zero takes
// the default below, so a test states only what it is about.
type ListingOptions struct {
	Title      string
	PriceCents int32
	StartDate  time.Time
	EndDate    time.Time
	Status     string
	DistanceM  *int32
	Utilities  []string
	Furnished  bool
	Bedrooms   int32
	UnitType   string
	Neighbour  string
}

// Listing inserts a listing owned by owner.
func Listing(t *testing.T, q *sqlcgen.Queries, owner uuid.UUID, opts ListingOptions) sqlcgen.Listing {
	t.Helper()

	params := sqlcgen.CreateListingParams{
		OwnerID:       owner,
		Title:         orString(opts.Title, "A room in a student house"),
		Body:          "Written by a fixture.",
		Conditions:    []string{},
		PriceCents:    orInt32(opts.PriceCents, 90000),
		StartDate:     orTime(opts.StartDate, date(2027, 1, 1)),
		EndDate:       orTime(opts.EndDate, date(2027, 4, 30)),
		LeaseMonths:   4,
		TermTag:       "Winter term",
		UnitType:      orString(opts.UnitType, "room"),
		BedroomsTotal: orInt32(opts.Bedrooms, 4),
		Bathrooms:     1,
		BathType:      "shared",
		Furnished:     opts.Furnished,
		Utilities:     orStrings(opts.Utilities),
		AddressLine:   "Hazel St",
		Neighbourhood: orString(opts.Neighbour, "Northdale"),
		DistanceM:     opts.DistanceM,
		CommuteMode:   "walk",
		Status:        orString(opts.Status, "published"),
		CreatedAt:     time.Now(),
	}

	listing, err := q.CreateListing(context.Background(), params)
	if err != nil {
		t.Fatalf("fixture listing %q: %v", params.Title, err)
	}
	return listing
}

// Metres is a shorthand for the distance field, which is a pointer because a
// listing may have no recorded distance at all.
func Metres(m int32) *int32 { return &m }

func date(year int, month time.Month, day int) time.Time {
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

func orString(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

func orInt32(v, fallback int32) int32 {
	if v == 0 {
		return fallback
	}
	return v
}

func orTime(v, fallback time.Time) time.Time {
	if v.IsZero() {
		return fallback
	}
	return v
}

func orStrings(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}
