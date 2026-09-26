# WaterlooStar API — working context

Backend for WaterlooStar, a housing marketplace and community forum for
University of Waterloo students. Go + gin, PostgreSQL via pgx, typed queries
from sqlc, spec-first OpenAPI.

The frontend is a **separate repository**, checked out beside this one as
`../WaterlooStar-Frontend`.

---

## Before doing anything: read the skills

**This project works from committed specifications, not from ad-hoc changes.**
The workflow is not optional and it is not obvious from the code, so read it
before touching anything:

1. `.claude/skills/feature-start/SKILL.md` — how a feature begins. Produces an
   agreed spec under `docs/specs/active/<slug>/`, committed on a feature branch,
   *before any code is written*.
2. `.claude/skills/feature-implement/SKILL.md` — how a phase is built. **One
   phase per execution.** Stops at the phase commit.

Also read whichever of these the task touches:

- `.claude/skills/implementation-breakdown/SKILL.md` — splitting work into phases
- `.claude/skills/release/SKILL.md` — release process

**Two skills in that folder do not apply here.** `az-pr-create` and
`azure-devops-project-creator` came with the toolkit from another organisation's
Azure DevOps setup. This project is on GitHub. Ignore them unless it moves.

Invoke a skill with the `Skill` tool by name, e.g. `feature-implement`.

### Backend and frontend are separate services

**Each repository has its own phases and its own specs.** They are not two halves
of one feature: the backend's Phase 4 and the frontend's Phase 4 are independent
pieces of work, specified separately, numbered per repository, and landing on
their own schedules.

What ties them together is `api/openapi.yaml` and nothing else. The backend
publishes a contract; the frontend consumes it. A contract change is coordinated
through that file, not through a shared spec.

This replaces the earlier convention, where one feature carried mirrored specs in
both repositories with phases aligned one-to-one. Platform Foundation was built
that way; everything from Phase 4 onward is not.

### The short version of the workflow

- Never work on `main`. Branch as `feature/<slug>`.
- A spec is written and **accepted by the developer** before implementation.
  Acceptance is explicit — silence is not agreement.
- One phase at a time. Each phase ends with: checklist ticked, Implementation
  Notes filled in, phase doc moved `active/` → `implemented/`, `index.md`
  updated, one commit.
- Divergences from the spec are recorded in the phase's Implementation Notes,
  never made silently.

---

## Where things stand

**Remote:** `origin` → `github.com/20age1million/WaterlooStar-Backend`. The rework
was grafted onto `main`, which now holds it; branch as `<type>/<slug>` and open a
PR into `main`. The early `carl/…` branches are history.

**Deployed.** Drone builds on a push to `main`, pushes the image to GHCR and runs
`deploy/docker-compose.yml` on the host. The API has no public URL: it sits on a
private Docker network and the Next.js frontend, serving waterloostar.com, is the
only thing that calls it.

| Phase | Delivered |
|---|---|
| 0 | gin service, golang-migrate harness, sqlc + oapi-codegen chain, `/healthz` |
| 1 | Registration gated on `uwaterloo.ca`, verification, login, sessions, password reset |
| 2 | Structured `listings` schema, seed data, `GET /listings` and `GET /listings/{id}` |
| 3 | Discovery: server-side search, filters, sorts and pagination |
| 4 | Listing write path: post, edit, status lifecycle, `/me/listings` |
| 5 | Query tests against a real PostgreSQL, plus a guard for untested queries |
| 6 | Housing requests: table, lifecycle, browse with filters, write path |
| 7 | Offers: an owner answers a request with one of their own listings |

Twenty-six endpoints are live; `README.md` has the table.

### What comes next

Nothing is specified. Candidates, in the order I would take them:

1. **Email delivery.** Verification and reset links only reach the log, so on the
   live site nobody can finish signing up. The developer has chosen Clerk's
   transactional endpoint; `internal/email` already has the `Sender` seam.
2. **Rate limiting.** There is none anywhere — login accepts unlimited guesses
   against guessable `uwaterloo.ca` addresses, and password reset has no cap.
