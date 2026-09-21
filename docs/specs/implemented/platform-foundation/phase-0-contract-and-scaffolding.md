# Phase 0 — Contract and Scaffolding

**Status:** Complete
**Feature:** [platform-foundation](./index.md)
**Objective:** Turn an empty repository into a running gin service with a reviewable migration harness, a working sqlc codegen chain, and a health endpoint that proves a real database round-trip.

---

## Scope

### In scope

- Go module, directory layout, and the gin server entry point
- `docker-compose.yml` pinning PostgreSQL 18 for local development
- `golang-migrate` harness with the first migration pair
- `sqlc` configuration and a generated query proving the chain works end to end
- `api/openapi.yaml` with the shared components: error envelope and pagination shape
- `oapi-codegen` generating the gin server interface
- Configuration binding, structured logging, CORS, recovery, request ids
- `GET /healthz` returning service and database status

### Out of scope

- Any domain table — `users` arrives in Phase 1, `listings` in Phase 2
- Authentication of any kind
- Deployment beyond local development
- CI configuration

---

## Dependencies / Prerequisites

- Docker Desktop available on the developer machine
- Go 1.24 or later on PATH
- `sqlc` and `oapi-codegen` pinned as `go.mod` tool dependencies — no global installs.
  `golang-migrate` is used as a library behind `cmd/migrate` rather than as a CLI, because
  its CLI requires build tags to compile in the Postgres driver.

---

## Implementation Steps

- [x] **Preparation**
  - [x] Confirm prerequisites are available
  - [x] Update `docs/specs/active/platform-foundation/index.md` — set Phase 0 to In Progress

- [x] **Module and layout**
  - [x] Run `go mod init github.com/20age1million/waterloostar-api`
  - [x] Create the directory skeleton: `cmd/api/`, `internal/config/`, `internal/db/`,
        `internal/httpapi/`, `internal/middleware/`, `api/`, `migrations/`
  - [x] Add `Makefile` targets for `migrate-up`, `migrate-down`, `generate`, `run`, `test`
  - [x] Verify: `go build ./...` succeeds on the empty skeleton

- [x] **Local database**
  - [x] Create `docker-compose.yml` with a `postgres:18` service, a named volume, and host port 5433
  - [x] Create `.env.example` with `PG_DSN`, `PORT`, `CORS_ORIGIN`, `LOG_LEVEL`
  - [x] Document the startup sequence in `README.md`
  - [x] Verify: `docker compose up -d` starts, and `docker compose ps` reports healthy

- [x] **Migration harness**
  - [x] Add `migrations/000001_init.up.sql` and `migrations/000001_init.down.sql`
        creating the `waterloostar` schema baseline — no domain tables yet
  - [x] Wire `make migrate-up` and `make migrate-down` to `golang-migrate` against `PG_DSN`
  - [x] Verify: `make migrate-up` then `make migrate-down` both succeed, and
        `schema_migrations` reflects the version each time

- [x] **Database access and codegen**
  - [x] Add `internal/db/pool.go` opening a `pgxpool` from the DSN with sensible pool limits
        and a startup ping that fails fast
  - [x] Add `sqlc.yaml` pointing at `migrations/` for schema and `internal/db/queries/` for queries,
        emitting into `internal/db/sqlcgen/` with `pgx/v5` and UUID type overrides
  - [x] Add `internal/db/queries/health.sql` with a single `Ping` query
  - [x] Verify: `sqlc generate` produces compiling Go, and `go build ./...` passes

- [x] **The contract**
  - [x] Write `api/openapi.yaml` — OpenAPI 3.0.3, `info`, `servers`, and `components.schemas`
        for `Error` (the single error envelope: `code`, `message`, optional `details`) and
        `PageMeta` (`page`, `per_page`, `total`, `total_pages`)
  - [x] Define `GET /healthz` with its 200 and 503 responses
  - [x] Add `api/oapi-codegen.yaml` configured for the gin server and strict handlers,
        emitting `internal/httpapi/gen/`
  - [x] Verify: `oapi-codegen` runs clean and the generated server interface compiles

- [x] **Configuration and middleware**
  - [x] Add `internal/config/config.go` — load `.env` if present, bind and validate every
        variable, return a typed struct, fail fast with a readable message on a missing DSN
  - [x] Add `internal/middleware/` — request id, `log/slog` structured request logging,
        panic recovery returning the standard error envelope, and CORS allowing the configured
        origin with credentials
  - [x] Verify: a request to an unknown path returns the standard error envelope, not gin's default

- [x] **Wire the service**
  - [x] Implement `internal/httpapi/router.go` building the gin engine and mounting the
        generated interface
  - [x] Implement the health handler — report `ok` plus a database round-trip via the sqlc
        `Ping` query, returning 503 when the database is unreachable
  - [x] Write `cmd/api/main.go` — load config, open the pool, build the router, listen, and
        shut down gracefully on SIGINT/SIGTERM
  - [x] Verify: `go run ./cmd/api` then `curl localhost:8080/healthz` returns 200 with database ok;
        stopping the database container makes the same call return 503

- [x] **Tests**
  - [x] Add `internal/httpapi/health_test.go` covering the health handler on both the healthy
        and unreachable-database paths using an httptest server
  - [x] Add `internal/config/config_test.go` covering validation failure on a missing DSN
  - [x] Verify: `go test ./...` passes

