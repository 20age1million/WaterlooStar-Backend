package httpapi_test

import (
	"context"
	"encoding/hex"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/20age1million/WaterlooStar-Backend/internal/db/sqlcgen"
)

// fakeQuerier is an in-memory stand-in for the generated Querier.
//
// It enforces the invariants the real SQL enforces — unique lower(email) and
// lower(username), and the consumed/revoked/expired filtering that the "GetLive"
// queries do in their WHERE clauses — because those are exactly what the handler
// tests are checking the handlers rely on. Anything looser would let a test pass
// against behaviour the database would reject.
type fakeQuerier struct {
	mu sync.Mutex

	users             map[uuid.UUID]sqlcgen.User
	verificationToken map[string]sqlcgen.EmailVerificationToken
	resetToken        map[string]sqlcgen.PasswordResetToken
	refreshToken      map[string]sqlcgen.RefreshToken
	listings          []sqlcgen.Listing
	requests          []sqlcgen.HousingRequest

	pingErr error
}

func newFakeQuerier() *fakeQuerier {
	return &fakeQuerier{
		users:             map[uuid.UUID]sqlcgen.User{},
		verificationToken: map[string]sqlcgen.EmailVerificationToken{},
		resetToken:        map[string]sqlcgen.PasswordResetToken{},
		refreshToken:      map[string]sqlcgen.RefreshToken{},
	}
}

func key(hash []byte) string { return hex.EncodeToString(hash) }

// errUniqueViolation is what PostgreSQL returns when a unique index is breached.
// The handler recognises this SQLSTATE and turns it into a 409, so the fake has
// to produce the real error type rather than a generic one.
var errUniqueViolation = &pgconn.PgError{
	Code:    "23505",
	Message: "duplicate key value violates unique constraint",
}

func (f *fakeQuerier) Ping(context.Context) (int32, error) {
	if f.pingErr != nil {
		return 0, f.pingErr
	}
	return 1, nil
}

// ------------------------------------------------------------------- users

func (f *fakeQuerier) CreateUser(_ context.Context, arg sqlcgen.CreateUserParams) (sqlcgen.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	for _, u := range f.users {
		if strings.EqualFold(u.Email, arg.Email) || strings.EqualFold(u.Username, arg.Username) {
			return sqlcgen.User{}, errUniqueViolation
		}
	}

	now := time.Now()
	user := sqlcgen.User{
		ID:           uuid.New(),
		Email:        arg.Email,
		Username:     arg.Username,
		PasswordHash: arg.PasswordHash,
		Role:         "user",
		Verified:     false,
		Level:        1,
		StarPoints:   0,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	f.users[user.ID] = user
	return user, nil
}

func (f *fakeQuerier) GetUserByID(_ context.Context, id uuid.UUID) (sqlcgen.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if u, ok := f.users[id]; ok {
		return u, nil
	}
	return sqlcgen.User{}, pgx.ErrNoRows
}

func (f *fakeQuerier) GetUserByEmail(_ context.Context, lower string) (sqlcgen.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, u := range f.users {
		if strings.EqualFold(u.Email, lower) {
			return u, nil
		}
	}
	return sqlcgen.User{}, pgx.ErrNoRows
}

func (f *fakeQuerier) GetUserByUsername(_ context.Context, lower string) (sqlcgen.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, u := range f.users {
		if strings.EqualFold(u.Username, lower) {
			return u, nil
		}
	}
	return sqlcgen.User{}, pgx.ErrNoRows
}

func (f *fakeQuerier) MarkUserVerified(_ context.Context, id uuid.UUID) (sqlcgen.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	u, ok := f.users[id]
	if !ok {
		return sqlcgen.User{}, pgx.ErrNoRows
	}
	u.Verified = true
	u.UpdatedAt = time.Now()
	f.users[id] = u
	return u, nil
}

func (f *fakeQuerier) UpdateUserPassword(_ context.Context, arg sqlcgen.UpdateUserPasswordParams) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	u, ok := f.users[arg.ID]
	if !ok {
		return pgx.ErrNoRows
	}
	u.PasswordHash = arg.PasswordHash
	f.users[arg.ID] = u
	return nil
}

