// Package dbtest gives a test its own migrated PostgreSQL database.
//
// It exists because the handler tests run against an in-memory fake, which
// cannot tell whether a query is valid SQL. Phase 4 shipped one that was not:
// it used a parameter as both varchar and ::text, every test passed, and
// pausing a listing failed against the real database. Anything in
// internal/db/queries/ is only as good as the database's opinion of it.
//
// Each test gets a database of its own, created and dropped around it, so tests
// can run in parallel and none of them can see another's rows — or the
// developer's.
package dbtest

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/20age1million/WaterlooStar-Backend/internal/db/sqlcgen"
)

// DSNEnv names the connection string these tests use. It points at a server,
// not at the database they run in: each test makes its own.
const DSNEnv = "PG_TEST_DSN"

const skipMessage = DSNEnv + ` is not set, so the query tests cannot run.

Start the development database and point this at it:

    docker compose up -d
    ` + DSNEnv + `=postgres://waterloostar:waterloostar@localhost:5433/waterloostar?sslmode=disable go test ./...

The tests never touch that database itself — each one creates and drops its own.`

// New returns a pool onto a freshly migrated database belonging to this test.
//
// Skips — rather than fails — when no server is configured, so that a developer
// without Docker still gets a green `go test ./...`. CI sets DSNEnv, and the
// coverage guard in this package's tests is what stops a query from going
// untested.
func New(t *testing.T) *pgxpool.Pool {
	t.Helper()

	adminDSN := strings.TrimSpace(os.Getenv(DSNEnv))
	if adminDSN == "" {
		t.Skip(skipMessage)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// A name no other test can collide with, short enough for PostgreSQL's
	// 63-byte identifier limit.
	name := "waterloostar_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:16]

	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatalf("connect to %s: %v\n\n%s", DSNEnv, err, skipMessage)
	}
	// CREATE DATABASE takes no parameters, so the name is quoted as an
	// identifier. It is generated above, not taken from anywhere.
	if _, err := admin.Exec(ctx, `CREATE DATABASE "`+name+`"`); err != nil {
		_ = admin.Close(ctx)
		t.Fatalf("create test database: %v", err)
	}
	_ = admin.Close(ctx)

	dsn, err := withDatabase(adminDSN, name)
	if err != nil {
		t.Fatalf("build test DSN: %v", err)
	}

	if err := migrateUp(dsn); err != nil {
		t.Fatalf("migrate test database: %v", err)
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("open pool on test database: %v", err)
	}

	t.Cleanup(func() {
		pool.Close()

		// A separate context: the test's may already be cancelled, and the
		// database has to go regardless or the server collects rubbish.
		dropCtx, dropCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer dropCancel()

		conn, err := pgx.Connect(dropCtx, adminDSN)
		if err != nil {
			t.Logf("drop test database %s: connect: %v", name, err)
			return
		}
		defer func() { _ = conn.Close(dropCtx) }()

		// FORCE: a leaked connection should not leave the database behind.
		if _, err := conn.Exec(dropCtx, `DROP DATABASE IF EXISTS "`+name+`" WITH (FORCE)`); err != nil {
			t.Logf("drop test database %s: %v", name, err)
		}
	})

	return pool
}

// Queries is New plus the generated query set, which is what most tests want.
func Queries(t *testing.T) (*sqlcgen.Queries, *pgxpool.Pool) {
	t.Helper()
	pool := New(t)
	return sqlcgen.New(pool), pool
}

// withDatabase rewrites the DSN's database name, leaving credentials and
// options as they were.
func withDatabase(dsn, name string) (string, error) {
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		return "", fmt.Errorf("parse %s: %w", DSNEnv, err)
	}

	host := cfg.Host
	if strings.Contains(host, ":") && !strings.HasPrefix(host, "[") {
		host = "[" + host + "]" // IPv6
	}
	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable",
		cfg.User, cfg.Password, host, cfg.Port, name), nil
}

func migrateUp(dsn string) error {
	// An io/fs source rather than a "file://" URL: golang-migrate parses that
	// URL, and a Windows path — with its drive letter and backslashes — does
	// not survive the round trip. A directory handle has no such opinion.
	source, err := iofs.New(os.DirFS(migrationsDir()), ".")
	if err != nil {
		return fmt.Errorf("open migrations: %w", err)
	}

	m, err := migrate.NewWithSourceInstance("iofs", source, dsn)
	if err != nil {
		return fmt.Errorf("open migrator: %w", err)
	}
	defer func() { _, _ = m.Close() }()

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("apply migrations: %w", err)
	}
	return nil
}

// migrationsDir locates migrations/ from this source file rather than the
// working directory, so the harness works from any package's tests.
func migrationsDir() string {
	_, thisFile, _, _ := runtime.Caller(0)
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "migrations"))
}
