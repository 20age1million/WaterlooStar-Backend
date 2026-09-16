# Listing Write Path — API Phase 4

**Status:** Complete
**Branch:** `feature/listing-write-path`
**Ticket / Work Item:** N/A
**Owner(s):** WaterlooStar backend
**Date:** 2026-09-16

---

## Purpose / Goal

Let a verified student create, edit and manage their own listing — the write half
of Housing Available, and the first place the API accepts something a user typed.

---

## Problem Statement / Motivation

Every listing in the database was put there by the seed. The product's whole
premise is that students post their own rooms when they leave on co-op, and none
of that is possible: there is no create, no edit, and no way to take a post down
once the room is gone.

It is also the first endpoint set that writes user input, so it is where
ownership, validation and the published/paused/archived lifecycle have to become
real rather than columns nobody sets.

---

## Proposed Solution / Design

Five endpoints on top of the existing table. No schema change beyond what the
lifecycle needs, because Phase 2 already modelled every field.

Authorisation is two-layered and both layers matter:

- **Verified students only.** An unverified account can browse but cannot post —
  that is the trust proposition the whole product rests on.
- **Owners only, for their own listing.** Someone else's listing answers 404
  rather than 403, so the API never confirms that a listing exists to someone
  with no business knowing.

### Endpoints

| Method | Path | Who |
|---|---|---|
| `POST` | `/listings` | Verified student |
| `PATCH` | `/listings/{id}` | Owner |
| `DELETE` | `/listings/{id}` | Owner |
| `POST` | `/listings/{id}/status` | Owner |
| `GET` | `/me/listings` | Any signed-in user, their own |

`GET /me/listings` exists because the owner needs to see their drafts, paused and
archived posts, which the public endpoint deliberately hides.

### Lifecycle

```
draft ──publish──► published ──pause──► paused ──publish──► published
                        │                  │
                        └───archive────────┴──► archived (terminal)
```

Archived is terminal: a post that has been taken down stays down. Bringing one
back is a new listing, which is honest — the room, the term and the price will
have changed.

---

## Layers / Areas Affected

| Layer / Area | Change |
|---|---|
| Database schema | `published_at`; a partial index for the owner's own listings |
| Migrations | One pair |
| DB access layer | Create, update, soft-delete, status transition, list-by-owner |
| Service layer | Validation, ownership, transition rules |
| API handlers | Five endpoints |
| DTOs / contracts | Request bodies and the new paths |
| Auth | First use of `RequireVerified`; ownership checks |
| Tests | Validation, ownership, each transition, and the rejection paths |

---

## Trade-offs / Alternatives Considered

- **`PATCH` over `PUT`** — chosen. An edit form sends what changed; a `PUT`
  would make every omitted field a deletion.
- **A `status` endpoint over patching `status`** — chosen. Pausing is a different
  act from correcting the rent, the rules differ, and a transition can be
  rejected on grounds an edit cannot.
- **Soft delete over a row delete** — chosen. `DELETE` archives. A listing has
  questions and, later, saves and conversations hanging off it; removing the row
  would take those with it and rewrite other people's history.
- **404 over 403 for someone else's listing** — chosen, matching the read path.
  A 403 confirms the listing exists.
- **Photos deferred** — decided by the developer. No storage provider is chosen;
  `listing_photos` stays present and unpopulated, and the gallery stays a
  placeholder. Nothing here blocks adding upload later.

---

## Constraints

- **A listing may only be created by a verified student.** `RequireVerified` has
  existed since Phase 1 with no caller; this is its first.
- **The end date must fall after the start**, rent must be positive, and
  `bedroom_of` cannot exceed `bedrooms_total` — all already enforced by check
  constraints, and to be rejected with a readable message before reaching them.
- Only published listings appear in public results; the owner's own endpoint is
  the only way to see anything else.
- The vocabulary rule stands: nothing named `renter` or `rentee`.
- **This repository is specified on its own.** The frontend is a separate service
  and will consume this through `api/openapi.yaml`.

---

## Success Criteria / Definition of Done

- A verified student can create a listing and find it in public results.
- An unverified account is refused with 403; an anonymous one with 401.
- Editing changes only the fields sent.
- Pause removes a listing from public results; publish restores it; archive is
  terminal and cannot be undone.
- Another user's listing answers 404 for every write endpoint.
- Invalid input is rejected with field-level messages, not a constraint violation.
- `go build`, `go vet`, `go test` pass; generators idempotent.

---

## Implementation Steps

