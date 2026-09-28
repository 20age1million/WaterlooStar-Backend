package db_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/20age1million/WaterlooStar-Backend/internal/db"
	"github.com/20age1million/WaterlooStar-Backend/internal/db/dbtest"
	"github.com/20age1million/WaterlooStar-Backend/internal/db/sqlcgen"
)

// The admin read surface, the ledger, and the transaction that ties a change to
// its ledger row. The last is the one worth testing hardest: an admin change
// that commits without its row, or a row that commits without its change, is
// exactly what this phase exists to make impossible.

func strp(s string) *string { return &s }
func boolp(b bool) *bool    { return &b }

func TestAdminAccountQueries(t *testing.T) {
	t.Parallel()
	q, _ := dbtest.Queries(t)
	ctx := context.Background()

	owner := dbtest.User(t, q, "meil@uwaterloo.ca", true)
	_ = dbtest.User(t, q, "quietone@uwaterloo.ca", false)
	dbtest.Listing(t, q, owner.ID, dbtest.ListingOptions{})
	dbtest.Listing(t, q, owner.ID, dbtest.ListingOptions{Status: "draft"})
	dbtest.Request(t, q, owner.ID, dbtest.RequestOptions{})

	list := func(p sqlcgen.ListUsersForAdminParams) []string {
		t.Helper()
		p.Limit = 50
		rows, err := q.ListUsersForAdmin(ctx, p)
		if err != nil {
			t.Fatalf("ListUsersForAdmin(%+v): %v", p, err)
		}
		count, err := q.CountUsersForAdmin(ctx, sqlcgen.CountUsersForAdminParams{
			Search: p.Search, Role: p.Role, Verified: p.Verified, Suspended: p.Suspended,
		})
		if err != nil {
			t.Fatalf("CountUsersForAdmin: %v", err)
		}
		if int(count) != len(rows) {
			t.Errorf("count %d disagrees with %d rows for %+v", count, len(rows), p)
		}
		names := make([]string, 0, len(rows))
		for _, r := range rows {
			names = append(names, r.Username)
		}
		return names
	}

	cases := []struct {
		name string
		p    sqlcgen.ListUsersForAdminParams
		want int
	}{
		{"everyone", sqlcgen.ListUsersForAdminParams{}, 2},
		{"email fragment, any case", sqlcgen.ListUsersForAdminParams{Search: strp("MEIL@")}, 1},
		{"username fragment", sqlcgen.ListUsersForAdminParams{Search: strp("quiet")}, 1},
		{"unverified", sqlcgen.ListUsersForAdminParams{Verified: boolp(false)}, 1},
		{"not suspended", sqlcgen.ListUsersForAdminParams{Suspended: boolp(false)}, 2},
		{"suspended", sqlcgen.ListUsersForAdminParams{Suspended: boolp(true)}, 0},
		{"no admins yet", sqlcgen.ListUsersForAdminParams{Role: strp("admin")}, 0},
		{"combined", sqlcgen.ListUsersForAdminParams{Search: strp("i"), Verified: boolp(true), Role: strp("user")}, 1},
	}
	for _, tc := range cases {
		if got := list(tc.p); len(got) != tc.want {
			t.Errorf("%s: got %v, want %d", tc.name, got, tc.want)
		}
	}

	// Counts include every status: the draft is counted.
	detail, err := q.GetUserForAdmin(ctx, owner.ID)
	if err != nil {
		t.Fatalf("GetUserForAdmin: %v", err)
	}
	if detail.ListingCount != 2 || detail.RequestCount != 1 || detail.OfferCount != 0 {
		t.Errorf("counts = %d listings, %d requests, %d offers", detail.ListingCount, detail.RequestCount, detail.OfferCount)
	}

	overview, err := q.AdminOverviewCounts(ctx)
	if err != nil {
		t.Fatalf("AdminOverviewCounts: %v", err)
	}
	if overview.UsersTotal != 2 || overview.UsersVerified != 1 || overview.ListingsTotal != 2 ||
		overview.ListingsPublished != 1 || overview.RequestsTotal != 1 || overview.UsersAdmin != 0 {
		t.Errorf("overview = %+v", overview)
	}
}

