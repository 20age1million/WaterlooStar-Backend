# Phase 5 — Query tests against a real database

**Status:** Ready
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

- [ ] **Preparation**
  - [ ] Confirm prerequisites are available
  - [ ] Update `docs/specs/active/housing-requests/index.md` — set Phase 5 to In Progress

- [ ] **The harness**
  - [ ] Add `internal/db/dbtest/dbtest.go` exposing `New(t *testing.T) *pgxpool.Pool`
  - [ ] Read `PG_TEST_DSN`; when unset, skip the test with a message naming the
        variable and how to set it from `docker-compose.yml`
  - [ ] Create a uniquely named database per test, migrate it with the
        migrations in `migrations/`, and drop it in `t.Cleanup`
  - [ ] Locate `migrations/` from the source file rather than the working
        directory, so the harness works from any package
  - [ ] Verify: two tests running in parallel each get their own database and
        neither sees the other's rows

- [ ] **Fixtures**
  - [ ] Add `dbtest.User(t, pool, ...)` and `dbtest.Listing(t, pool, ...)`
        returning rows with sensible defaults, so a test states only what it
        cares about
  - [ ] Verify: a test can create a verified user and a published listing in
        two lines

- [ ] **Query tests for what already exists**
  - [ ] `internal/db/queries_listings_test.go` — every listing query, including
        the discovery filters and `SetListingStatus`, the one Phase 4 shipped
        broken
  - [ ] `internal/db/queries_auth_test.go` — users, verification, password
        reset and refresh tokens
  - [ ] Verify: reintroducing the Phase 4 bug (a parameter used as both
        `varchar` and `::text`) makes the suite fail

- [ ] **The guard**
  - [ ] Add a test that reflects over the generated `sqlcgen.Querier` interface
        and compares its method set against the queries these tests exercise
  - [ ] Fail with the names of any query that has no test
  - [ ] Verify: adding a query to `internal/db/queries/` and regenerating makes
        the suite fail until a test covers it

- [ ] **CI**
  - [ ] Add a `postgres:18` service to the test step in `.drone.yml` and set
        `PG_TEST_DSN` for it
  - [ ] Verify: the pipeline runs the query tests, and they do not silently skip

- [ ] **Final verification**
  - [ ] `go vet ./...` and `go build ./...` pass
  - [ ] `go test ./...` passes with a database available, and still passes with
        `PG_TEST_DSN` unset — skipping the query tests, not failing them
  - [ ] Update `index.md` — set Phase 5 to Complete
  - [ ] Move `docs/specs/active/housing-requests/phase-5-query-test-layer.md` →
        `docs/specs/implemented/housing-requests/phase-5-query-test-layer.md`

---

## Completion Criteria

- [ ] All checklist items complete
- [ ] Every query in `internal/db/queries/` runs against a real PostgreSQL in
      at least one test
- [ ] A new query with no test fails the suite
- [ ] CI runs the query tests; a developer without Docker still gets a green
      `go test ./...`
- [ ] No production code changed
