package seed

// The "Looking for Housing" fixtures.
//
// Written from the other side of the same market as the listings: the terms,
// budgets and distances line up with places that exist in `listings`, so an
// owner browsing requests finds someone their room would actually suit. A
// seeded market where nothing matches anything would demonstrate the feature
// and hide its point.

type request struct {
	poster string

	title string
	body  string

	budgetCents int32

	startDate   string
	endDate     string
	leaseMonths int32
	termTag     string

	occupants          int32
	pets               bool
	furnishedPreferred bool
	parkingNeeded      bool
	laundryNeeded      bool

	maxDistanceM  *int32
	neighbourhood string

	// Days before now, so "posted 2 days ago" is true of the seed.
	postedDaysAgo int
	views         int32
}

var requests = []request{
	{
		poster: "danielo",
		title:  "Quiet room within walking distance, Winter term",
		body: "Coming back from a Toronto co-op in January. I study late and sleep light, so sound " +
			"isolation matters more to me than square footage. Clean, no parties, happy to sign a " +
			"sublet agreement and pay the term up front if that helps.",
		budgetCents: 90000,
		startDate:   "2027-01-01", endDate: "2027-04-30", leaseMonths: 4, termTag: "Winter term",
		occupants: 1, furnishedPreferred: true,
		maxDistanceM: intPtr(2000), neighbourhood: "Northdale",
		postedDaysAgo: 2, views: 41,
	},
	{
		poster: "priyas",
		title:  "Two roommates looking for a 2-bed from January",
		body: "We have lived together since first year and are both on the same co-op cycle. Tidy, " +
			"quiet, references from our current landlord available. A second bathroom would be a " +
			"luxury but we can share.",
		budgetCents: 175000,
		startDate:   "2027-01-01", endDate: "2027-08-31", leaseMonths: 8, termTag: "8 months",
		occupants: 2, laundryNeeded: true,
		maxDistanceM: intPtr(3000), neighbourhood: "Uptown",
		postedDaysAgo: 4, views: 88,
	},
	{
		poster: "ahmedr",
		title:  "Spring term sublet, anywhere on a bus route",
		body: "Four months in Waterloo for a research term. I do not need to be close to campus as " +
			"long as the bus is direct, which should widen the field a good deal. Parking for a small " +
			"car would help.",
		budgetCents: 80000,
		startDate:   "2027-05-01", endDate: "2027-08-31", leaseMonths: 4, termTag: "Spring term",
		occupants: 1, parkingNeeded: true,
		// No stated radius: this one should appear whatever distance an owner filters by.
		maxDistanceM: nil, neighbourhood: "",
		postedDaysAgo: 6, views: 23,
	},
	{
		poster: "kaylam",
		title:  "Room with a desk for a full year, pets welcome either way",
		body: "Starting a master's in September and would rather sign once than move twice. I have a " +
			"cat, so I need somewhere that allows one — that is the hard constraint, everything else " +
			"is negotiable.",
		budgetCents: 110000,
		startDate:   "2027-09-01", endDate: "2028-08-31", leaseMonths: 12, termTag: "Full year",
		occupants: 1, pets: true, furnishedPreferred: true,
		maxDistanceM: intPtr(1500), neighbourhood: "Northdale",
		postedDaysAgo: 9, views: 64,
	},
	{
		poster: "tomasv",
		title:  "Fall + Winter, studio or a very quiet room",
		body: "I work nights and sleep through part of the morning, which has not gone well in a " +
			"shared house before. A studio is ideal; a room in a quiet place with an understanding " +
			"housemate is fine too.",
		budgetCents: 130000,
		startDate:   "2027-09-01", endDate: "2028-04-30", leaseMonths: 8, termTag: "Fall + Winter",
		occupants: 1,
		// Willing to go a long way for quiet.
		maxDistanceM: intPtr(6000), neighbourhood: "",
		postedDaysAgo: 12, views: 31,
	},
}

func intPtr(v int32) *int32 { return &v }
