// Command admin grants and removes the admin role from the host.
//
// It is the only way the first admin can exist: no HTTP path grants the role to
// someone who does not already hold it. Every change it makes is written to the
// admin_actions ledger with no actor, so a grant from the host is as visible as
// one made in the portal.
//
//	go run ./cmd/admin promote meil@uwaterloo.ca -reason "Site operator"
//	go run ./cmd/admin demote  meil@uwaterloo.ca -reason "Stepped down"
//	go run ./cmd/admin list
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/20age1million/WaterlooStar-Backend/internal/config"
	"github.com/20age1million/WaterlooStar-Backend/internal/db"
	"github.com/20age1million/WaterlooStar-Backend/internal/db/sqlcgen"
)

// defaultReason is used when -reason is not given. The ledger requires a reason
// and the CLI should not make bootstrapping awkward, but a real one is better.
const defaultReason = "Changed from the host with cmd/admin"

const usage = `Usage:
  admin promote <email> [-reason "why"]   grant the admin role
  admin demote  <email> [-reason "why"]   remove it (never the last admin)
  admin list                              show every admin
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	ctx := context.Background()
	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := run(ctx, pool, os.Stdout, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run executes one subcommand. Split from main so the tests can drive it
// against a real database without a process boundary.
func run(ctx context.Context, pool *pgxpool.Pool, out io.Writer, args []string) error {
	switch args[0] {
	case "promote":
		return setRole(ctx, pool, out, args[1:], "admin")
	case "demote":
		return setRole(ctx, pool, out, args[1:], "user")
	case "list":
		return list(ctx, pool, out)
	default:
		return fmt.Errorf("unknown command %q\n\n%s", args[0], usage)
	}
}

func setRole(ctx context.Context, pool *pgxpool.Pool, out io.Writer, args []string, role string) error {
	email, reason, err := parseTarget(args)
	if err != nil {
		return err
	}

	q := sqlcgen.New(pool)
	user, err := q.GetUserByEmail(ctx, email)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("no account uses %s", email)
	}
	if err != nil {
		return err
	}

	change, err := db.ChangeRole(ctx, db.PoolTx(pool), nil, user.ID, role, reason)
	switch {
	case errors.Is(err, db.ErrRoleUnchanged):
		// Not an error, but not a success either: say what is true.
		fmt.Fprintf(out, "%s is already %s. Nothing changed.\n", user.Email, article(role))
		return nil
	case errors.Is(err, db.ErrLastAdmin):
		return fmt.Errorf("refusing to demote %s: they are the last admin. Promote someone else first", user.Email)
	case err != nil:
		return err
	}

	fmt.Fprintf(out, "%s: %s → %s. Their sessions were ended; the new role applies from their next login.\n",
		change.User.Email, change.Previous, change.User.Role)
	if role == "admin" && !change.User.Verified {
		fmt.Fprintln(out, "Note: this account has not verified its email address.")
	}
	return nil
}

func list(ctx context.Context, pool *pgxpool.Pool, out io.Writer) error {
	role := "admin"
	rows, err := sqlcgen.New(pool).ListUsersForAdmin(ctx, sqlcgen.ListUsersForAdminParams{
		Role: &role, Limit: 1000,
	})
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		fmt.Fprintln(out, "No admins yet. Create one with: admin promote <email>")
		return nil
	}
	for _, r := range rows {
		fmt.Fprintf(out, "%-40s %-20s since %s\n", r.Email, r.Username, r.CreatedAt.Format("2006-01-02"))
	}
	return nil
}

// parseTarget reads "<email> [-reason ...]", with the flag on either side of
// the address — Go's flag package stops at the first positional argument, and
// an operator should not have to remember which order it wants.
func parseTarget(args []string) (email, reason string, err error) {
	fs := flag.NewFlagSet("admin", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	r := fs.String("reason", defaultReason, "why the role is changing; recorded in the ledger")

	if err := fs.Parse(args); err != nil {
		return "", "", fmt.Errorf("%v\n\n%s", err, usage)
	}
	if fs.NArg() < 1 {
		return "", "", fmt.Errorf("an email address is required\n\n%s", usage)
	}
	email = strings.TrimSpace(fs.Arg(0))
	if err := fs.Parse(fs.Args()[1:]); err != nil {
		return "", "", fmt.Errorf("%v\n\n%s", err, usage)
	}
	if fs.NArg() > 0 {
		return "", "", fmt.Errorf("unexpected arguments: %s\n\n%s", strings.Join(fs.Args(), " "), usage)
	}
	return email, *r, nil
}

func article(role string) string {
	if role == "admin" {
		return "an admin"
	}
	return "a " + role
}
