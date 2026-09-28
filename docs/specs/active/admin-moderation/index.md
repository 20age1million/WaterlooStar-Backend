# Admin and Moderation — API Phases 9–11

**Status:** In Progress
**Branch:** `feature/admin-moderation`
**Ticket / Work Item:** N/A
**Owner(s):** WaterlooStar backend
**Date:** 2026-09-26

---

## Purpose / Goal

Give the site an operator. An admin can see every account and every post,
suspend a user, take a post down, and undo either — with each action recorded
against the person who took it and the reason they gave.

---

## Problem Statement / Motivation

The API has no operator at all. Twenty-six endpoints serve students and owners;
nothing serves whoever runs the site. Today a spam listing, a doxxed address or
an abusive account can only be dealt with by opening `psql` on the production
host and writing an `UPDATE` by hand — unlogged, unreviewable, and one typo away
from a `WHERE`-less statement.

The vocabulary for this already exists and does nothing. `users.role` carries a
`CHECK (role IN ('user', 'moderator', 'admin'))` from migration 2, and
`auth.Principal.IsAdmin()` has been in `internal/auth/token.go` since Phase 1
without a single caller. Every account on the live site is a `user`, and no code
path can make one anything else.

Why now: the site is live and about to accept real sign-ups once email delivery
lands. The first bad post arrives some time after the first real user, and the
tooling to deal with it takes longer to build than it takes to arrive. There is
also an immediate second use: **manual verification**. Verification links only
reach the API log, so an admin who can flip `verified` can let a real student in
before the email provider exists.

---

## Proposed Solution / Design

Two axes that do not touch each other, plus a ledger.

**Suspension** is a property of an account. It is a timestamp, not a flag or a
deletion, in the same shape as `request_offers.withdrawn_at`: reversible,
self-dating, and impossible to confuse with never having happened.

**Takedown** is a property of a post, and deliberately *not* its `status`.
`listings.status` and `housing_requests.status` belong to the owner — they are
what the Phase 4 and Phase 6 write paths manipulate. An admin who set
`status = 'archived'` would be overruled the moment the owner republished. So a
takedown is its own column set, and the public read filter checks both.

**The ledger** is `admin_actions`, append-only. Every state change an admin
makes writes one row: who, what, to whom, why, when. The query surface has no
update and no delete, so there is no code path that can rewrite history.

Nothing is ever erased. Every admin power in this feature has an inverse.

### Key Components

- **`migrations/000008_admin`**: `users.suspended_at/suspended_by/suspend_reason`
  and the `admin_actions` table.
- **`migrations/000009_post_moderation`**: `removed_at/removed_by/removed_reason`
  on `listings` and `housing_requests`, and the partial indexes the public reads
  need once they filter on it.
- **`cmd/admin`**: the bootstrap. `promote`, `demote`, `list` — the only way the
  first admin can exist, since no HTTP path grants the role to someone who does
  not already hold it.
- **`internal/httpapi/admin.go`, `admin_users.go`, `admin_posts.go`**: the
  handlers, under `/admin`.
- **`internal/httpapi/admin_guard.go`**: the role gate, and the 404 it returns to
  everyone else.
- **`internal/db/queries/admin.sql`**: the admin read surface and the ledger
  writes.

### Data / Control Flow

- An operator runs `cmd/admin promote <email>` once on the host. That account's
  next login mints a JWT carrying `role: "admin"`, which is all the API needs.
- Every `/admin` route resolves the principal, requires `role = 'admin'`, and
  answers **404** to anyone else — not 403. The rule already applied to someone
  else's listing; here it also means an ordinary user probing `/admin/users`
  cannot tell the surface exists.
- An admin reads accounts with search and filters, and posts across *every*
  status — the one caller allowed past the `status = 'published'` filter that has
  lived in the SQL since Phase 2.
- A state change requires a reason. The handler writes the change and the
  `admin_actions` row in **one transaction**: an action that is not logged did
  not happen.
