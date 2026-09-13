// Package seed loads the six prototype listings into a development database.
//
// These are the fixtures from Waterloostar-web/src/data/housing.ts, with their
// display strings taken apart into columns: "Jan 1 – Apr 30 · 4-month sublet"
// becomes start_date, end_date and lease_months; "1 of 4 bed" becomes
// bedroom_of and bedrooms_total; "Internet incl." becomes an entry in
// utilities. Reassembling those strings is the frontend's job.
//
// Re-running is safe: every listing is deleted and rewritten, and seed users are
// created only if their address is not already taken.
package seed

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/20age1million/waterloostar-api/internal/auth"
	"github.com/20age1million/waterloostar-api/internal/db/sqlcgen"
)

// seedPassword is the password every seeded account shares, so the six posters
// can be logged into while developing. Development data only.
const seedPassword = "seed account password"

type poster struct {
	username string
	email    string
}

type listing struct {
	poster           string
	title            string
	body             string
	conditions       []string
	priceCents       int32
	depositCents     *int32
	startDate        string
	endDate          string
	leaseMonths      int32
	termTag          string
	unitType         string
	bedroomsTotal    int32
	bedroomOf        *int32
	bathrooms        float64
	bathType         string
	furnished        bool
	utilities        []string
	parking          bool
	pets             bool
	laundry          bool
	addressLine      string
	neighbourhood    string
	distanceM        int32
	commuteMinutes   int32
	commuteMode      string
	minutesToTransit *int32
	minutesToGrocery *int32
	views            int32
	replies          int32
	postedDaysAgo    int
}

func ptr[T any](v T) *T { return &v }

var posters = []poster{
	{username: "meil", email: "meil@uwaterloo.ca"},
	{username: "danielo", email: "danielo@uwaterloo.ca"},
	{username: "priyas", email: "priyas@uwaterloo.ca"},
	{username: "ahmedr", email: "ahmedr@uwaterloo.ca"},
	{username: "kaylam", email: "kaylam@uwaterloo.ca"},
	{username: "tomasv", email: "tomasv@uwaterloo.ca"},
}

