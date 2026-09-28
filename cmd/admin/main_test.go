package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/20age1million/WaterlooStar-Backend/internal/db/dbtest"
	"github.com/20age1million/WaterlooStar-Backend/internal/db/sqlcgen"
)

func TestParseTargetAcceptsTheFlagOnEitherSide(t *testing.T) {
	for _, args := range [][]string{
		{"meil@uwaterloo.ca", "-reason", "Operator"},
		{"-reason", "Operator", "meil@uwaterloo.ca"},
	} {
		email, reason, err := parseTarget(args)
		if err != nil || email != "meil@uwaterloo.ca" || reason != "Operator" {
			t.Errorf("parseTarget(%q) = %q, %q, %v", args, email, reason, err)
		}
	}

	if _, reason, _ := parseTarget([]string{"meil@uwaterloo.ca"}); reason != defaultReason {
		t.Errorf("default reason = %q", reason)
	}
	if _, _, err := parseTarget(nil); err == nil {
		t.Error("a missing address was accepted")
	}
	if _, _, err := parseTarget([]string{"a@uwaterloo.ca", "b@uwaterloo.ca"}); err == nil {
		t.Error("two addresses were accepted")
	}
}

func TestPromoteDemoteAndList(t *testing.T) {
	q, pool := dbtest.Queries(t)
	ctx := context.Background()
	dbtest.User(t, q, "meil@uwaterloo.ca", true)

	cmd := func(args ...string) (string, error) {
		var out bytes.Buffer
		err := run(ctx, pool, &out, args)
		return out.String(), err
	}

	if out, _ := cmd("list"); !strings.Contains(out, "No admins yet") {
		t.Errorf("list before any admin = %q", out)
	}

	out, err := cmd("promote", "MEIL@uwaterloo.ca", "-reason", "Site operator")
	if err != nil || !strings.Contains(out, "user → admin") {
		t.Fatalf("promote = %q, %v", out, err)
	}

	// Promoting an admin says so rather than reporting a success.
	if out, err := cmd("promote", "meil@uwaterloo.ca"); err != nil || !strings.Contains(out, "already an admin") {
		t.Errorf("second promote = %q, %v", out, err)
	}

	if out, _ := cmd("list"); !strings.Contains(out, "meil@uwaterloo.ca") {
		t.Errorf("list = %q", out)
	}

	if _, err := cmd("demote", "meil@uwaterloo.ca"); err == nil || !strings.Contains(err.Error(), "last admin") {
		t.Errorf("demoting the last admin: %v", err)
	}

	if _, err := cmd("promote", "nobody@uwaterloo.ca"); err == nil || !strings.Contains(err.Error(), "no account") {
		t.Errorf("unknown address: %v", err)
	}

	// Exactly one ledger row: the refusals and the no-op wrote nothing.
	rows, err := q.ListAdminActions(ctx, sqlcgen.ListAdminActionsParams{Limit: 10})
	if err != nil || len(rows) != 1 || rows[0].AdminAction.Reason != "Site operator" {
		t.Errorf("ledger = %+v, %v", rows, err)
	}
}
