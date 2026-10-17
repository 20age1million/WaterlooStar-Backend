# Development

## Layout

```
api/openapi.yaml         The contract. Authoritative — edit before the handler.
cmd/api                  Service entry point.
cmd/migrate              Migration runner (golang-migrate as a library).
cmd/seed                 Loads development fixtures. Refuses to run outside development.
cmd/admin                Grants and removes the admin role from the host. Every change is logged.
internal/apierror        The single error envelope every failure returns.
internal/auth            Password hashing, JWT mint/verify, cookie shaping, tokens.
internal/config          Environment binding and validation.
internal/db              pgx pool; queries/ holds the .sql sqlc generates from.
internal/db/sqlcgen      Generated. Do not edit.
internal/httpapi         Router and handlers.
internal/httpapi/gen     Generated from openapi.yaml. Do not edit.
internal/db/seed         Development fixtures: 6 prototype listings and 5 requests.
internal/email           Sender interface; Resend delivery, or the log without a key; the account and notification emails.
internal/middleware      Request id, logging, recovery, CORS, auth, CSRF.
internal/ratelimit       Token buckets and the limits, in policy.go.
internal/db/dbtest       Harness that runs queries against a real PostgreSQL.
migrations               Schema source of truth; sqlc reads these too.
deploy                   Production compose file and the reverse-proxy notes.
docs/specs               Feature specifications.
.claude/skills           The skill library. Read it before changing anything.
```

## Working on it

**Read `.claude/skills/` first — it is required, not optional.** Every feature here
was built from a committed specification under `docs/specs/`: `feature-start`
produces one and gets it accepted before any code is written, and
`feature-implement` builds a single phase per run and stops at its commit. None of
that is inferable from the code, and work that skips it cannot be reviewed against
anything. [The specs index](specs/INDEX.md) says what is built and what is specified.
[The project roadmap](ROADMAP.md) records what comes next.

**The contract comes first.** `api/openapi.yaml` is edited *before* the handler
that serves a new shape, never after. Then:

```bash
make generate            # regenerates the gin interfaces and the sqlc queries
```

Without `make`:

```bash
go tool oapi-codegen -config api/oapi-codegen.yaml api/openapi.yaml
go tool sqlc generate
```

A handler that no longer matches the contract fails to compile, which is the
point. The frontend regenerates its TypeScript from the same file.

**Schema changes are migrations.** Add a numbered `.up.sql`/`.down.sql` pair in
`migrations/` and run `make migrate-up`. sqlc reads those files to derive its
types, so there is no second schema definition to keep in step.

**Before pushing:**

```bash
make check               # generate, vet, test, build
```

Without `make`, run the generators above, then `go vet ./...`, `go test ./...`
and `go build ./...`. Set `PG_TEST_DSN` to run the query tests against a real
PostgreSQL; otherwise they skip. [Setup](setup.md) covers the development
database and running the API.

## Decisions worth knowing

- **UUID primary keys** throughout, so public URLs leak no record counts.
- **Cookie-based sessions.** The JWT is delivered in an httpOnly, SameSite
  cookie rather than a bearer token, so the Next.js frontend can authenticate
  during server rendering. Unsafe methods therefore also require a CSRF header.
- **No ORM.** sqlc generates typed methods from hand-written SQL.
- **`renter` and `rentee` are internal words only.** The product says "Housing
  Available" and "Looking for Housing"; neither internal term may appear in a
  path, a field name, or a response body.
