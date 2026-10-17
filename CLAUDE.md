# WaterlooStar API — working context

Backend for WaterlooStar, a housing marketplace and community forum for
University of Waterloo students. Go + gin, PostgreSQL via pgx, typed queries
from sqlc, spec-first OpenAPI.

The frontend is a **separate repository**, checked out beside this one as
`../WaterlooStar-Frontend`.

---

## Required before anything else: read the skill library

**Reading the skill library in `.claude/skills/` is a requirement for picking up
this project, not a suggestion.** Every feature here was built through it, and the
workflow is not inferable from the code: nothing in the repository tells you that
a specification is written and accepted before implementation, that a phase is one
commit, or where a phase doc has to end up. Work that skips it produces changes
that cannot be reviewed against anything and phases nobody can pick up after you.

**This project works from committed specifications, not from ad-hoc changes.**
Read these before touching anything:

1. `.claude/skills/feature-start/SKILL.md` — how a feature begins. Produces an
   agreed spec under `docs/specs/active/<slug>/`, committed on a feature branch,
   *before any code is written*.
2. `.claude/skills/feature-implement/SKILL.md` — how a phase is built. **One
   phase per execution.** Stops at the phase commit.

Also read whichever of these the task touches:

- `.claude/skills/implementation-breakdown/SKILL.md` — splitting work into phases
- `.claude/skills/release/SKILL.md` — release process

**Every pull request leaves the documentation current — this file above all.**
Another agent must be able to pick the work up from the repository alone, with no
access to the conversation that produced it. Before opening or updating any pull
request, spec work or not, update:
- this file: what is built, what is in flight (branch, pull request), what comes next,
  the developer's decisions with their dates, and the gotchas learned;
- `README.md` and its linked guides under `docs/`, `docs/ROADMAP.md` among them;
- `docs/specs/INDEX.md`;
- anything else the change made stale.

After a merge, the next piece of work starts by recording it here. The rule is in
`feature-implement` (Step 4, item 4) and in every phase doc's checklist.

