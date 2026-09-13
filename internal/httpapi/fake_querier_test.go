package httpapi_test

import (
	"context"
	"encoding/hex"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/20age1million/waterloostar-api/internal/db/sqlcgen"
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
