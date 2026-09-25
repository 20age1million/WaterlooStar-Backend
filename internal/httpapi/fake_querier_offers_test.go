package httpapi_test

import (
	"context"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/20age1million/WaterlooStar-Backend/internal/db/sqlcgen"
)

// The fake's offers.
//
// The rule it has to keep is the one the SQL keeps with a join: an offer counts
// only while its listing is published and it has not been withdrawn. A fake
// that simply stored a list would let a handler test pass while students saw
// rooms that no longer exist.

func (f *fakeQuerier) CreateOffer(_ context.Context, arg sqlcgen.CreateOfferParams) (sqlcgen.RequestOffer, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	// UNIQUE (request_id, listing_id).
	for _, o := range f.offers {
		if o.RequestID == arg.RequestID && o.ListingID == arg.ListingID {
			return sqlcgen.RequestOffer{}, errUniqueViolation
		}
	}

	now := time.Now()
	offer := sqlcgen.RequestOffer{
		ID:        uuid.New(),
		RequestID: arg.RequestID,
		ListingID: arg.ListingID,
		OwnerID:   arg.OwnerID,
		Note:      arg.Note,
		CreatedAt: now,
		UpdatedAt: now,
	}
	f.offers = append(f.offers, offer)
	return offer, nil
}

// visibleOffers is the join the SQL does: live offers on published listings.
func (f *fakeQuerier) visibleOffers(requestID uuid.UUID) []sqlcgen.RequestOffer {
	out := []sqlcgen.RequestOffer{}
	for _, o := range f.offers {
		if o.RequestID != requestID || o.WithdrawnAt != nil {
			continue
		}
		for _, l := range f.listings {
			if l.ID == o.ListingID && l.Status == "published" {
				out = append(out, o)
				break
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

func (f *fakeQuerier) ListOffersForRequest(_ context.Context, requestID uuid.UUID) ([]sqlcgen.ListOffersForRequestRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	rows := []sqlcgen.ListOffersForRequestRow{}
	for _, o := range f.visibleOffers(requestID) {
		owner := f.users[o.OwnerID]
		for _, l := range f.listings {
			if l.ID == o.ListingID {
				rows = append(rows, sqlcgen.ListOffersForRequestRow{
					RequestOffer:   o,
					Listing:        l,
					OwnerUsername:  owner.Username,
					OwnerAvatarUrl: owner.AvatarUrl,
					OwnerVerified:  owner.Verified,
				})
				break
			}
		}
	}
	return rows, nil
}

func (f *fakeQuerier) CountOffersForRequest(_ context.Context, requestID uuid.UUID) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return int64(len(f.visibleOffers(requestID))), nil
}

func (f *fakeQuerier) GetOffer(_ context.Context, id uuid.UUID) (sqlcgen.RequestOffer, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	for _, o := range f.offers {
		if o.ID == id {
			return o, nil
		}
	}
	return sqlcgen.RequestOffer{}, pgx.ErrNoRows
}

func (f *fakeQuerier) GetOfferForListing(_ context.Context, arg sqlcgen.GetOfferForListingParams) (sqlcgen.RequestOffer, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	for _, o := range f.offers {
		if o.RequestID == arg.RequestID && o.ListingID == arg.ListingID {
			return o, nil
		}
	}
	return sqlcgen.RequestOffer{}, pgx.ErrNoRows
}

func (f *fakeQuerier) WithdrawOffer(_ context.Context, id uuid.UUID) (sqlcgen.RequestOffer, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	for i, o := range f.offers {
		// The SQL matches only offers that are still live, so withdrawing twice
		// finds nothing.
		if o.ID == id && o.WithdrawnAt == nil {
			now := time.Now()
			o.WithdrawnAt = &now
			o.UpdatedAt = now
			f.offers[i] = o
			return o, nil
		}
	}
	return sqlcgen.RequestOffer{}, pgx.ErrNoRows
}

func (f *fakeQuerier) ListOffersByOwner(_ context.Context, ownerID uuid.UUID) ([]sqlcgen.ListOffersByOwnerRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	rows := []sqlcgen.ListOffersByOwnerRow{}
	for _, o := range f.offers {
		if o.OwnerID != ownerID {
			continue
		}
		for _, l := range f.listings {
			if l.ID == o.ListingID {
				rows = append(rows, sqlcgen.ListOffersByOwnerRow{RequestOffer: o, Listing: l})
				break
			}
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		return rows[i].RequestOffer.CreatedAt.After(rows[j].RequestOffer.CreatedAt)
	})
	return rows, nil
}