var listings = []listing{
	{
		poster: "meil",
		title:  "Bright room in a 4-bed student house",
		body: "I am heading to Ottawa for a Winter co-op and need someone to take my room from January. " +
			"It is the back bedroom on the second floor, so you get the quiet side of the house — no street " +
			"noise, and the room holds a double bed, a desk and a dresser with room left over.\n\n" +
			"Three housemates are staying: two in 3B Environment and one in 2A Math. Everyone is fairly quiet, " +
			"cooks their own food and cleans the kitchen on a rota. The bus stop on University Ave is a " +
			"two-minute walk if you would rather not walk in February.",
		conditions: []string{
			"I would like to leave a few boxes and my winter tires in the basement storage.",
			"Sublet runs through the landlord's standard agreement — I will send the template before anything is signed.",
			"Looking for another student. Happy to come down on rent slightly for someone the house gets on with.",
			"No smoking inside. Pets would need the landlord's approval.",
		},
		priceCents: 84500, depositCents: ptr(int32(84500)),
		startDate: "2027-01-01", endDate: "2027-04-30", leaseMonths: 4, termTag: "Winter term",
		unitType: "room", bedroomsTotal: 4, bedroomOf: ptr(int32(1)), bathrooms: 1, bathType: "shared",
		furnished: true, utilities: []string{"internet", "hydro", "water", "heat"},
		addressLine: "Hazel St", neighbourhood: "Northdale", distanceM: 800,
		commuteMinutes: 9, commuteMode: "walk",
		minutesToTransit: ptr(int32(2)), minutesToGrocery: ptr(int32(6)),
		views: 214, replies: 6, postedDaysAgo: 2,
	},
	{
		poster: "danielo",
		title:  "Quiet 1-bedroom basement apartment",
		body: "Separate entrance at the side of the house, so you come and go without passing anyone. " +
			"The ceiling is a little low at the far end but the bedroom itself is a good size and the " +
			"windows are above ground.",
		conditions: []string{"Eight-month lease, no shorter.", "No smoking."},
		priceCents: 110000,
		startDate:  "2027-01-01", endDate: "2027-08-31", leaseMonths: 8, termTag: "8 months",
		unitType: "unit", bedroomsTotal: 1, bathrooms: 1, bathType: "private",
		furnished: false, utilities: []string{"hydro"},
		addressLine: "Regina St N", neighbourhood: "Uptown", distanceM: 1300,
		commuteMinutes: 14, commuteMode: "walk",
		minutesToTransit: ptr(int32(4)), minutesToGrocery: ptr(int32(3)),
		views: 98, replies: 3, postedDaysAgo: 1,
	},
	{
		poster: "priyas",
		title:  "Private room, sound-isolated corner unit",
		body: "Corner room on the top floor with no shared walls to another bedroom. I put acoustic panels " +
			"on the one wall that adjoins the hallway and they are staying. Ensuite bathroom, so no queue " +
			"in the morning.",
		conditions: []string{"Spring term only.", "Quiet hours after 11pm — the house keeps to them."},
		priceCents: 76000,
		startDate:  "2027-05-01", endDate: "2027-08-31", leaseMonths: 4, termTag: "Spring term",
		unitType: "room", bedroomsTotal: 5, bedroomOf: ptr(int32(1)), bathrooms: 1, bathType: "ensuite",
		furnished: true, utilities: []string{"internet", "hydro", "water", "heat", "gas"},
		addressLine: "Lester St", neighbourhood: "Northdale", distanceM: 500,
		commuteMinutes: 6, commuteMode: "walk",
		minutesToTransit: ptr(int32(3)), minutesToGrocery: ptr(int32(8)),
		views: 331, replies: 11, postedDaysAgo: 4,
	},
	{
		poster: "ahmedr",
		title:  "Two-bedroom unit, ideal for a pair of co-ops",
		body: "Whole two-bedroom on the second floor of a small building. Both bedrooms fit a double bed " +
			"and a desk. One parking spot comes with it, which is rare this close to a bus route.",
		conditions: []string{"Two occupants maximum.", "Parking spot is for one car."},
		priceCents: 165000, depositCents: ptr(int32(165000)),
		startDate: "2027-09-01", endDate: "2028-04-30", leaseMonths: 8, termTag: "Fall + Winter",
		unitType: "unit", bedroomsTotal: 2, bathrooms: 1, bathType: "private",
		furnished: true, utilities: []string{"water", "heat"}, parking: true,
		addressLine: "King St N", neighbourhood: "Uptown", distanceM: 2600,
		commuteMinutes: 18, commuteMode: "bus",
		minutesToTransit: ptr(int32(1)), minutesToGrocery: ptr(int32(4)),
		views: 142, replies: 2, postedDaysAgo: 6,
	},
	{
		poster: "kaylam",
		title:  "Room in a townhouse with three grad students",
		body: "Quiet house — everyone here is writing a thesis and keeps sensible hours. In-unit laundry, " +
			"and the landlord is fine with cats.",
		conditions: []string{"Cats are fine, dogs would need a conversation.", "Shared cleaning rota."},
		priceCents: 70000,
		startDate:  "2027-01-01", endDate: "2027-04-30", leaseMonths: 4, termTag: "Winter term",
		unitType: "room", bedroomsTotal: 4, bedroomOf: ptr(int32(1)), bathrooms: 1.5, bathType: "shared",
		furnished: false, utilities: []string{"internet", "water"}, pets: true, laundry: true,
		addressLine: "Columbia St W", neighbourhood: "Westmount", distanceM: 1100,
		commuteMinutes: 11, commuteMode: "walk",
		minutesToTransit: ptr(int32(5)), minutesToGrocery: ptr(int32(7)),
		views: 187, replies: 8, postedDaysAgo: 3,
	},
	{
		poster: "tomasv",
		title:  "Studio with a desk wall and blackout blinds",
		body: "Purpose-built studio: one room, kitchenette along the wall, own bathroom. The previous " +
			"tenant built a desk the full width of the window wall and it stays. Blackout blinds throughout, " +
			"which matters if you are working nights.",
		conditions: []string{"Full-year lease.", "No subletting on."},
		priceCents: 129000, depositCents: ptr(int32(129000)),
		startDate: "2027-01-01", endDate: "2027-12-31", leaseMonths: 12, termTag: "Full year",
		unitType: "studio", bedroomsTotal: 0, bathrooms: 1, bathType: "private",
		furnished: true, utilities: []string{"internet", "hydro", "water", "heat"},
		addressLine: "University Ave W", neighbourhood: "Northdale", distanceM: 600,
		commuteMinutes: 7, commuteMode: "walk",
		minutesToTransit: ptr(int32(2)), minutesToGrocery: ptr(int32(9)),
		views: 256, replies: 5, postedDaysAgo: 5,
	},
}