3. Saves, views and the question thread → messaging with contact privacy →
   matching and real maps.

---

## Rules that bind

- **The contract comes first.** `api/openapi.yaml` is authoritative and is edited
  *before* the handler that serves a new shape. Then regenerate. A handler that
  drifts fails to compile — that is the point.
- **`renter` and `rentee` are internal words only.** The product says "Housing
  Available" and "Looking for Housing". Neither internal term may appear in a
  path, a field name, or a response body.
- **Schema changes are migrations.** Numbered `.up.sql`/`.down.sql` pairs in
  `migrations/`. sqlc reads those files for its types, so there is no second
  schema definition to keep in step.
- **Every error uses the one envelope** in `internal/apierror`. Including errors
  from generated request binding — see the gotcha below.
- **Only published listings are served**, and that filter lives in the SQL so a
  handler cannot forget it. An unpublished listing 404s, never 403s.
- Generated code (`internal/db/sqlcgen/`, `internal/httpapi/gen/`) is never
  edited by hand.

---

## Gotchas that cost real time

These were all found the hard way and will silently regress if undone.

- **`r.ContextWithFallback = true` in `internal/httpapi/router.go` is mandatory.**
  The generated strict handlers receive the `*gin.Context` as their
  `context.Context`, and gin does *not* fall through to the request's context
  without it. Remove it and every authenticated request looks anonymous.
- **Two separate error hooks must both be replaced.** The strict handler's
  `RequestErrorHandlerFunc` covers the request *body*;
  `GinServerOptions.ErrorHandler` covers *path and query parameters*. Miss either
  and that class of failure escapes the documented envelope. oapi-codegen's
  defaults also leak the raw Go error into 500 responses.
- **The refresh cookie is scoped to `/auth`, not `/auth/refresh`.** Scoped to the
  refresh path alone, the browser never sends it to `/auth/logout`, so logout
  cannot revoke the token it exists to revoke.
- **`make` is not installed on the current development machine.** Every target is
  a one-line command; the README lists the equivalents.
- **Docker maps PostgreSQL to host port 5433**, because a local PostgreSQL
  already occupies 5432 on that machine.
- **Every query needs a test in `internal/db`.** `TestEveryQueryIsExercised`
  reflects over the generated `Querier` and fails, naming them, when one has no
  test — because Phase 4 shipped a query PostgreSQL refused to type and every
  handler test passed anyway. Set `PG_TEST_DSN` to run them; unset, they skip.
- **Referring to one `sqlc.arg` as both `varchar` and `::text` is SQLSTATE
  42P08.** Cast both uses. This is the bug above.
- **A request's filters invert a listing's.** A budget is a ceiling, so
  `budget_min` means "who can afford this rent"; a radius is how far out they
  would go, so `distance_min` means "who would accept a place this far out". A
  request with *no* stated radius passes every distance filter, where a listing
  with no recorded distance is excluded by one.
- **Offer counts are computed, never stored.** An offer stops counting when its
  listing is taken down — a different table — so a stored counter would drift.
  The column added in migration 6 was dropped in migration 7 for that reason.

---

## Running it

```bash
cp .env.example .env
docker compose up -d       # PostgreSQL 18 on :5433
go run ./cmd/migrate up
go run ./cmd/seed          # six development listings
go run ./cmd/api           # :8080
```

Checks: `go build ./...`, `go vet ./...`, `go test ./...`.
Regenerate: `go tool oapi-codegen -config api/oapi-codegen.yaml api/openapi.yaml`
and `go tool sqlc generate`.

No email provider is configured — verification and reset links are printed to
the API log. That is how you complete a signup in development.

---

## Deliberately deferred

Do not quietly decide these; they are the developer's calls.

- **Whether a listing's street address stays public.** Public today.
  `address_line` is separate from `neighbourhood` so a private field can be added
  without moving data.
- **Email, image-hosting and map/geocoding providers.** Placeholders behind
  interfaces.
