# Phase 0 — Contract and Scaffolding

**Status:** Ready
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
- Go 1.25 or later on PATH
- `migrate`, `sqlc` and `oapi-codegen` binaries installable via `go install`

---

## Implementation Steps

- [ ] **Preparation**
  - [ ] Confirm prerequisites are available
  - [ ] Update `docs/specs/active/platform-foundation/index.md` — set Phase 0 to In Progress

- [ ] **Module and layout**
  - [ ] Run `go mod init github.com/20age1million/waterloostar-api`
  - [ ] Create the directory skeleton: `cmd/api/`, `internal/config/`, `internal/db/`,
        `internal/httpapi/`, `internal/middleware/`, `api/`, `migrations/`
  - [ ] Add `Makefile` targets for `migrate-up`, `migrate-down`, `generate`, `run`, `test`
  - [ ] Verify: `go build ./...` succeeds on the empty skeleton

- [ ] **Local database**
  - [ ] Create `docker-compose.yml` with a `postgres:18` service, a named volume, and port 5432
  - [ ] Create `.env.example` with `PG_DSN`, `PORT`, `CORS_ORIGIN`, `LOG_LEVEL`
  - [ ] Document the startup sequence in `README.md`
  - [ ] Verify: `docker compose up -d` starts, and `docker compose ps` reports healthy

- [ ] **Migration harness**
  - [ ] Add `migrations/000001_init.up.sql` and `migrations/000001_init.down.sql`
        creating the `waterloostar` schema baseline — no domain tables yet
  - [ ] Wire `make migrate-up` and `make migrate-down` to `golang-migrate` against `PG_DSN`
  - [ ] Verify: `make migrate-up` then `make migrate-down` both succeed, and
        `schema_migrations` reflects the version each time

- [ ] **Database access and codegen**
  - [ ] Add `internal/db/pool.go` opening a `pgxpool` from the DSN with sensible pool limits
        and a startup ping that fails fast
  - [ ] Add `sqlc.yaml` pointing at `migrations/` for schema and `internal/db/queries/` for queries,
        emitting into `internal/db/sqlc/` with `pgx/v5` and UUID type overrides
  - [ ] Add `internal/db/queries/health.sql` with a single `Ping` query
  - [ ] Verify: `sqlc generate` produces compiling Go, and `go build ./...` passes

- [ ] **The contract**
  - [ ] Write `api/openapi.yaml` — OpenAPI 3.1, `info`, `servers`, and `components.schemas`
        for `Error` (the single error envelope: `code`, `message`, optional `details`) and
        `PageMeta` (`page`, `per_page`, `total`, `total_pages`)
  - [ ] Define `GET /healthz` with its 200 and 503 responses
  - [ ] Add `api/oapi-codegen.yaml` configured for the gin server and strict handlers,
        emitting `internal/httpapi/gen/`
  - [ ] Verify: `oapi-codegen` runs clean and the generated server interface compiles

- [ ] **Configuration and middleware**
  - [ ] Add `internal/config/config.go` — load `.env` if present, bind and validate every
        variable, return a typed struct, fail fast with a readable message on a missing DSN
  - [ ] Add `internal/middleware/` — request id, `log/slog` structured request logging,
        panic recovery returning the standard error envelope, and CORS allowing the configured
        origin with credentials
  - [ ] Verify: a request to an unknown path returns the standard error envelope, not gin's default

- [ ] **Wire the service**
  - [ ] Implement `internal/httpapi/router.go` building the gin engine and mounting the
        generated interface
  - [ ] Implement the health handler — report `ok` plus a database round-trip via the sqlc
        `Ping` query, returning 503 when the database is unreachable
  - [ ] Write `cmd/api/main.go` — load config, open the pool, build the router, listen, and
        shut down gracefully on SIGINT/SIGTERM
  - [ ] Verify: `go run ./cmd/api` then `curl localhost:8080/healthz` returns 200 with database ok;
        stopping the database container makes the same call return 503

- [ ] **Tests**
  - [ ] Add `internal/httpapi/health_test.go` covering the health handler on both the healthy
        and unreachable-database paths using an httptest server
  - [ ] Add `internal/config/config_test.go` covering validation failure on a missing DSN
  - [ ] Verify: `go test ./...` passes

- [ ] **Final verification**
  - [ ] `go build ./...` passes with no errors
  - [ ] `go vet ./...` reports nothing
  - [ ] `go test ./...` passes
  - [ ] `sqlc generate` and `oapi-codegen` produce no diff when re-run
  - [ ] Update `index.md` — set Phase 0 to Complete
  - [ ] Move `docs/specs/active/platform-foundation/phase-0-contract-and-scaffolding.md` →
        `docs/specs/implemented/platform-foundation/phase-0-contract-and-scaffolding.md`

---

## Completion Criteria

- [ ] All checklist items completed and verified
- [ ] A single documented command sequence takes a clean machine to a responding `/healthz`
- [ ] The migration harness rolls forward and back cleanly
- [ ] Both code generators run from `make generate` and their output compiles
- [ ] No regressions in related areas
- [ ] `index.md` phase status set to Complete
- [ ] Phase spec doc moved to `docs/specs/implemented/platform-foundation/`

---

## Implementation Notes

> *Added after completion. Fill in before committing the phase.*
>
> **Key files changed:**
> - `path/to/file`: what changed
>
> **Divergences from plan:**
> - <none / description>
>
> **Verification run:**
> - <command or action and its result>
