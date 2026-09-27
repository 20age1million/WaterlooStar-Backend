package db

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/20age1million/WaterlooStar-Backend/internal/db/sqlcgen"
)

var (
	// ErrLastAdmin refuses a change that would leave the site with no admin.
	// With nobody left to hold the role, only the host could grant it again.
	ErrLastAdmin = errors.New("that would remove the last admin")

	// ErrRoleUnchanged means the account already holds the role. Nothing is
	// written, so the ledger does not fill with changes that changed nothing.
	ErrRoleUnchanged = errors.New("the account already has that role")
)

// RoleChange is the outcome of ChangeRole.
type RoleChange struct {
	User     sqlcgen.User
	Previous string
	Logged   sqlcgen.AdminAction
}

// ChangeRole sets an account's role and records it, in one transaction.
//
// Every live session of the account is revoked with it, so a demoted admin
// stops being one at the next refresh rather than whenever their refresh token
// happens to expire. The access token already issued still carries the old
// role for up to its fifteen-minute life; the Admin and Moderation spec accepts
// that window.
//
// cmd/admin calls this with a nil actor; the portal will pass the admin's id.
func ChangeRole(ctx context.Context, run TxRunner, actor *uuid.UUID, userID uuid.UUID, role, reason string) (RoleChange, error) {
	var out RoleChange

	logged, err := Audited(ctx, run, Action{
		ActorID:     actor,
		Action:      ActionSetRole,
		SubjectType: SubjectUser,
		SubjectID:   userID,
		Reason:      reason,
	}, func(q sqlcgen.Querier) (map[string]any, error) {
		before, err := q.GetUserByID(ctx, userID)
		if err != nil {
			return nil, err
		}
		if before.Role == role {
			return nil, ErrRoleUnchanged
		}

		after, err := q.SetUserRole(ctx, sqlcgen.SetUserRoleParams{ID: userID, Role: role})
		if err != nil {
			return nil, err
		}

		// Counted after the update, inside the transaction, so the check sees
		// the state that is about to be committed rather than the one before it.
		if before.Role == "admin" {
			admins, err := q.CountAdmins(ctx)
			if err != nil {
				return nil, err
			}
			if admins == 0 {
				return nil, ErrLastAdmin
			}
		}

		if err := q.RevokeAllRefreshTokensForUser(ctx, userID); err != nil {
			return nil, err
		}

		out.User = after
		out.Previous = before.Role
		return map[string]any{"from": before.Role, "to": role, "sessions_revoked": true}, nil
	})
	if err != nil {
		return RoleChange{}, err
	}

	out.Logged = logged
	return out, nil
}