func (f *fakeQuerier) CountUsersByEmailOrUsername(_ context.Context, arg sqlcgen.CountUsersByEmailOrUsernameParams) (sqlcgen.CountUsersByEmailOrUsernameRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var row sqlcgen.CountUsersByEmailOrUsernameRow
	for _, u := range f.users {
		if strings.EqualFold(u.Email, arg.Lower) {
			row.EmailCount++
		}
		if strings.EqualFold(u.Username, arg.Lower_2) {
			row.UsernameCount++
		}
	}
	return row, nil
}

// ------------------------------------------------- email verification tokens

func (f *fakeQuerier) CreateEmailVerificationToken(_ context.Context, arg sqlcgen.CreateEmailVerificationTokenParams) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.verificationToken[key(arg.TokenHash)] = sqlcgen.EmailVerificationToken{
		TokenHash: arg.TokenHash,
		UserID:    arg.UserID,
		ExpiresAt: arg.ExpiresAt,
		CreatedAt: time.Now(),
	}
	return nil
}

// Mirrors the SQL: consumed or expired tokens are simply not found.
func (f *fakeQuerier) GetLiveEmailVerificationToken(_ context.Context, hash []byte) (sqlcgen.EmailVerificationToken, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.verificationToken[key(hash)]
	if !ok || t.ConsumedAt != nil || !t.ExpiresAt.After(time.Now()) {
		return sqlcgen.EmailVerificationToken{}, pgx.ErrNoRows
	}
	return t, nil
}

func (f *fakeQuerier) ConsumeEmailVerificationToken(_ context.Context, hash []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if t, ok := f.verificationToken[key(hash)]; ok && t.ConsumedAt == nil {
		now := time.Now()
		t.ConsumedAt = &now
		f.verificationToken[key(hash)] = t
	}
	return nil
}

func (f *fakeQuerier) ConsumeAllEmailVerificationTokensForUser(_ context.Context, userID uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	now := time.Now()
	for k, t := range f.verificationToken {
		if t.UserID == userID && t.ConsumedAt == nil {
			t.ConsumedAt = &now
			f.verificationToken[k] = t
		}
	}
	return nil
}

// ------------------------------------------------------ password reset tokens

func (f *fakeQuerier) CreatePasswordResetToken(_ context.Context, arg sqlcgen.CreatePasswordResetTokenParams) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.resetToken[key(arg.TokenHash)] = sqlcgen.PasswordResetToken{
		TokenHash: arg.TokenHash,
		UserID:    arg.UserID,
		ExpiresAt: arg.ExpiresAt,
		CreatedAt: time.Now(),
	}
	return nil
}

func (f *fakeQuerier) GetLivePasswordResetToken(_ context.Context, hash []byte) (sqlcgen.PasswordResetToken, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.resetToken[key(hash)]
	if !ok || t.ConsumedAt != nil || !t.ExpiresAt.After(time.Now()) {
		return sqlcgen.PasswordResetToken{}, pgx.ErrNoRows
	}
	return t, nil
}

func (f *fakeQuerier) ConsumePasswordResetToken(_ context.Context, hash []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if t, ok := f.resetToken[key(hash)]; ok && t.ConsumedAt == nil {
		now := time.Now()
		t.ConsumedAt = &now
		f.resetToken[key(hash)] = t
	}
	return nil
}

func (f *fakeQuerier) ConsumeAllPasswordResetTokensForUser(_ context.Context, userID uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	now := time.Now()
	for k, t := range f.resetToken {
		if t.UserID == userID && t.ConsumedAt == nil {
			t.ConsumedAt = &now
			f.resetToken[k] = t
		}
	}
	return nil
}

// ---------------------------------------------------------- refresh tokens

func (f *fakeQuerier) CreateRefreshToken(_ context.Context, arg sqlcgen.CreateRefreshTokenParams) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.refreshToken[key(arg.TokenHash)] = sqlcgen.RefreshToken{
		TokenHash: arg.TokenHash,
		UserID:    arg.UserID,
		ExpiresAt: arg.ExpiresAt,
		UserAgent: arg.UserAgent,
		CreatedAt: time.Now(),
	}
	return nil
}

func (f *fakeQuerier) GetLiveRefreshToken(_ context.Context, hash []byte) (sqlcgen.RefreshToken, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.refreshToken[key(hash)]
	if !ok || t.RevokedAt != nil || !t.ExpiresAt.After(time.Now()) {
		return sqlcgen.RefreshToken{}, pgx.ErrNoRows
	}
	return t, nil
}

