# Phase 5 — Query tests against a real database

**Status:** Complete
**Feature:** [Housing Requests](./index.md)
**Objective:** Make a malformed query fail a test rather than a deploy, before this feature writes a dozen more of them.

---

## Scope

### In scope

- `internal/db/dbtest`: a harness that gives a test a migrated, private
  PostgreSQL database and takes it away afterwards.
- Query tests for the existing surface — listings and auth — so the harness is
  proven against queries whose behaviour is already known.
- A guard that fails when a query exists with no test.
- CI: a PostgreSQL service for the test step.

### Out of scope

- Rewriting the handler tests. The in-memory fake in
  `internal/httpapi/fake_querier_test.go` stays: it is fast and it tests
  handlers, which is a different job.
- Any change to production code. If this phase changes behaviour, something is
  wrong.

---

## Dependencies / Prerequisites

- Docker, for a PostgreSQL to test against. Already required by
  `docker-compose.yml` for development and present on the Drone runner.
- `golang-migrate`, already a dependency and already used by `cmd/migrate`.

---

## Implementation Steps

- [x] **Preparation**
  - [x] Confirm prerequisites are available
  - [x] Update `docs/specs/active/housing-requests/index.md` — set Phase 5 to In Progress

- [x] **The harness**
  - [x] Add `internal/db/dbtest/dbtest.go` exposing `New(t *testing.T) *pgxpool.Pool`
  - [x] Read `PG_TEST_DSN`; when unset, skip the test with a message naming the
        variable and how to set it from `docker-compose.yml`
  - [x] Create a uniquely named database per test, migrate it with the
        migrations in `migrations/`, and drop it in `t.Cleanup`
  - [x] Locate `migrations/` from the source file rather than the working
        directory, so the harness works from any package
  - [x] Verify: two tests running in parallel each get their own database and
        neither sees the other's rows

- [x] **Fixtures**
  - [x] Add `dbtest.User(t, pool, ...)` and `dbtest.Listing(t, pool, ...)`
        returning rows with sensible defaults, so a test states only what it
        cares about
  - [x] Verify: a test can create a verified user and a published listing in
        two lines

- [x] **Query tests for what already exists**
  - [x] `internal/db/queries_listings_test.go` — every listing query, including
        the discovery filters and `SetListingStatus`, the one Phase 4 shipped
        broken
  - [x] `internal/db/queries_auth_test.go` — users, verification, password
        reset and refresh tokens
  - [x] Verify: reintroducing the Phase 4 bug (a parameter used as both
        `varchar` and `::text`) makes the suite fail

- [x] **The guard**
  - [x] Add a test that reflects over the generated `sqlcgen.Querier` interface
        and compares its method set against the queries these tests exercise
  - [x] Fail with the names of any query that has no test
  - [x] Verify: adding a query to `internal/db/queries/` and regenerating makes
        the suite fail until a test covers it

- [x] **CI**
  - [x] Add a `postgres:18` service to the test step in `.drone.yml` and set
        `PG_TEST_DSN` for it
  - [x] Verify: the pipeline runs the query tests, and they do not silently skip

- [x] **Final verification**
  - [x] `go vet ./...` and `go build ./...` pass
  - [x] `go test ./...` passes with a database available, and still passes with
        `PG_TEST_DSN` unset — skipping the query tests, not failing them
  - [x] Update `index.md` — set Phase 5 to Complete
  - [x] Move `docs/specs/active/housing-requests/phase-5-query-test-layer.md` →
        `docs/specs/implemented/housing-requests/phase-5-query-test-layer.md`

---

## Completion Criteria

- [x] All checklist items complete
- [x] Every query in `internal/db/queries/` runs against a real PostgreSQL in
      at least one test
- [x] A new query with no test fails the suite
- [x] CI runs the query tests; a developer without Docker still gets a green
      `go test ./...`
- [x] No production code changed

---

## Implementation Notes

**Delivered as specified.** 32 queries, all executed against PostgreSQL 18 by
`internal/db/queries_auth_test.go` and `queries_listings_test.go`, with
`internal/db/dbtest` creating and dropping a database per test.

**Proved rather than asserted.** Three claims this phase rests on were each
checked by deliberately breaking something:

- **The Phase 4 bug is caught.** Reverting `SetListingStatus` to the version
  that used one parameter as both `varchar` and `::text`, then regenerating,
  makes the suite fail with `SQLSTATE 42P08` — the error that reached
  production. Restored afterwards; the generated code is byte-identical to
  what was committed.
- **The guard catches an untested query.** Removing the `Ping` call made it fail
  naming `Ping`.
- **No database is not a failure.** With `PG_TEST_DSN` unset, `go test ./...`
  passes and the query tests skip with instructions.

**The guard was wrong on its first attempt.** It scanned every `.go` file in
`internal/db`, and `pool.go` calls `Ping` itself — so the first run reported
full coverage while `Ping` had no test at all. It now counts only `_test.go`
files here, plus the `dbtest` fixtures. A coverage check that cannot fail is
worse than none, because it is believed.

**Found, not fixed — `published_at` is never set on creation.** `CreateListing`
does not touch the column, so a listing posted through `POST /listings` has
`published_at IS NULL`, while one that was paused and republished carries a
timestamp. Nothing reads the column today — no query, handler or contract field
— so nothing is broken, but the first feature to sort by it will find most rows
empty. Left alone because this phase changes no production code. Phase 6 gives
requests the same lifecycle and should not repeat it.

**Windows.** `golang-migrate`'s `file://` source cannot parse a Windows path;
the harness uses the `iofs` source over `os.DirFS` instead, which behaves the
same on both platforms.

**`go mod tidy`** promoted `golang-jwt`, `oapi-codegen/nullable`,
`oapi-codegen/runtime` and `golang.org/x/crypto` from indirect to direct. They
were always imported directly and merely mislabelled; no dependency was added
or removed.