- Suspending an account revokes its refresh tokens, so the session dies at the
  next refresh rather than at the end of the access token's life.
- A suspended user's posts stop appearing in public results, and their offers
  stop appearing on requests, through the same SQL-level filter that hides an
  unpublished listing. One suspension, no sweep over their content.
- Reinstating restores exactly what suspension hid, because suspension changed
  nothing else. Restoring a post returns it to whatever status its owner had set.

---

## Layers / Areas Affected

| Layer / Area | Change |
|---|---|
| Database schema | `users` suspension columns; `admin_actions`; moderation columns on `listings` and `housing_requests` |
| Migrations | Two pairs |
| DB access layer | Admin account and post reads, suspension, role change, takedown, ledger insert and list; public read queries gain the moderation filter |
| Service layer | Admin authorisation, reason validation, self-action guards, transactional log-with-change |
| API handlers | `/admin` surface — overview, accounts, posts, ledger (roughly 14 operations across three phases) |
| DTOs / contracts | `api/openapi.yaml`: admin schemas and paths, tagged so the public surface stays readable |
| Auth | The `admin` role becomes load-bearing; suspension blocks login and refresh; role change revokes sessions |
| CLI | New `cmd/admin` alongside `cmd/migrate` and `cmd/seed` |
| Tests | Query tests for every new query; handler tests for the guard, each action and the ledger; regression tests proving suspended and removed content leaves public reads |
| Deployment / Infra | None — no new services, ports or images |
| Configuration | **None.** No new environment variables or secrets; the role lives in the database |
| Operational | The operator runbook gains one command. `admin_actions` grows unbounded and is never pruned by this feature |

---

## Phase Tracker

| Phase | Title | Status | Location |
|---|---|---|---|
| [9 — Admin identity and the ledger](../../implemented/admin-moderation/phase-9-admin-identity.md) | The role becomes real, the audit table exists, and an admin can read every account | Complete | `docs/specs/implemented/admin-moderation/` |
| [10 — Account management](../../implemented/admin-moderation/phase-10-account-management.md) | Suspend, reinstate, verify by hand, change role — each logged, each reversible | Complete | `docs/specs/implemented/admin-moderation/` |
| [11 — Post moderation](./phase-11-post-moderation.md) | Read every post regardless of status; take one down and restore it | Ready | `docs/specs/active/admin-moderation/` |

**Phase 8 — [Rate Limiting](../../implemented/rate-limiting/index.md) — was a
prerequisite of this whole feature**, not a phase of it. It stayed a separate
feature because it protects every student's login as much as an admin's, and it
shipped on its own and first: an admin password is the most valuable secret on the
site, and login accepted unlimited guesses until it landed. **It is now complete**,
so this feature's prerequisite is met.

---

## Trade-offs / Alternatives Considered

- **Reversible actions only, no deletes** — chosen by the developer. Every user
  foreign key in the schema is `ON DELETE CASCADE`: deleting one account erases
  its listings, its requests, and the offers other students are relying on, with
  nothing left to explain the absence. Moderation mistakes happen at speed and
  under pressure; all of these are undoable.
- **A CLI for the first admin, the portal for the rest** — chosen by the
  developer. An `ADMIN_EMAILS` environment variable was rejected: the `role`
  column would then be a lie, changing the admin set would need a redeploy, and
  the grant would appear in no ledger.
- **Admins keep the `uwaterloo.ca` gate** — chosen by the developer. The schema
  constraint from migration 2 stands, so an admin is a student account that was
  promoted. No exemption, no second login path, no staff table.
- **Rate limiting first, as its own feature** — decided by the developer. Folding
  it in as a phase here would have tied a site-wide protection to an operator
  feature and delayed it behind three phases of admin work.
- **Takedown is separate from `status`** — rather than reusing `archived`. An
  owner's write path owns `status`; sharing the column would let the owner
  republish over a moderator's decision, and would lose what the status was
  before.