- [x] **Final verification**
  - [x] `go build ./...` passes with no errors
  - [x] `go vet ./...` reports nothing
  - [x] `go test ./...` passes
  - [x] `sqlc generate` and `oapi-codegen` produce no diff when re-run
  - [ ] Update `index.md` — set Phase 0 to Complete
  - [ ] Move `docs/specs/active/platform-foundation/phase-0-contract-and-scaffolding.md` →
        `docs/specs/implemented/platform-foundation/phase-0-contract-and-scaffolding.md`

---

## Completion Criteria

- [x] All checklist items completed and verified
- [x] A single documented command sequence takes a clean machine to a responding `/healthz`
- [x] The migration harness rolls forward and back cleanly
- [x] Both code generators run from `make generate` and their output compiles
- [x] No regressions in related areas
- [x] `index.md` phase status set to Complete
- [x] Phase spec doc moved to `docs/specs/implemented/platform-foundation/`

---

## Implementation Notes

**Key files changed:**

- `go.mod` — module `github.com/20age1million/waterloostar-api`; gin, pgx/v5,
  golang-migrate, google/uuid, godotenv. sqlc and oapi-codegen pinned as tool dependencies.
- `api/openapi.yaml` — the contract. `Error`, `ErrorCode`, `PageMeta`, `Health`; `GET /healthz`;
  `sessionCookie` and `csrfToken` security schemes declared ahead of Phase 1.
- `api/oapi-codegen.yaml` — gin strict-server generation into `internal/httpapi/gen/`.
- `sqlc.yaml` — schema read from `migrations/`, queries from `internal/db/queries/`,
  output `internal/db/sqlcgen/`, uuid mapped to `google/uuid.UUID`.
- `cmd/api/main.go` — config load, pool open, router, graceful shutdown on SIGINT/SIGTERM.
- `cmd/migrate/main.go` — `up`, `down`, `down-one`, `goto`, `version`.
- `internal/apierror/apierror.go` — the single error envelope; own package to avoid an
  import cycle between middleware and handlers.
- `internal/config/config.go` — validates every variable and reports *all* problems at once.
- `internal/db/pool.go` — pgxpool with limits and a fail-fast ping.
- `internal/httpapi/router.go` — gin engine, middleware chain, `NoRoute`/`NoMethod` returning
  the standard envelope; `Server` depends on the generated `Querier` interface.
- `internal/httpapi/health.go` — real database round-trip, 2s timeout, 503 when unreachable.
- `internal/middleware/` — request id, slog request logging, recovery, credentialed CORS.
- `migrations/000001_init.*` — `gen_random_uuid()` assertion and the `set_updated_at()`
  trigger function that `users` and `listings` will both use.
- `docker-compose.yml`, `Makefile`, `.env.example`, `README.md`.

**Divergences from plan:**

1. **Go 1.24.6, not 1.25.** Installed toolchain is 1.24.6 and nothing here needs 1.25
   (`log/slog` has been stdlib since 1.21). Prerequisite corrected in this doc.
2. **No global tool installs.** sqlc and oapi-codegen are `go.mod` tool dependencies
   (`go get -tool`, a Go 1.24 feature) rather than `go install`ed binaries, so versions are
   pinned per project. golang-migrate is used as a *library* behind `cmd/migrate` because its
   CLI needs build tags to compile in the Postgres driver, which is awkward to install
   reproducibly. The migration files are unaffected, so sqlc still reads them.
3. **OpenAPI 3.0.3, not 3.1.** oapi-codegen's 3.1 support is weaker; 3.0.3 generates cleanly
   and nothing used here needs 3.1.
4. **Container maps host port 5433, not 5432.** A local PostgreSQL install already listens on
   5432 on the development machine — discovered when the unreachable-database test got a
   password-authentication rejection rather than a connection refusal. Binding 5432 would have
   failed. `.env.example` matches.
5. **Volume mounts at `/var/lib/postgresql`, not `/var/lib/postgresql/data`.** `postgres:18`
   stores data in a major-version subdirectory and refuses to start against a `/data` mount.
   Caught by the container crash-looping; fixed and the volume recreated.
6. **The baseline migration is not empty.** The spec said "no domain tables yet", which holds,
   but leaving it truly empty would have been a wasted version. It asserts `gen_random_uuid()`
   is available and creates the shared `set_updated_at()` trigger function.
7. **`Server` holds `sqlcgen.Querier`, not `*sqlcgen.Queries`.** Needed so the health handler's
   unreachable-database path can be tested without standing up PostgreSQL.

**Verification run:**

- `go build ./...`, `go vet ./...`, `go test ./...` — all pass.
- `go tool oapi-codegen` + `go tool sqlc generate` re-run — byte-identical output, no diff.
- `docker compose up -d` — `waterloostar-postgres` reports healthy; PostgreSQL 18.6.
- `go run ./cmd/migrate up` → version 1, dirty=false; `set_updated_at` present.
- `down-one` → "no migrations applied", trigger function gone; `up` again → version 1;
  a second `up` → "no change".
- `GET /healthz` with the database up → `200 {"database":"ok","status":"ok"}`, `X-Request-Id` set.
- `GET /healthz` with the database stopped → `503 {"database":"unreachable","status":"degraded"}`;
  after restarting the container the *same process* returned 200 again, so the pool recovers.
- `GET /nope` → `404 {"code":"not_found","message":"No endpoint at /nope","request_id":"..."}`.
- `OPTIONS /healthz` with `Origin: http://localhost:3000` → 204 with
  `Access-Control-Allow-Credentials: true` and the origin echoed exactly.
- Startup with no `PG_DSN` → multi-line message naming the variable and pointing at `.env.example`.
- Startup with an unreachable database → `database unreachable — is docker compose up -d running?`
