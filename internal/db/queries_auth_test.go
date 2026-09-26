package db_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/20age1million/WaterlooStar-Backend/internal/db/dbtest"
	"github.com/20age1million/WaterlooStar-Backend/internal/db/sqlcgen"
)

// The account and token queries, against a real PostgreSQL.
//
// These check what SQL decides rather than what Go decides: the case-folded
// lookups, the check constraints, and above all the "live token" filters, which
// exclude spent and expired rows in SQL precisely so that no caller can forget
// to.

func TestUserLookupsFoldCase(t *testing.T) {
	t.Parallel()
	q, _ := dbtest.Queries(t)
	ctx := context.Background()

	created := dbtest.User(t, q, "Meil@uwaterloo.ca", true)

	// Someone typing their address in a hurry must still find their account.
	byEmail, err := q.GetUserByEmail(ctx, "MEIL@UWATERLOO.CA")
	if err != nil {
		t.Fatalf("GetUserByEmail: %v", err)
	}
	if byEmail.ID != created.ID {
		t.Error("a differently cased address found a different account")
	}

	byName, err := q.GetUserByUsername(ctx, "MEIL")
	if err != nil {
		t.Fatalf("GetUserByUsername: %v", err)
	}
	if byName.ID != created.ID {
		t.Error("a differently cased username found a different account")
	}

	byID, err := q.GetUserByID(ctx, created.ID)
	if err != nil || byID.Email != created.Email {
		t.Fatalf("GetUserByID: %v", err)
	}
}

func TestUserLookupReportsMissingAsNoRows(t *testing.T) {
	t.Parallel()
	q, _ := dbtest.Queries(t)

	// Callers branch on pgx.ErrNoRows to tell "wrong password" from "no such
	// account" without leaking which it was.
	_, err := q.GetUserByEmail(context.Background(), "nobody@uwaterloo.ca")
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("err = %v, want pgx.ErrNoRows", err)
	}
}

func TestCountUsersByEmailOrUsernameSeparatesTheTwo(t *testing.T) {
	t.Parallel()
	q, _ := dbtest.Queries(t)

	dbtest.User(t, q, "taken@uwaterloo.ca", true)

	// Registration tells the person which of the two is taken, so the counts
	// must not be pooled.
	row, err := q.CountUsersByEmailOrUsername(context.Background(),
		sqlcgen.CountUsersByEmailOrUsernameParams{Lower: "taken@uwaterloo.ca", Lower_2: "free"})
	if err != nil {
		t.Fatalf("CountUsersByEmailOrUsername: %v", err)
	}
	if row.EmailCount != 1 || row.UsernameCount != 0 {
		t.Errorf("email=%d username=%d, want 1 and 0", row.EmailCount, row.UsernameCount)
	}
}

func TestOnlyUwaterlooAddressesAreAccepted(t *testing.T) {
	t.Parallel()
	q, _ := dbtest.Queries(t)

	// The whole trust proposition, enforced by a check constraint rather than
	// by whichever handler happens to run.
	_, err := q.CreateUser(context.Background(), sqlcgen.CreateUserParams{
		Email: "someone@gmail.com", Username: "outsider", PasswordHash: "x",
	})
	if err == nil {
		t.Fatal("the database accepted a non-uwaterloo address")
	}
}

func TestVerifyingAndChangingAPassword(t *testing.T) {
	t.Parallel()
	q, _ := dbtest.Queries(t)
	ctx := context.Background()

	user := dbtest.User(t, q, "changer@uwaterloo.ca", false)
	if user.Verified {
		t.Fatal("a new account must start unverified")
	}

	verified, err := q.MarkUserVerified(ctx, user.ID)
	if err != nil || !verified.Verified {
		t.Fatalf("MarkUserVerified: %v", err)
	}

	if err := q.UpdateUserPassword(ctx,
		sqlcgen.UpdateUserPasswordParams{ID: user.ID, PasswordHash: "second-hash"}); err != nil {
		t.Fatalf("UpdateUserPassword: %v", err)
	}
	after, err := q.GetUserByID(ctx, user.ID)
	if err != nil || after.PasswordHash != "second-hash" {
		t.Fatalf("password not changed: %v", err)
	}
}

// ------------------------------------------------------------------- tokens

func TestEmailVerificationTokenLifecycle(t *testing.T) {
	t.Parallel()
	q, _ := dbtest.Queries(t)
	ctx := context.Background()

	user := dbtest.User(t, q, "verify@uwaterloo.ca", false)
	hash := []byte("verification-token-hash")

	if err := q.CreateEmailVerificationToken(ctx, sqlcgen.CreateEmailVerificationTokenParams{
		TokenHash: hash, UserID: user.ID, ExpiresAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("CreateEmailVerificationToken: %v", err)
	}

	if _, err := q.GetLiveEmailVerificationToken(ctx, hash); err != nil {
		t.Fatalf("a live token must be found: %v", err)
	}

	if err := q.ConsumeEmailVerificationToken(ctx, hash); err != nil {
		t.Fatalf("ConsumeEmailVerificationToken: %v", err)
	}
	// Spent is not merely marked: the lookup must stop returning it, or a link
	// would work twice.
	if _, err := q.GetLiveEmailVerificationToken(ctx, hash); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("a spent token was still live: %v", err)
	}
}