- **404 for non-admins, not 403** — consistent with someone else's listing, and
  it keeps the surface undiscoverable by probing.
- **`admin_actions` as its own table** rather than a generic event log. A generic
  log invites everything and answers nothing; this one has a fixed question to
  answer — who did this to my account, and why.
- **Suspension hides content through the read queries** rather than by rewriting
  the owner's posts. Rewriting would destroy the owner's own statuses and could
  not be undone precisely.
- **No moderator role behaviour yet.** The value exists in the `CHECK` and stays
  unused; treating `moderator` as a reduced admin would mean designing a
  permission split nobody has asked for. Recorded here so it is a decision, not
  an oversight.

---

## Assumptions

- The `admin` claim in the access JWT is trustworthy for authorisation. It is
  signed and strictly decoded (Phase 5), and its fifteen-minute life bounds how
  long a demoted admin keeps acting as one — accepted rather than solved by
  checking the database on every admin request.
- Every public read path that must hide suspended and removed content is reachable
  from `internal/db/queries/`. Nothing constructs SQL outside that directory, so
  auditing the filter is auditing those files.
- Ledger volume is small — a human's clicks, not traffic — so `admin_actions`
  needs no partitioning or retention policy inside this feature.
- One or two people operate the site. Nothing here needs delegated permissions,
  approval flows or four-eyes review.

---

## Constraints

- **Phase 8 (rate limiting) ships before this feature.** Not a suggestion: the
  admin login is the highest-value target on the site. **Met** — Phase 8 is
  complete, and login now allows five failed attempts per address per 15 minutes.
- **This repository is specified on its own.** The admin portal is a separate
  feature in the frontend repository, consuming this through `api/openapi.yaml`.
- **No new runtime configuration.** No secrets, no environment variables — the
  deployed stack must not need a new value to gain an admin.
- **Every admin state change is logged in the same transaction as the change.**
  An unlogged action is a bug, not a shortcut.
- **An admin cannot suspend, demote or delete themselves**, and the last
  remaining admin cannot be demoted — losing the only admin account means
  another trip to `psql`.
- **A suspended account cannot log in, cannot refresh, and cannot be reached by a
  password reset** into a working session.
- The vocabulary rule stands: nothing named `renter` or `rentee`.
- `/admin` answers 404 to non-admins, never 403.
- Every query in `internal/db/queries/` needs a test that runs against a real
  PostgreSQL (`TestEveryQueryIsExercised` enforces this).
- The public read surface must not change shape. A suspended or removed post
  answers 404 exactly as an unpublished one does.

---

## Success Criteria / Definition of Done

- `cmd/admin promote <email>` makes an existing account an admin, and `list`
  shows who holds the role.
- An admin can search and page through every account, and see each one's role,
  verification, suspension and post counts.
- An admin can suspend and reinstate an account, verify one by hand, and change a
  role — each with a reason, each recorded in `admin_actions`.
- A suspended user cannot log in or refresh, and their listings, requests and
  offers disappear from public results without their own statuses changing.
- An admin can list every listing and request in any status, take one down with a
  reason, and restore it to the status its owner had.
- A taken-down post answers 404 publicly, and republishing it as its owner does
  not bring it back.
- Every `/admin` route answers 404 to an anonymous caller and to a signed-in
  non-admin.
- `admin_actions` has no update or delete query anywhere in the codebase.
- All phase checklists complete; `go test ./...` passes with and without a
  database available.

---

## References (internal only)

- The prerequisite, now shipped: `docs/specs/implemented/rate-limiting/index.md`
- Related specs: `docs/specs/implemented/platform-foundation/index.md` (roles,
  sessions, the 404 rule), `docs/specs/implemented/housing-requests/index.md`
  (the query test layer these phases depend on)
- The frontend's consuming feature: `../WaterlooStar-Frontend/docs/specs/active/admin-portal/index.md`