func TestChangeRoleWritesTheLedgerInTheSameTransaction(t *testing.T) {
	t.Parallel()
	q, pool := dbtest.Queries(t)
	ctx := context.Background()
	run := db.PoolTx(pool)

	user := dbtest.User(t, q, "operator@uwaterloo.ca", true)

	change, err := db.ChangeRole(ctx, run, nil, user.ID, "admin", "Site operator")
	if err != nil {
		t.Fatalf("promote: %v", err)
	}
	if change.Previous != "user" || change.User.Role != "admin" {
		t.Errorf("change = %s → %s", change.Previous, change.User.Role)
	}

	admins, err := q.CountAdmins(ctx)
	if err != nil || admins != 1 {
		t.Fatalf("CountAdmins = %d (%v)", admins, err)
	}

	entries, err := q.ListAdminActionsForSubject(ctx, sqlcgen.ListAdminActionsForSubjectParams{
		SubjectType: db.SubjectUser, SubjectID: user.ID,
	})
	if err != nil || len(entries) != 1 {
		t.Fatalf("ListAdminActionsForSubject: %v (%d)", err, len(entries))
	}
	e := entries[0]
	if e.AdminAction.ActorID != nil || e.ActorUsername != nil {
		t.Error("a change from the host should have no actor")
	}
	var detail map[string]any
	if err := json.Unmarshal(e.AdminAction.Detail, &detail); err != nil || detail["from"] != "user" || detail["to"] != "admin" {
		t.Errorf("detail = %s (%v)", e.AdminAction.Detail, err)
	}

	// Asking for the role already held changes nothing and logs nothing.
	if _, err := db.ChangeRole(ctx, run, nil, user.ID, "admin", "Again"); !errors.Is(err, db.ErrRoleUnchanged) {
		t.Errorf("repeat promote: %v, want ErrRoleUnchanged", err)
	}

	// The last admin cannot be demoted, and the refusal leaves no trace: the
	// role update inside the transaction is rolled back with it.
	if _, err := db.ChangeRole(ctx, run, nil, user.ID, "user", "Stepping down"); !errors.Is(err, db.ErrLastAdmin) {
		t.Fatalf("demote last admin: %v, want ErrLastAdmin", err)
	}
	still, err := q.GetUserByID(ctx, user.ID)
	if err != nil || still.Role != "admin" {
		t.Errorf("after refused demotion role = %q (%v), want admin", still.Role, err)
	}

	// With a second admin, the first can step down.
	second := dbtest.User(t, q, "second@uwaterloo.ca", true)
	if _, err := db.ChangeRole(ctx, run, &user.ID, second.ID, "admin", "Handing over"); err != nil {
		t.Fatalf("promote second: %v", err)
	}
	if _, err := db.ChangeRole(ctx, run, &second.ID, user.ID, "user", "Stepping down"); err != nil {
		t.Fatalf("demote first: %v", err)
	}

	total, err := q.CountAdminActions(ctx)
	if err != nil || total != 3 {
		t.Errorf("CountAdminActions = %d (%v), want 3: refusals are not logged", total, err)
	}
	page, err := q.ListAdminActions(ctx, sqlcgen.ListAdminActionsParams{Limit: 10})
	if err != nil || len(page) != 3 {
		t.Fatalf("ListAdminActions: %v (%d)", err, len(page))
	}
	if page[0].ActorUsername == nil || *page[0].ActorUsername != second.Username {
		t.Errorf("newest entry should name its actor, got %v", page[0].ActorUsername)
	}
}

func TestAuditedRollsBackTheChangeWhenTheLedgerRefusesIt(t *testing.T) {
	t.Parallel()
	q, pool := dbtest.Queries(t)
	ctx := context.Background()

	user := dbtest.User(t, q, "subject@uwaterloo.ca", true)

	// An action name the CHECK does not know: the change runs, the insert
	// fails, and the change must not survive.
	_, err := db.Audited(ctx, db.PoolTx(pool), db.Action{
		Action: "erase_everything", SubjectType: db.SubjectUser, SubjectID: user.ID, Reason: "typo",
	}, func(tx sqlcgen.Querier) (map[string]any, error) {
		_, err := tx.SetUserRole(ctx, sqlcgen.SetUserRoleParams{ID: user.ID, Role: "admin"})
		return nil, err
	})
	if err == nil {
		t.Fatal("an unknown action was accepted by the ledger")
	}
	after, _ := q.GetUserByID(ctx, user.ID)
	if after.Role != "user" {
		t.Errorf("role = %q: the change committed without its ledger row", after.Role)
	}

	// And the other way round: a failed change writes no row.
	boom := errors.New("change failed")
	_, err = db.Audited(ctx, db.PoolTx(pool), db.Action{
		Action: db.ActionVerify, SubjectType: db.SubjectUser, SubjectID: user.ID, Reason: "should not log",
	}, func(sqlcgen.Querier) (map[string]any, error) { return nil, boom })
	if !errors.Is(err, boom) {
		t.Errorf("err = %v, want the change's error", err)
	}

	// A blank reason is refused before anything is written.
	_, err = db.Audited(ctx, db.PoolTx(pool), db.Action{
		Action: db.ActionVerify, SubjectType: db.SubjectUser, SubjectID: user.ID, Reason: "   ",
	}, func(sqlcgen.Querier) (map[string]any, error) { return nil, nil })
	if !errors.Is(err, db.ErrReasonRequired) {
		t.Errorf("blank reason: %v, want ErrReasonRequired", err)
	}

	if n, _ := q.CountAdminActions(ctx); n != 0 {
		t.Errorf("ledger has %d rows, want 0", n)
	}
}

func TestLedgerConstraints(t *testing.T) {
	t.Parallel()
	q, _ := dbtest.Queries(t)
	ctx := context.Background()

	valid := sqlcgen.InsertAdminActionParams{
		Action: db.ActionSetRole, SubjectType: db.SubjectUser, SubjectID: uuid.New(),
		Reason: "A reason", Detail: []byte(`{}`),
	}
	if _, err := q.InsertAdminAction(ctx, valid); err != nil {
		t.Fatalf("valid insert: %v", err)
	}

	bad := map[string]func(p *sqlcgen.InsertAdminActionParams){
		"unknown action":  func(p *sqlcgen.InsertAdminActionParams) { p.Action = "delete_user" },
		"unknown subject": func(p *sqlcgen.InsertAdminActionParams) { p.SubjectType = "comment" },
		"blank reason":    func(p *sqlcgen.InsertAdminActionParams) { p.Reason = "  " },
	}
	for name, mutate := range bad {
		p := valid
		mutate(&p)
		if _, err := q.InsertAdminAction(ctx, p); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
}
