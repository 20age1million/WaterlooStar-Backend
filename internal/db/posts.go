package db

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/20age1million/WaterlooStar-Backend/internal/db/sqlcgen"
)

// Takedown and restore of posts. Takedown is the moderator's axis, separate
// from the owner's status: removing a post sets removed_*, restoring clears
// them, and status is never written here, so a restored post is back in
// whatever state its owner left it.

var (
	ErrAlreadyRemoved = errors.New("the post is already taken down")
	ErrNotRemoved     = errors.New("the post is not taken down")
)

// DefaultRestoreReason is recorded when an admin restores a post without
// saying why. Restoring needs no reason, but the ledger does.
const DefaultRestoreReason = "Restored by an admin"

// postState is what the ledger needs to know about a post either side of a
// change, whichever table it lives in.
type postState struct {
	status        string
	removedAt     *time.Time
	removedReason *string
}

// postOps binds the generic takedown logic to one table's queries.
type postOps struct {
	subject string
	get     func(q sqlcgen.Querier, ctx context.Context, id uuid.UUID) (postState, error)
	remove  func(q sqlcgen.Querier, ctx context.Context, id uuid.UUID, actor *uuid.UUID, reason string) error
	restore func(q sqlcgen.Querier, ctx context.Context, id uuid.UUID) error
}

var listingOps = postOps{
	subject: SubjectListing,
	get: func(q sqlcgen.Querier, ctx context.Context, id uuid.UUID) (postState, error) {
		l, err := q.GetListingForOwner(ctx, id)
		return postState{l.Status, l.RemovedAt, l.RemovedReason}, err
	},
	remove: func(q sqlcgen.Querier, ctx context.Context, id uuid.UUID, actor *uuid.UUID, reason string) error {
		_, err := q.RemoveListing(ctx, sqlcgen.RemoveListingParams{ID: id, ActorID: actor, Reason: &reason})
		return err
	},
	restore: func(q sqlcgen.Querier, ctx context.Context, id uuid.UUID) error {
		_, err := q.RestoreListing(ctx, id)
		return err
	},
}

var requestOps = postOps{
	subject: SubjectRequest,
	get: func(q sqlcgen.Querier, ctx context.Context, id uuid.UUID) (postState, error) {
		r, err := q.GetRequestForOwner(ctx, id)
		return postState{r.Status, r.RemovedAt, r.RemovedReason}, err
	},
	remove: func(q sqlcgen.Querier, ctx context.Context, id uuid.UUID, actor *uuid.UUID, reason string) error {
		_, err := q.RemoveRequest(ctx, sqlcgen.RemoveRequestParams{ID: id, ActorID: actor, Reason: &reason})
		return err
	},
	restore: func(q sqlcgen.Querier, ctx context.Context, id uuid.UUID) error {
		_, err := q.RestoreRequest(ctx, id)
		return err
	},
}

func opsFor(subject string) (postOps, error) {
	switch subject {
	case SubjectListing:
		return listingOps, nil
	case SubjectRequest:
		return requestOps, nil
	default:
		return postOps{}, fmt.Errorf("no takedown for subject %q", subject)
	}
}

// RemovePost takes a listing or request down. A missing post returns
// pgx.ErrNoRows; one already down returns ErrAlreadyRemoved and writes nothing,
// so the first takedown's time and reason stand.
func RemovePost(ctx context.Context, run TxRunner, actor uuid.UUID, subject string, id uuid.UUID, reason string) error {
	ops, err := opsFor(subject)
	if err != nil {
		return err
	}
	_, err = Audited(ctx, run, Action{
		ActorID: &actor, Action: ActionRemovePost, SubjectType: subject, SubjectID: id, Reason: reason,
	}, func(q sqlcgen.Querier) (map[string]any, error) {
		before, err := ops.get(q, ctx, id)
		if err != nil {
			return nil, err
		}
		if err := ops.remove(q, ctx, id, &actor, trimmedReason(reason)); errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrAlreadyRemoved
		} else if err != nil {
			return nil, err
		}
		// The owner's status at the time, so the ledger says what was hidden.
		return map[string]any{"from": "visible", "to": "removed", "status": before.status}, nil
	})
	return err
}

// RestorePost lifts a takedown. reason may be empty.
func RestorePost(ctx context.Context, run TxRunner, actor uuid.UUID, subject string, id uuid.UUID, reason string) error {
	ops, err := opsFor(subject)
	if err != nil {
		return err
	}
	if trimmedReason(reason) == "" {
		reason = DefaultRestoreReason
	}
	_, err = Audited(ctx, run, Action{
		ActorID: &actor, Action: ActionRestorePost, SubjectType: subject, SubjectID: id, Reason: reason,
	}, func(q sqlcgen.Querier) (map[string]any, error) {
		before, err := ops.get(q, ctx, id)
		if err != nil {
			return nil, err
		}
		if err := ops.restore(q, ctx, id); errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotRemoved
		} else if err != nil {
			return nil, err
		}
		detail := map[string]any{"from": "removed", "to": "visible", "status": before.status}
		if before.removedAt != nil {
			detail["removed_at"] = before.removedAt.UTC().Format(time.RFC3339)
		}
		if before.removedReason != nil {
			detail["removed_reason"] = *before.removedReason
		}
		return detail, nil
	})
	return err
}