func (f *fakeQuerier) RevokeRefreshToken(_ context.Context, hash []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if t, ok := f.refreshToken[key(hash)]; ok && t.RevokedAt == nil {
		now := time.Now()
		t.RevokedAt = &now
		f.refreshToken[key(hash)] = t
	}
	return nil
}

func (f *fakeQuerier) RevokeAllRefreshTokensForUser(_ context.Context, userID uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	now := time.Now()
	for k, t := range f.refreshToken {
		if t.UserID == userID && t.RevokedAt == nil {
			t.RevokedAt = &now
			f.refreshToken[k] = t
		}
	}
	return nil
}

func (f *fakeQuerier) DeleteExpiredTokens(context.Context) error { return nil }

// liveRefreshTokenCount reports how many refresh tokens are still usable, so a
// test can assert that logout and password reset actually revoked them.
func (f *fakeQuerier) liveRefreshTokenCount(userID uuid.UUID) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, t := range f.refreshToken {
		if t.UserID == userID && t.RevokedAt == nil && t.ExpiresAt.After(time.Now()) {
			n++
		}
	}
	return n
}

var _ sqlcgen.Querier = (*fakeQuerier)(nil)

// ------------------------------------------------------------------ listings

func (f *fakeQuerier) CreateListing(_ context.Context, arg sqlcgen.CreateListingParams) (sqlcgen.Listing, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	l := sqlcgen.Listing{
		ID: uuid.New(), OwnerID: arg.OwnerID, Title: arg.Title, Body: arg.Body,
		Conditions: arg.Conditions, PriceCents: arg.PriceCents, DepositCents: arg.DepositCents,
		StartDate: arg.StartDate, EndDate: arg.EndDate, LeaseMonths: arg.LeaseMonths,
		TermTag: arg.TermTag, UnitType: arg.UnitType, BedroomsTotal: arg.BedroomsTotal,
		BedroomOf: arg.BedroomOf, Bathrooms: arg.Bathrooms, BathType: arg.BathType,
		Furnished: arg.Furnished, Utilities: arg.Utilities, Parking: arg.Parking,
		Pets: arg.Pets, Laundry: arg.Laundry, AddressLine: arg.AddressLine,
		Neighbourhood: arg.Neighbourhood, Lat: arg.Lat, Lng: arg.Lng, DistanceM: arg.DistanceM,
		CommuteMinutes: arg.CommuteMinutes, CommuteMode: arg.CommuteMode,
		MinutesToTransit: arg.MinutesToTransit, MinutesToGrocery: arg.MinutesToGrocery,
		Status: arg.Status, Views: arg.Views, Replies: arg.Replies,
		CreatedAt: arg.CreatedAt, UpdatedAt: arg.CreatedAt,
	}
	if l.Conditions == nil {
		l.Conditions = []string{}
	}
	if l.Utilities == nil {
		l.Utilities = []string{}
	}
	f.listings = append(f.listings, l)
	return l, nil
}

func (f *fakeQuerier) DeleteAllListings(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.listings = nil
	return nil
}

// matches mirrors the WHERE clause of ListListings. Kept faithful on purpose:
// a fake that filtered more loosely than the database would let a handler test
// pass against behaviour PostgreSQL would reject.
func (f *fakeQuerier) matches(l sqlcgen.Listing, p sqlcgen.ListListingsParams) bool {
	if l.Status != "published" {
		return false
	}
	owner := f.users[l.OwnerID]

	if p.Search != nil {
		haystack := strings.ToLower(strings.Join(
			[]string{l.Title, l.Body, l.Neighbourhood, l.AddressLine}, " "))
		for _, word := range strings.Fields(strings.ToLower(*p.Search)) {
			if !strings.Contains(haystack, strings.Trim(word, `"`)) {
				return false
			}
		}
	}
	if p.StartAfter != nil && l.StartDate.After(*p.StartAfter) {
		return false
	}
	if p.EndBefore != nil && l.EndDate.Before(*p.EndBefore) {
		return false
	}
	if p.PriceMin != nil && l.PriceCents < *p.PriceMin {
		return false
	}
	if p.PriceMax != nil && l.PriceCents > *p.PriceMax {
		return false
	}
	if p.DistanceMax != nil {
		// An unknown distance cannot satisfy "within N metres".
		if l.DistanceM == nil || *l.DistanceM > *p.DistanceMax {
			return false
		}
	}
	if p.BedroomsMin != nil && l.BedroomsTotal < *p.BedroomsMin {
		return false
	}
	if p.Furnished != nil && l.Furnished != *p.Furnished {
		return false
	}
	if p.Parking != nil && l.Parking != *p.Parking {
		return false
	}
	if p.Pets != nil && l.Pets != *p.Pets {
		return false
	}
	if p.Laundry != nil && l.Laundry != *p.Laundry {
		return false
	}
	for _, want := range p.Utilities {
		if !slices.Contains(l.Utilities, want) {
			return false
		}
	}
	if p.VerifiedOnly != nil && *p.VerifiedOnly && !owner.Verified {
		return false
	}
	return true
}

