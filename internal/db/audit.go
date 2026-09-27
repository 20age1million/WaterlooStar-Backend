package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/20age1million/WaterlooStar-Backend/internal/db/sqlcgen"
)

// TxRunner runs fn inside one database transaction: fn's error rolls
// everything back, and a nil return commits.
//
// It takes the Querier interface rather than a pool so the HTTP layer can be
// handed Direct over an in-memory fake in its tests.
type TxRunner func(ctx context.Context, fn func(q sqlcgen.Querier) error) error

// PoolTx is the real TxRunner.
func PoolTx(pool *pgxpool.Pool) TxRunner {
	return func(ctx context.Context, fn func(q sqlcgen.Querier) error) error {
		return pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
			return fn(sqlcgen.New(tx))
		})
	}
}

// Direct runs fn against q with no transaction at all. It exists for the
// handler tests' in-memory fake, which has nothing to roll back; the query
// tests exercise PoolTx against PostgreSQL.
func Direct(q sqlcgen.Querier) TxRunner {
	return func(_ context.Context, fn func(q sqlcgen.Querier) error) error {
		return fn(q)
	}
}

// The ledger's vocabulary. These must match the CHECK constraints in migration
// 8, which is what rejects a typo.
const (
	ActionSetRole     = "set_role"
	ActionSuspend     = "suspend"
	ActionReinstate   = "reinstate"
	ActionVerify      = "verify"
	ActionRemovePost  = "remove_post"
	ActionRestorePost = "restore_post"

	SubjectUser    = "user"
	SubjectListing = "listing"
	SubjectRequest = "request"
)

// ErrReasonRequired is returned before anything is written when an action has
// no reason. The database would refuse it too; this lets a caller say so in
// words.
var ErrReasonRequired = errors.New("a reason is required")

// Action is one entry for the ledger. ActorID is nil when the change is made
// from the host with cmd/admin.
type Action struct {
	ActorID     *uuid.UUID
	Action      string
	SubjectType string
	SubjectID   uuid.UUID
	Reason      string
}

// Audited performs change and records it in admin_actions, in one transaction.
//
// This is the only way an admin change should be made. An action that is not
// logged did not happen, and a log row for a change that failed is a lie: if
// either the change or the insert fails, both are rolled back. change returns
// the before and after of what it did, which becomes the row's detail.
func Audited(
	ctx context.Context,
	run TxRunner,
	a Action,
	change func(q sqlcgen.Querier) (detail map[string]any, err error),
) (sqlcgen.AdminAction, error) {
	reason := strings.TrimSpace(a.Reason)
	if reason == "" {
		return sqlcgen.AdminAction{}, ErrReasonRequired
	}

	var logged sqlcgen.AdminAction
	err := run(ctx, func(q sqlcgen.Querier) error {
		detail, err := change(q)
		if err != nil {
			return err
		}
		if detail == nil {
			detail = map[string]any{}
		}
		encoded, err := json.Marshal(detail)
		if err != nil {
			return fmt.Errorf("encode ledger detail: %w", err)
		}

		logged, err = q.InsertAdminAction(ctx, sqlcgen.InsertAdminActionParams{
			ActorID:     a.ActorID,
			Action:      a.Action,
			SubjectType: a.SubjectType,
			SubjectID:   a.SubjectID,
			Reason:      reason,
			Detail:      encoded,
		})
		if err != nil {
			return fmt.Errorf("record admin action: %w", err)
		}
		return nil
	})
	if err != nil {
		return sqlcgen.AdminAction{}, err
	}
	return logged, nil
}
