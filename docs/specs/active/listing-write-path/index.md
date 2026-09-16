# Listing Write Path — API Phase 4

**Status:** Ready
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

- [ ] **Preparation**
  - [ ] Confirm Discovery is complete and the database is seeded

- [ ] **Schema**
  - [ ] `migrations/000005_listing_lifecycle.*`: `published_at timestamptz`,
        and an index on `(owner_id, created_at DESC)` for the owner's list
  - [ ] Verify: migrate up and down both succeed

- [ ] **Queries**
  - [ ] Insert, partial update, status transition, archive, list-by-owner,
        and an ownership lookup that does not leak existence
  - [ ] Verify: `sqlc generate` produces compiling Go

- [ ] **Contract**
  - [ ] Add the five operations and their request bodies to `api/openapi.yaml`,
        with the security requirements stated per operation
  - [ ] Verify: `oapi-codegen` regenerates and the interface compiles

- [ ] **Handlers**
  - [ ] Create: verified-only, validate, default to `published`
  - [ ] Update: owner-only, partial, revalidate the result as a whole
  - [ ] Status: owner-only, enforce the transition rules
  - [ ] Delete: owner-only, archive rather than remove
  - [ ] `GET /me/listings`: the owner's own, every status
  - [ ] Verify: by hand against the running API

- [ ] **Tests**
  - [ ] Creation by a verified student; refusal for unverified and anonymous
  - [ ] Validation: bad dates, non-positive rent, bedroom_of out of range
  - [ ] Partial update leaves untouched fields alone
  - [ ] Each transition, and the refusals: archived is terminal
  - [ ] Another user's listing is 404 on every write path
  - [ ] Verify: `go test ./...` passes

- [ ] **Final verification**
  - [ ] Build, vet, test, generators idempotent
  - [ ] Move this file to `docs/specs/implemented/listing-write-path/`

---

## Implementation Notes

> *Added after completion.*