func (f *fakeQuerier) filtered(p sqlcgen.ListListingsParams) []sqlcgen.Listing {
	out := []sqlcgen.Listing{}
	for _, l := range f.listings {
		if f.matches(l, p) {
			out = append(out, l)
		}
	}

	switch p.Sort {
	case "priceAsc":
		sort.SliceStable(out, func(i, j int) bool { return out[i].PriceCents < out[j].PriceCents })
	case "priceDesc":
		sort.SliceStable(out, func(i, j int) bool { return out[i].PriceCents > out[j].PriceCents })
	case "distance":
		sort.SliceStable(out, func(i, j int) bool {
			if out[i].DistanceM == nil {
				return false
			}
			if out[j].DistanceM == nil {
				return true
			}
			return *out[i].DistanceM < *out[j].DistanceM
		})
	default: // new, match
		sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	}
	return out
}

func (f *fakeQuerier) CountListings(_ context.Context, arg sqlcgen.CountListingsParams) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return int64(len(f.filtered(sqlcgen.ListListingsParams{
		Search: arg.Search, StartAfter: arg.StartAfter, EndBefore: arg.EndBefore,
		PriceMin: arg.PriceMin, PriceMax: arg.PriceMax, DistanceMax: arg.DistanceMax,
		BedroomsMin: arg.BedroomsMin, Furnished: arg.Furnished, Parking: arg.Parking,
		Pets: arg.Pets, Laundry: arg.Laundry, Utilities: arg.Utilities,
		VerifiedOnly: arg.VerifiedOnly,
	}))), nil
}

func (f *fakeQuerier) ListListings(_ context.Context, arg sqlcgen.ListListingsParams) ([]sqlcgen.ListListingsRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	all := f.filtered(arg)
	start := int(arg.Offset)
	if start > len(all) {
		start = len(all)
	}
	end := start + int(arg.Limit)
	if end > len(all) {
		end = len(all)
	}

	rows := []sqlcgen.ListListingsRow{}
	for _, l := range all[start:end] {
		owner := f.users[l.OwnerID]
		rows = append(rows, sqlcgen.ListListingsRow{
			Listing:        l,
			OwnerUsername:  owner.Username,
			OwnerAvatarUrl: owner.AvatarUrl,
			OwnerVerified:  owner.Verified,
		})
	}
	return rows, nil
}

func (f *fakeQuerier) GetPublishedListing(_ context.Context, id uuid.UUID) (sqlcgen.GetPublishedListingRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, l := range f.listings {
		if l.ID == id && l.Status == "published" {
			owner := f.users[l.OwnerID]
			return sqlcgen.GetPublishedListingRow{
				Listing:        l,
				OwnerUsername:  owner.Username,
				OwnerAvatarUrl: owner.AvatarUrl,
				OwnerVerified:  owner.Verified,
			}, nil
		}
	}
	return sqlcgen.GetPublishedListingRow{}, pgx.ErrNoRows
}

func (f *fakeQuerier) ListPhotosForListing(context.Context, uuid.UUID) ([]sqlcgen.ListingPhoto, error) {
	return []sqlcgen.ListingPhoto{}, nil
}

func (f *fakeQuerier) ListPhotosForListings(context.Context, []uuid.UUID) ([]sqlcgen.ListingPhoto, error) {
	return []sqlcgen.ListingPhoto{}, nil
}

