package httpapi

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/20age1million/WaterlooStar-Backend/internal/auth"
)

// pgUniqueViolation is PostgreSQL's SQLSTATE for a unique constraint breach.
const pgUniqueViolation = "23505"

// isUniqueViolation reports whether the error is a unique-index conflict.
//
// The handlers check for duplicates before inserting, but two simultaneous
// registrations can both pass that check; the index is the real guarantee and
// this turns its error into the same clean 409.
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation
}

// refreshTokenFrom returns the raw refresh cookie lifted onto the context by the
// authentication middleware.
func refreshTokenFrom(ctx context.Context) string {
	return auth.RefreshTokenFrom(ctx)
}

// userAgentFrom returns the request's User-Agent, recorded against a refresh
// token so sessions can later be listed meaningfully.
func userAgentFrom(ctx context.Context) string {
	return auth.UserAgentFrom(ctx)
}