- [x] **Preparation**
  - [x] Confirm Discovery is complete and the database is seeded

- [x] **Schema**
  - [x] `migrations/000005_listing_lifecycle.*`: `published_at timestamptz`,
        and an index on `(owner_id, created_at DESC)` for the owner's list
  - [x] Verify: migrate up and down both succeed

- [x] **Queries**
  - [x] Insert, partial update, status transition, archive, list-by-owner,
        and an ownership lookup that does not leak existence
  - [x] Verify: `sqlc generate` produces compiling Go

- [x] **Contract**
  - [x] Add the five operations and their request bodies to `api/openapi.yaml`,
        with the security requirements stated per operation
  - [x] Verify: `oapi-codegen` regenerates and the interface compiles

- [x] **Handlers**
  - [x] Create: verified-only, validate, default to `published`
  - [x] Update: owner-only, partial, revalidate the result as a whole
  - [x] Status: owner-only, enforce the transition rules
  - [x] Delete: owner-only, archive rather than remove
  - [x] `GET /me/listings`: the owner's own, every status
  - [x] Verify: by hand against the running API

- [x] **Tests**
  - [x] Creation by a verified student; refusal for unverified and anonymous
  - [x] Validation: bad dates, non-positive rent, bedroom_of out of range
  - [x] Partial update leaves untouched fields alone
  - [x] Each transition, and the refusals: archived is terminal
  - [x] Another user's listing is 404 on every write path
  - [x] Verify: `go test ./...` passes

- [x] **Final verification**
  - [x] Build, vet, test, generators idempotent
  - [x] Move this file to `docs/specs/implemented/listing-write-path/`

---

## Implementation Notes

**Key files changed:**

- `migrations/000005_listing_lifecycle.*` — `published_at`, backfilled for
  already-published rows, and an owner index that is deliberately not partial on
  status, since the owner's own query must return drafts and archived posts.
- `internal/db/queries/listings.sql` — insert, partial update via COALESCE,
  status transition, owner list, and an ownership lookup.
- `internal/httpapi/listings_write.go` — the five handlers, the transition table
  and the shared `ownedListing` refusal.
- `internal/httpapi/listings_validate.go` — field-level validation and the
  conversions between the generated nullable types and the database's pointers.
- `internal/httpapi/listings_write_test.go` — 11 tests.
- `api/openapi.yaml` — five operations, `ListingInput` and `ListingUpdate`.

**Divergences from plan:** none. Photos remain deferred as agreed; the gallery
stays a placeholder and `listing_photos` is untouched.

**Decisions worth recording:**

- **`published_at` is set once and never moved.** Re-publishing after a pause
  does not make an old post look new in the feed.
- **Archived is terminal**, and the refusal says why: post again rather than
  revive, because the term and the rent will have moved on.
- **Setting the status a listing already has is idempotent**, not an error.
- **An edit is validated as the row it will become**, not as the patch in
  isolation — moving only the end date can still invert the term. Tested.
- **Every ownership failure is 404.** "Not yours" and "does not exist" are
  indistinguishable, because a 403 confirms the listing exists.

**A bug the unit tests could not catch, and what it means**

The first run passed all 11 handler tests and then failed against the real
database: `SetListingStatus` returned *inconsistent types deduced for parameter
$1* (SQLSTATE 42P08), because the query referred to the same parameter as
`varchar` in the `SET` and as `::text` in the `CASE`. Pausing silently did
nothing, and archived listings could be un-archived, because the transition check
was reading a status that had never changed.

The handler tests run against an in-memory fake, so **no test in this repository
can catch a malformed query.** That is a real gap, not a one-off: every `.sql`
file here is only exercised when someone runs the service by hand. The layers
table in Platform Foundation's spec named "query tests against a test database"
and that has never been built.

Recommend a database-backed test layer before the next feature that adds
queries. Out of scope here, and recorded rather than quietly carried.

**Verification run:**

- `go build`, `go vet`, `go test ./...` pass; generators idempotent.
- Migration up, down-one, up — clean.
- Against the real database, signed in as a seeded student: create → 201 and the
  listing appears in public results (6 → 7); `PATCH {price_cents}` changes only
  the rent and leaves title, address and bedrooms alone; pause → public total
  back to 6; republish → 7; archive → 200; un-archive → 400 with the terminal
  message.
- A second seeded account (`danielo`) attempting `PATCH`, `DELETE` and the status
  endpoint on another user's listing gets **404** on all three, and the owner's
  listing is unaltered.
- Seed data restored to six listings afterwards.
