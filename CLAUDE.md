# WaterlooStar API — working context

Backend for WaterlooStar, a housing marketplace and community forum for
University of Waterloo students. Go + gin, PostgreSQL via pgx, typed queries
from sqlc, spec-first OpenAPI.

The frontend is a **separate repository**, checked out beside this one as
`../Waterloostar-web`.

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
Azure DevOps setup. This repository has no remote at all. Ignore them unless the
project actually moves to Azure DevOps.

Invoke a skill with the `Skill` tool by name, e.g. `feature-implement`.

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

**Branch:** `feature/platform-foundation` · **No remote configured; nothing pushed.**

The **Platform Foundation** feature is complete — all three phases shipped and
their specs are under `docs/specs/implemented/platform-foundation/`. See
`docs/specs/INDEX.md`.

| Phase | Delivered |
|---|---|
| 0 | gin service, golang-migrate harness, sqlc + oapi-codegen chain, `/healthz` |
| 1 | Registration gated on `uwaterloo.ca`, verification, login, sessions, password reset |
| 2 | Structured `listings` schema, seed data, `GET /listings` and `GET /listings/{id}` |

Eleven endpoints are live; `README.md` has the table. `docs/specs/active/` is
empty — **the next feature starts with `feature-start`.**

### What comes next

Discovery: server-side search, filtering, sorting and pagination — what makes
the frontend's filter rail real. Query params, a Postgres full-text index, and
radius search. `SortKey`'s values in the frontend (`match`, `new`, `priceAsc`,
`priceDesc`, `distance`) are the intended parameter names.

After that, roughly: listing write path → requests ("Looking for Housing") →
saves/views/Q&A → messaging with contact privacy → matching and real maps.

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