// Run loads the development fixtures. Safe to run repeatedly.
func Run(ctx context.Context, pool *pgxpool.Pool) (int, error) {
	q := sqlcgen.New(pool)

	hash, err := auth.HashPassword(seedPassword)
	if err != nil {
		return 0, fmt.Errorf("hash seed password: %w", err)
	}

	owners := map[string]uuid.UUID{}
	for _, p := range posters {
		user, err := q.GetUserByEmail(ctx, p.email)
		if errors.Is(err, pgx.ErrNoRows) {
			user, err = q.CreateUser(ctx, sqlcgen.CreateUserParams{
				Email:        p.email,
				Username:     p.username,
				PasswordHash: hash,
			})
			if err != nil {
				return 0, fmt.Errorf("create seed user %s: %w", p.username, err)
			}
			// Seed posters are verified; an unverified poster could not have
			// published in the first place.
			if _, err := q.MarkUserVerified(ctx, user.ID); err != nil {
				return 0, fmt.Errorf("verify seed user %s: %w", p.username, err)
			}
		} else if err != nil {
			return 0, fmt.Errorf("look up seed user %s: %w", p.username, err)
		}
		owners[p.username] = user.ID
	}

	// Rewritten wholesale rather than upserted, so editing a fixture above is
	// reflected exactly on the next run.
	if err := q.DeleteAllListings(ctx); err != nil {
		return 0, fmt.Errorf("clear listings: %w", err)
	}

	now := time.Now()
	for _, l := range listings {
		ownerID, ok := owners[l.poster]
		if !ok {
			return 0, fmt.Errorf("listing %q references unknown poster %q", l.title, l.poster)
		}

		start, err := time.Parse(time.DateOnly, l.startDate)
		if err != nil {
			return 0, fmt.Errorf("listing %q start date: %w", l.title, err)
		}
		end, err := time.Parse(time.DateOnly, l.endDate)
		if err != nil {
			return 0, fmt.Errorf("listing %q end date: %w", l.title, err)
		}

		if _, err := q.CreateListing(ctx, sqlcgen.CreateListingParams{
			OwnerID:       ownerID,
			Title:         l.title,
			Body:          l.body,
			Conditions:    l.conditions,
			PriceCents:    l.priceCents,
			DepositCents:  l.depositCents,
			StartDate:     start,
			EndDate:       end,
			LeaseMonths:   l.leaseMonths,
			TermTag:       l.termTag,
			UnitType:      l.unitType,
			BedroomsTotal: l.bedroomsTotal,
			BedroomOf:     l.bedroomOf,
			Bathrooms:     l.bathrooms,
			BathType:      l.bathType,
			Furnished:     l.furnished,
			Utilities:     l.utilities,
			Parking:       l.parking,
			Pets:          l.pets,
			Laundry:       l.laundry,
			AddressLine:   l.addressLine,
			Neighbourhood: l.neighbourhood,
			DistanceM:     ptr(l.distanceM),
			// Coordinates need a geocoding provider, which is deliberately
			// deferred. The map placeholder does not read them yet.
			Lat:              nil,
			Lng:              nil,
			CommuteMinutes:   ptr(l.commuteMinutes),
			CommuteMode:      l.commuteMode,
			MinutesToTransit: l.minutesToTransit,
			MinutesToGrocery: l.minutesToGrocery,
			Status:           "published",
			Views:            l.views,
			Replies:          l.replies,
			// Backdated so "posted 2 days ago" is true of the data rather than
			// of the moment the seed ran.
			CreatedAt: now.AddDate(0, 0, -l.postedDaysAgo),
		}); err != nil {
			return 0, fmt.Errorf("create listing %q: %w", l.title, err)
		}
	}

	return len(listings), nil
}
