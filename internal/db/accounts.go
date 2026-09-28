package db

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/20age1million/WaterlooStar-Backend/internal/db/sqlcgen"
)

// Account actions an admin takes: suspend, reinstate, verify by hand. Each one
// runs through Audited, so the change and its ledger row commit together, and
// each refuses a change that would change nothing rather than logging it.

var (
	// ErrSelfAction refuses an admin acting on their own account. Suspending
	// yourself locks you out; demoting yourself may leave nobody to undo it.
	ErrSelfAction = errors.New("an admin cannot do that to their own account")

	ErrAlreadySuspended = errors.New("the account is already suspended")
	ErrNotSuspended     = errors.New("the account is not suspended")
	ErrAlreadyVerified  = errors.New("the account is already verified")
)

// Suspend suspends an account and ends every session it has.
//
// The access token already issued keeps working for at most its fifteen-minute
// life; refresh is refused from now on, so the session dies with it. The
// account's posts leave public view through the read queries, which filter on
// suspended_at — nothing about the posts themselves is touched.
func Suspend(ctx context.Context, run TxRunner, actor uuid.UUID, userID uuid.UUID, reason string) (sqlcgen.User, error) {
	if actor == userID {
		return sqlcgen.User{}, ErrSelfAction
	}

	var out sqlcgen.User
	_, err := Audited(ctx, run, Action{
		ActorID: &actor, Action: ActionSuspend, SubjectType: SubjectUser, SubjectID: userID, Reason: reason,
	}, func(q sqlcgen.Querier) (map[string]any, error) {
		if _, err := q.GetUserByID(ctx, userID); err != nil {
			return nil, err
		}

		trimmed := trimmedReason(reason)
		after, err := q.SuspendUser(ctx, sqlcgen.SuspendUserParams{ID: userID, ActorID: &actor, Reason: &trimmed})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrAlreadySuspended
		}
		if err != nil {
			return nil, err
		}

		if err := q.RevokeAllRefreshTokensForUser(ctx, userID); err != nil {
			return nil, err
		}

		out = after
		return map[string]any{"from": "active", "to": "suspended", "sessions_revoked": true}, nil
	})
	return out, err
}

// Reinstate lifts a suspension. It restores exactly what suspension hid,
// because suspension changed nothing else.
func Reinstate(ctx context.Context, run TxRunner, actor uuid.UUID, userID uuid.UUID, reason string) (sqlcgen.User, error) {
	var out sqlcgen.User
	_, err := Audited(ctx, run, Action{
		ActorID: &actor, Action: ActionReinstate, SubjectType: SubjectUser, SubjectID: userID, Reason: reason,
	}, func(q sqlcgen.Querier) (map[string]any, error) {
		before, err := q.GetUserByID(ctx, userID)
		if err != nil {
			return nil, err
		}

		after, err := q.ReinstateUser(ctx, userID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotSuspended
		}
		if err != nil {
			return nil, err
		}

		out = after
		// The suspension being lifted, so the ledger keeps what the row forgets.
		detail := map[string]any{"from": "suspended", "to": "active"}
		if before.SuspendedAt != nil {
			detail["suspended_at"] = before.SuspendedAt.UTC().Format(time.RFC3339)
		}
		if before.SuspendReason != nil {
			detail["suspend_reason"] = *before.SuspendReason
		}
		return detail, nil
	})
	return out, err
}

// VerifyByHand marks an account's address as confirmed without the email link.
// It exists because the links only reach the API log until email delivery is
// built. The account's next refresh carries the new claim.
func VerifyByHand(ctx context.Context, run TxRunner, actor uuid.UUID, userID uuid.UUID, reason string) (sqlcgen.User, error) {
	var out sqlcgen.User
	_, err := Audited(ctx, run, Action{
		ActorID: &actor, Action: ActionVerify, SubjectType: SubjectUser, SubjectID: userID, Reason: reason,
	}, func(q sqlcgen.Querier) (map[string]any, error) {
		if _, err := q.GetUserByID(ctx, userID); err != nil {
			return nil, err
		}

		after, err := q.VerifyUserAsAdmin(ctx, userID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrAlreadyVerified
		}
		if err != nil {
			return nil, err
		}

		out = after
		return map[string]any{"from": false, "to": true}, nil
	})
	return out, err
}