**Every pull request carries a description of what it changes.** A title alone is
not one. The description says what changed and why, grouped by area; the commits it
contains; how it was verified; and what it leaves open. When more commits land on the
branch, edit the description to cover them. `feature-implement` ("The pull request
description") has the full rule.

The commit and pull-request conventions come from the skill library too:
conventional commits as `<type>(<slug>): <description>`, branches as
`<type>/<slug>`. The spec commit that opens a feature takes **no trailer lines** —
`feature-start` says so explicitly, and it is the one exception to how every other
commit here is signed.

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

- Never work on `main`. Branch as `<type>/<slug>` — the type is the kind of change,
  matching the commit's, so a spec branch is `feature/…` but a test-only phase is
  `test/…` and a fix is `fix/…`.
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
| 8 | Rate limiting on the auth and write endpoints, keyed on identity |
| 9 | Admin identity: `cmd/admin`, the `admin_actions` ledger, read-only `/admin` surface |
| 10 | Account management: suspend, reinstate, verify by hand, change role |
| 11 | Post moderation: every post in any state, takedown and restore |
| 12 | Listing terms: shorter stays, bill estimate and all-in cost, window ranking, monthly availability |
| 13 | Resend sender: verification and reset emails delivered through Resend; the log without a key |
| 14 | A new verification link: `POST /auth/verify/resend`, rate-limited, the same answer for any address |
| 15 | Conversations and messages: message a listing's owner, the inbox, unread counts, closing (on `feature/messaging`, not yet merged) |
| 16 | Contact privacy: the exact address off public reads and search, `cross_streets`, the owner's address share (on `feature/messaging-privacy`, not yet merged) |

Fifty-one endpoints exist, fourteen of them admin-only; nine are messaging
endpoints not yet on `main`: eight from Phase 15 and Phase 16's
`POST /conversations/{id}/share-address`. Read
[API surface](docs/api.md) for student endpoints, messages and rate limits, and
[Admin](docs/admin.md) for admin endpoints and `cmd/admin` usage.

Phases 5–14 are merged into `main` (pull requests #6–#13). Email Delivery is
complete; its [spec](docs/specs/implemented/email-delivery/index.md) is implemented.
The roadmap and README split merged in #14. The documentation-current rule and
the `.gitattributes` that keeps `go.mod`/`go.sum` on LF merged in #15
(2026-10-15). The pull-request-description rule merged in #16.

**In flight:** `feature/messaging` — the
[Messaging with Contact Privacy spec](docs/specs/active/messaging/index.md)
(Phases 15–18) is accepted (2026-10-16), and Phase 15, conversations and
messages, is complete on the branch: migration 11, eight `messages`-tag
endpoints, two new rate limits. It is open as
[pull request #17](https://github.com/20age1million/WaterlooStar-Backend/pull/17).
Each later phase is a stacked pull request (decided by Claude under the
developer's delegation, 2026-10-16), with the phase before as its base.

Phase 16, contact privacy and address sharing, is complete on
`feature/messaging-privacy` (branched from `feature/messaging`): migration 12,
`cross_streets`, the exact address off every public read and out of search,
and `POST /conversations/{id}/share-address` with its own rate limit. Its pull
request is still to be opened, **with `feature/messaging` as its base**.
**Merge order: the frontend's `fix/public-address` (frontend pull request #19)
must merge to frontend `main` and deploy before Phase 16 reaches `main`.**
Phase 16 makes `address_line` absent from public reads; a frontend that still
reads it shows "undefined" on cards. Drone deploys each repository from `main`
on its own, so nothing else enforces this order. After it merges, the frontend
regenerates its types (`Listing.address_line` optional; `cross_streets`;
`Conversation.place`, `address_shared_at`, `can_share_address`), adds a
cross-streets field to the listing form and shows it where it shows the
neighbourhood, and its messaging phase adds the owner's "Share exact address"
action with a confirmation; the full list is the spec's "Frontend impact".

Next is Phase 17 (offers open conversations), stacked on
`feature/messaging-privacy`, then 18 (email notifications).

**Email goes through Resend**, called with `net/http` (no SDK). Clerk's
transactional endpoint was chosen first (2026-09-25) and answered 404 with both a
development and a production key, so the developer switched to Resend on
2026-10-11. Clerk was never used for sign-in: do not run Clerk's "Add Clerk
Authentication" prompt (`clerk init`, `@clerk/nextjs`) in either repository.

### What comes next

Read [the project roadmap](docs/ROADMAP.md) before choosing or specifying the
next feature. On 2026-10-14 the developer moved messaging with contact privacy
first, ahead of saves, views and the question thread, replacing the order agreed
on 2026-09-27. It starts in parallel with the frontend Admin Portal and overhaul;
answering an offer ships with it, and since 2026-10-16 promotion waits on it.
The messaging spec is in `docs/specs/active/messaging/`. The frontend's new
verification-link form has
already shipped. Real email needs `RESEND_API_KEY` in the host env file; until
then an admin can verify accounts by hand.

### Developer decisions — 2026-10-14

[The project roadmap](docs/ROADMAP.md) is authoritative for the full plan:

- The MVP is finding and posting housing. Target: before Winter 2027 (January);
  cut scope rather than move the date. The community forum is outside this
  roadmap. (The launch gate was revised on 2026-10-16; see below.)
- Address privacy is settled: approximate location publicly, exact address
  later, designed with messaging (how it is revealed: 2026-10-16, below).
  Implemented in messaging Phase 16: public reads show `neighbourhood` and
  `cross_streets`; the exact address is the owner's to share in one
  conversation.
- Create the first admin on the host with `cmd/admin promote`; existing admins
  grant every later admin through the portal's `POST /admin/users/{id}/role`.
  Both backend paths already exist.
- Frontend order: Admin Portal (new spec; lost `feature/admin-portal` branch;
  Phases 13–15), UI/UX Overhaul from Phase 16, a minimal messaging phase before
  promotion (2026-10-16), photos, question thread, poster profiles, saved
  searches, real maps. The
  portal guard returns 404 to non-admins. Phase 16 removes fake data and header
  links to absent features; mobile layout ends the overhaul. Small UI fixes
  happen any time.
- The signed-in home is variant A, “Desk”: waiting offers, the student's places
  and requests with status, then new places for their dates and students
  looking. Messages, questions, saved places and saved searches use labelled
  skeletons until they exist. Frontend prototype: local branch
  `t3/signed-in-home-prototypes`, commit `0761309`; build it in Phase 16.
- Backend order: messaging with contact privacy now, alongside the frontend
  portal and overhaul; then saves, views and questions. This supersedes the
  2026-09-27 order.
- The developer chooses image hosting and map/geocoding providers when their
  features start. Each repo keeps its own specs and phase numbering; only
  `api/openapi.yaml` coordinates them.

### Developer decisions — 2026-10-16

- **Messaging before launch.** Promotion waits on the frontend Admin Portal,
  frontend Phase 16, this repository's messaging with contact privacy, and a
  minimal frontend messaging phase. There is no interim contact path: no phone
  number or email is published. The listing's "phone bar" is the frontend's
  pinned mobile bar and holds no phone.
- **The owner shares the exact address by hand.** An explicit “Share exact
  address” action in the conversation, by the listing's owner, whenever they
  choose. Nothing else reveals it — not a reply, not opening a conversation,
  not an offer.
- **Only the exact address is revealed.** No email, no phone, ever.
- **The rest of messaging was delegated to Claude.** The developer delegated
  the spec's open questions (Q-1 to Q-11) to Claude the same day. Each was
  decided by Claude under the developer's delegation, 2026-10-16, taking the
  drafted recommendation: no accept step; either side can close, no blocking;
  suspension and takedown make a conversation read-only and withhold a shared
  address without un-sharing it; every conversation is about a listing; admins
  never read message bodies in this feature; messages are append-only and
  kept; the public location is `neighbourhood` plus a new `cross_streets`; no
  un-share; sharing is per conversation; an offer opens or joins a
  conversation and replying is the answer; email on conversation opened,
  address shared and first unread reply, with an opt-out and caps. The
  frontend messaging phase goes straight after frontend Phase 16, under the
  same delegation. The spec's `index.md`, Decisions, has the table.
  During Phase 15, also decided by Claude under the developer's delegation:
  a listing its owner paused or archived does not stop a conversation; only a
  moderator's takedown or a suspended counterpart makes it read-only (the
  phase doc's Implementation Notes say why).
  During Phase 16, also decided by Claude under the developer's delegation,
  2026-10-16, taking the most conservative reading each time: sharing is
  refused (409) while the enquirer is suspended and while the listing is
  paused, archived or a draft, not only when it is taken down; a shared
  address stays in a closed conversation (closing un-shares nothing); and
  both participants see `place`, the owner included. Sharing also got a rate
  limit (20 an hour) that the phase doc had not planned, at the implementation
  brief's request.

---

## Rules that bind

- **The exact address is never on a public read, and only the owner reveals
  it.** `address_line` and coordinates come only from owner reads, admin reads
  and a conversation whose owner pressed share (`address_shared_at`);
  `toListing`'s `exact` flag is set explicitly at every call site. The search
  index does not contain the address. No email or phone is ever revealed.
  `contact_privacy_test.go` decodes every public read as raw JSON and fails
  on an `address_line` key, a non-null coordinate, or any email or phone
  field; `TestOnlyTheShareQuerySetsAddressSharedAt` fails if any query but
  `ShareConversationAddress` writes `address_shared_at`.

- **The API never invents a bill figure.** `bills_estimate_cents` is the owner's
  number or null; `all_in_cents` is null when it is. Filters and sorts on all-in
  cost treat null as unknown, never as zero.

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
  a one-line command; [Setup](docs/setup.md#the-api-from-source) and
  [Development](docs/development.md#working-on-it) list the equivalents.
- **`go.mod` and `go.sum` are pinned to LF in `.gitattributes`.** The go tool
  always writes LF, and with `core.autocrlf=true` on the development machine
  every `go mod tidy` otherwise left both files showing as modified with no
  change in content.
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
- **The rate limiter cannot see client IP addresses.** This service has no public
  URL and the Next.js server is its only client, so every request arrives from one
  address. Limits are keyed on identity — the email, the token, the user — and
  per-IP limiting lives at the reverse proxy (`deploy/RATE-LIMITING.md`). A
  per-IP limiter added here would throttle the entire site as one client.
- **Only a *failed* login is charged to the limiter.** `RateLimit` peeks on the
  login route and `chargeFailedLogin` spends afterwards. Make login spend up
  front and the Playwright suite — which signs in repeatedly on purpose — starts
  failing, along with anyone who signs in several times a day.
- **`internal/ratelimit` runs on an injected clock, and no test may sleep.** A
  suite that waits for real seconds flakes in CI and then gets deleted. The
  race detector needs a C toolchain the development machine does not have, so CI
  runs `-race` over that package instead.
- **The limited routes are an allowlist** in `internal/httpapi/ratelimit_mw.go`,
  matched on `c.FullPath()`. Its failure mode is silence: a path that matches no
  route limits nothing and nothing complains, which is why
  `TestEveryLimitedRouteExists` checks them against the registered routes.

- **`/admin` must look unrouted to non-admins, down to the headers.**
  `requireAdmin` writes the refusal itself with `apierror.NotFound`, the call the
  router's `NoRoute` makes, and the handler returns `unrouted{}`. The generated 404
  response types encode differently (bare `application/json`, trailing newline),
  so using them leaks which paths exist. A new admin operation needs a `Visit`
  method on `unrouted`.
- **`UpdateListing` uses COALESCE, so a PATCH cannot set a column to NULL** —
  except `min_stay_months` and `bills_estimate_cents`, which carry explicit
  `set_min_stay`/`set_bills_estimate` flags so they can be cleared. `deposit_cents`
  and `distance_m` still cannot be cleared once set: a known gap, found in Phase
  12, not yet fixed.
- **Turning `shorter_stays` off clears the minimum.** `minStayChange` does it, so
  a client need not send `min_stay_months: null` as well.
- **Every admin change goes through `db.Audited`**, which runs the change and its
  `admin_actions` row in one transaction. `db.ChangeRole` is the role change, with
  the last-admin guard, for both `cmd/admin` and the portal.
- **Takedown is not `status`.** `removed_at` is the moderator's; `status` is the
  owner's. Sharing a column would let an owner republish over a moderator and
  lose what the status was. `ownedListing`/`ownedRequest` refuse a removed post
  with a 409, so every owner write path inherits the rule.
- **Conversation reads are the participants' own, so they do not hide.** Like
  `/me/...`, `GetConversationForParticipant` and `ListConversationsForUser` do
  not filter out a suspended counterpart or a taken-down listing; they return
  `counterpart_suspended`, `listing_removed` and `listing_available`, and
  `sendRefusal` in `internal/httpapi/messages.go` turns those into a read-only
  conversation (`can_send: false`, 409 on send). Hiding it would erase what the
  other student might be reporting. Starting one still goes through
  `GetPublishedListing`, so the public filters apply there. A paused or
  archived listing does *not* stop a conversation: only a takedown does.
- **sqlc types a `LEFT JOIN LATERAL (subquery)` column as non-null.** The first
  draft of the conversation reads did, and a conversation with no messages
  would have failed to scan its NULL last message into a `string`. Join the
  table itself (`LEFT JOIN messages lm ON lm.id = (SELECT ... LIMIT 1)`) and
  sqlc makes the columns pointers. Check the generated row type whenever a
  query left-joins anything other than a table.
- **sqlc types a `CASE ... END` column as `interface{}`.** Phase 16's reveal
  condition was first written as a `CASE` per column and would have reached
  the handlers untyped. It is a second `LEFT JOIN listings pl ON ... AND
  c.address_shared_at IS NOT NULL AND l.removed_at IS NULL AND
  o.suspended_at IS NULL` instead, whose columns sqlc makes pointers. Check
  the generated row type after writing any computed column.
- **`gen.Listing.AddressLine` is a pointer with `omitempty`.** Leaving it nil
  is what makes the key absent, not null, on public reads. A new field on
  `Listing` that belongs to the exact address must be set only inside
  `toListing`'s `if exact` block, and the guard in `contact_privacy_test.go`
  extended to it.
- **The search column decides what `q` can find, on the admin surface too.**
  Migration 12 rebuilt `listings.search` without `address_line`; the admin
  listing search reads the same column, so an admin cannot search by street
  either. A field added to the index can be confirmed by probing even if it
  is hidden from every response.
- **The fake's conversations run on a strictly increasing clock** (`tick` in
  `internal/httpapi/fake_querier_conversations_test.go`). Unread counts compare
  a message's time with a read marker, and with the wall clock two calls in a
  row can read the same instant, making an unread message look read. Use
  `f.tick()` for any new timestamp the unread rule compares.
- **The admin post lists are the one exception to the published-only rule.**
  `ListListingsForAdmin` and `ListRequestsForAdmin` read every row on purpose,
  behind the `/admin` guard. Every other read of a post filters on `status`,
  `removed_at` and the owner's `suspended_at`.
- **Every public read must filter out suspended accounts, in the SQL.** Listings,
  requests and offers each carry `AND u.suspended_at IS NULL` on the owner, and
  the offer counts inside the request reads join the offering owner for the same
  reason. A new public query without it shows a suspended account's posts. The
  owner's own `/me/...` reads deliberately do not filter.
- **Login says "suspended" only after the password matches.** Checking earlier
  would tell anyone guessing that the address exists and is suspended.
- **`go run ./cmd/migrate down` rolls back every migration**, and ignores a count
  after it. One step is `down-one`. On the development database that means
  re-running `migrate up` and `cmd/seed`.
- **sqlc's nullable-uuid override names the type `UUID`, not `uuid.UUID`**,
  because `import` already supplies the package. The doubled form went unnoticed
  until migration 8 added the first nullable uuid column.
- **The `waterloostar-api` container holds port 8080.** With the Docker stack up,
  `go run ./cmd/api` fails to bind, says so once in its own output, and every
  request goes to the container instead — running whatever code it was built
  from. Phase 13's first real send was lost that way. Before a hand check, make
  sure the process answering `:8080` is the one you think: `docker compose stop
  api` to run from source.
- **The container reads `.env` when it is created, not when it starts.** A key
  added to `.env` afterwards is ignored until `docker compose up -d api`
  recreates it; startup logs which email sender it chose.

---

## Running it

Read [Setup](docs/setup.md) when starting Docker, running the API from source or
configuring email. Read [Development](docs/development.md) when regenerating
code or running checks; it includes the commands without `make` and the query
test database requirement. The rules and gotchas above apply to every change.

---

## Deliberately deferred

Read [the roadmap's provider decision](docs/ROADMAP.md#providers-still-to-be-chosen)
before starting photos or maps. Image hosting and map/geocoding providers are
the developer's calls at that time.