// -------------------------------------------------------------- write path

func (f *fakeQuerier) ListListingsByOwner(_ context.Context, ownerID uuid.UUID) ([]sqlcgen.ListListingsByOwnerRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	// Every status, not just published — that is the point of this query.
	mine := []sqlcgen.Listing{}
	for _, l := range f.listings {
		if l.OwnerID == ownerID {
			mine = append(mine, l)
		}
	}
	sort.SliceStable(mine, func(i, j int) bool { return mine[i].CreatedAt.After(mine[j].CreatedAt) })

	rows := []sqlcgen.ListListingsByOwnerRow{}
	for _, l := range mine {
		owner := f.users[l.OwnerID]
		rows = append(rows, sqlcgen.ListListingsByOwnerRow{
			Listing:        l,
			OwnerUsername:  owner.Username,
			OwnerAvatarUrl: owner.AvatarUrl,
			OwnerVerified:  owner.Verified,
		})
	}
	return rows, nil
}

func (f *fakeQuerier) GetListingForOwner(_ context.Context, id uuid.UUID) (sqlcgen.Listing, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, l := range f.listings {
		if l.ID == id {
			return l, nil
		}
	}
	return sqlcgen.Listing{}, pgx.ErrNoRows
}

func (f *fakeQuerier) UpdateListing(_ context.Context, arg sqlcgen.UpdateListingParams) (sqlcgen.Listing, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	for i, l := range f.listings {
		if l.ID != arg.ID {
			continue
		}
		// Mirrors the SQL's COALESCE: nil leaves the column alone.
		if arg.Title != nil {
			l.Title = *arg.Title
		}
		if arg.Body != nil {
			l.Body = *arg.Body
		}
		if arg.Conditions != nil {
			l.Conditions = arg.Conditions
		}
		if arg.PriceCents != nil {
			l.PriceCents = *arg.PriceCents
		}
		if arg.DepositCents != nil {
			l.DepositCents = arg.DepositCents
		}
		if arg.StartDate != nil {
			l.StartDate = *arg.StartDate
		}
		if arg.EndDate != nil {
			l.EndDate = *arg.EndDate
		}
		if arg.LeaseMonths != nil {
			l.LeaseMonths = *arg.LeaseMonths
		}
		if arg.TermTag != nil {
			l.TermTag = *arg.TermTag
		}
		if arg.UnitType != nil {
			l.UnitType = *arg.UnitType
		}
		if arg.BedroomsTotal != nil {
			l.BedroomsTotal = *arg.BedroomsTotal
		}
		if arg.BedroomOf != nil {
			l.BedroomOf = arg.BedroomOf
		}
		if arg.Bathrooms != nil {
			l.Bathrooms = *arg.Bathrooms
		}
		if arg.BathType != nil {
			l.BathType = *arg.BathType
		}
		if arg.Furnished != nil {
			l.Furnished = *arg.Furnished
		}
		if arg.Utilities != nil {
			l.Utilities = arg.Utilities
		}
		if arg.Parking != nil {
			l.Parking = *arg.Parking
		}
		if arg.Pets != nil {
			l.Pets = *arg.Pets
		}
		if arg.Laundry != nil {
			l.Laundry = *arg.Laundry
		}
		if arg.AddressLine != nil {
			l.AddressLine = *arg.AddressLine
		}
		if arg.Neighbourhood != nil {
			l.Neighbourhood = *arg.Neighbourhood
		}
		if arg.DistanceM != nil {
			l.DistanceM = arg.DistanceM
		}
		l.UpdatedAt = time.Now()
		f.listings[i] = l
		return l, nil
	}
	return sqlcgen.Listing{}, pgx.ErrNoRows
}

func (f *fakeQuerier) SetListingStatus(_ context.Context, arg sqlcgen.SetListingStatusParams) (sqlcgen.Listing, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	for i, l := range f.listings {
		if l.ID != arg.ID {
			continue
		}
		l.Status = arg.Status
		// Set once and never moved, so re-publishing does not make an old post
		// look new.
		if arg.Status == "published" && l.PublishedAt == nil {
			now := time.Now()
			l.PublishedAt = &now
		}
		f.listings[i] = l
		return l, nil
	}
	return sqlcgen.Listing{}, pgx.ErrNoRows
}