func TestExpiredTokensAreNotLive(t *testing.T) {
	t.Parallel()
	q, _ := dbtest.Queries(t)
	ctx := context.Background()

	user := dbtest.User(t, q, "expired@uwaterloo.ca", false)
	hash := []byte("already-expired")

	if err := q.CreateEmailVerificationToken(ctx, sqlcgen.CreateEmailVerificationTokenParams{
		TokenHash: hash, UserID: user.ID, ExpiresAt: time.Now().Add(-time.Minute),
	}); err != nil {
		t.Fatalf("CreateEmailVerificationToken: %v", err)
	}

	// The expiry check lives in SQL so no caller can skip it.
	if _, err := q.GetLiveEmailVerificationToken(ctx, hash); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("an expired token was live: %v", err)
	}
}

func TestPasswordResetTokensAreConsumedTogether(t *testing.T) {
	t.Parallel()
	q, _ := dbtest.Queries(t)
	ctx := context.Background()

	user := dbtest.User(t, q, "resetter@uwaterloo.ca", true)
	for _, h := range [][]byte{[]byte("reset-a"), []byte("reset-b")} {
		if err := q.CreatePasswordResetToken(ctx, sqlcgen.CreatePasswordResetTokenParams{
			TokenHash: h, UserID: user.ID, ExpiresAt: time.Now().Add(time.Hour),
		}); err != nil {
			t.Fatalf("CreatePasswordResetToken: %v", err)
		}
	}

	if _, err := q.GetLivePasswordResetToken(ctx, []byte("reset-a")); err != nil {
		t.Fatalf("GetLivePasswordResetToken: %v", err)
	}
	if err := q.ConsumePasswordResetToken(ctx, []byte("reset-a")); err != nil {
		t.Fatalf("ConsumePasswordResetToken: %v", err)
	}

	// Using one link must kill the others: someone who asked three times has
	// three live links in their inbox, and only the one they clicked should work.
	if err := q.ConsumeAllPasswordResetTokensForUser(ctx, user.ID); err != nil {
		t.Fatalf("ConsumeAllPasswordResetTokensForUser: %v", err)
	}
	if _, err := q.GetLivePasswordResetToken(ctx, []byte("reset-b")); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("a sibling reset token stayed live: %v", err)
	}

	if err := q.ConsumeAllEmailVerificationTokensForUser(ctx, user.ID); err != nil {
		t.Fatalf("ConsumeAllEmailVerificationTokensForUser: %v", err)
	}
}

func TestRefreshTokenRotationAndRevocation(t *testing.T) {
	t.Parallel()
	q, _ := dbtest.Queries(t)
	ctx := context.Background()

	user := dbtest.User(t, q, "sessions@uwaterloo.ca", true)
	agent := "Mozilla/5.0 (test)"

	for _, h := range [][]byte{[]byte("refresh-one"), []byte("refresh-two")} {
		if err := q.CreateRefreshToken(ctx, sqlcgen.CreateRefreshTokenParams{
			TokenHash: h, UserID: user.ID,
			ExpiresAt: time.Now().Add(30 * 24 * time.Hour), UserAgent: &agent,
		}); err != nil {
			t.Fatalf("CreateRefreshToken: %v", err)
		}
	}

	live, err := q.GetLiveRefreshToken(ctx, []byte("refresh-one"))
	if err != nil {
		t.Fatalf("GetLiveRefreshToken: %v", err)
	}
	if live.UserID != user.ID {
		t.Error("refresh token belongs to the wrong account")
	}

	// Rotation: the presented token dies as its replacement is born.
	if err := q.RevokeRefreshToken(ctx, []byte("refresh-one")); err != nil {
		t.Fatalf("RevokeRefreshToken: %v", err)
	}
	if _, err := q.GetLiveRefreshToken(ctx, []byte("refresh-one")); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("a revoked refresh token was still live: %v", err)
	}

	// Signing out everywhere.
	if err := q.RevokeAllRefreshTokensForUser(ctx, user.ID); err != nil {
		t.Fatalf("RevokeAllRefreshTokensForUser: %v", err)
	}
	if _, err := q.GetLiveRefreshToken(ctx, []byte("refresh-two")); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("a session survived revoking them all: %v", err)
	}
}

func TestDeleteExpiredTokensLeavesLiveOnesAlone(t *testing.T) {
	t.Parallel()
	q, _ := dbtest.Queries(t)
	ctx := context.Background()

	user := dbtest.User(t, q, "sweeper@uwaterloo.ca", true)
	agent := "sweeper"
	dead := []byte("dead-refresh")
	alive := []byte("live-refresh")

	if err := q.CreateRefreshToken(ctx, sqlcgen.CreateRefreshTokenParams{
		TokenHash: dead, UserID: user.ID,
		ExpiresAt: time.Now().Add(-time.Hour), UserAgent: &agent,
	}); err != nil {
		t.Fatalf("CreateRefreshToken: %v", err)
	}
	if err := q.CreateRefreshToken(ctx, sqlcgen.CreateRefreshTokenParams{
		TokenHash: alive, UserID: user.ID,
		ExpiresAt: time.Now().Add(time.Hour), UserAgent: &agent,
	}); err != nil {
		t.Fatalf("CreateRefreshToken: %v", err)
	}

	if err := q.DeleteExpiredTokens(ctx); err != nil {
		t.Fatalf("DeleteExpiredTokens: %v", err)
	}
	if _, err := q.GetLiveRefreshToken(ctx, alive); err != nil {
		t.Fatalf("the sweep took a live session with it: %v", err)
	}
}

func TestPingAnswers(t *testing.T) {
	t.Parallel()
	q, _ := dbtest.Queries(t)

	// What /healthz asks the database.
	if _, err := q.Ping(context.Background()); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}
