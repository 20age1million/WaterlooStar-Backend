# Phase 6 — Requests

**Status:** Complete
**Feature:** [Housing Requests](./index.md)
**Objective:** A verified student can post what they need, and anyone can browse and filter those requests.

---

## Scope

### In scope

- `housing_requests`: table, check constraints, status lifecycle, search index.
- Seven operations: browse, one by id, create, edit, set status, take down, and
  the poster's own.
- Discovery filters over requests, mirroring Phase 3's listing filters.
- Seed data, so the hub has something to show.

### Out of scope

- Offers. Phase 7.
- Matching a request against listings automatically. A filter is not a matcher,
  and a real one deserves its own feature.
- Notifying anyone that a request was posted. There is no email provider.

---

## Dependencies / Prerequisites

- Phase 5 — every query written here is tested against a real database.

---

## Implementation Steps

- [x] **Preparation**
  - [x] Confirm Phase 5 is complete
  - [x] Update `index.md` — set Phase 6 to In Progress

- [x] **Schema**
  - [x] Add a migration pair creating `housing_requests`: poster, title, body,
        `budget_cents`, `start_date`, `end_date`, `lease_months`, `term_tag`,
        `occupants`, `pets`, `furnished_preferred`, `max_distance_m`,
        `neighbourhood`, `status`, `published_at`, `views`, timestamps
  - [x] Constrain what the API validates: a positive budget, an end after the
        start, occupants between 1 and 12, a status in the four known values
  - [x] Add the generated `search` tsvector over title and body, with a GIN
        index, and the partial indexes browse and "my requests" will use
  - [x] Verify: `go run ./cmd/migrate up` then `down` leaves the schema as it
        was found

- [x] **Queries**
  - [x] Add `internal/db/queries/requests.sql` — `ListRequests`,
        `CountRequests`, `GetPublishedRequest`, `GetRequestForOwner`,
        `ListRequestsByPoster`, `CreateRequest`, `UpdateRequest`,
        `SetRequestStatus`
  - [x] Filters follow the Phase 3 pattern: `sqlc.narg(...) IS NULL OR ...`, so
        one prepared statement serves every combination
  - [x] Verify: query tests cover each one, including a filter combination that
        matches nothing

- [x] **Contract**
  - [x] Add the request schemas and paths to `api/openapi.yaml`, mirroring the
        listing operations in naming and error shapes
  - [x] Verify: `go tool oapi-codegen` regenerates with no hand edits, and the
        generators are idempotent

- [x] **Handlers**
  - [x] Add `internal/httpapi/requests.go` — browse and detail, with the filter
        mapping
  - [x] Add `internal/httpapi/requests_write.go` — create, edit, status, take
        down, own list; `RequireVerified` on create; 404 for someone else's
  - [x] Reuse the listing lifecycle rules: draft → published → paused →
        published, archived terminal
  - [x] Verify: by hand against the running API — post, edit, pause, confirm it
        leaves public results, republish, take down

- [x] **Seed**
  - [x] Extend `internal/db/seed/seed.go` with a handful of requests across
        terms and budgets, posted by the existing seeded students
  - [x] Verify: `go run ./cmd/seed` is still idempotent

- [x] **Tests**
  - [x] Query tests for every new query (Phase 5's guard enforces this)
  - [x] Handler tests in `internal/httpapi/requests_test.go`: validation,
        ownership, lifecycle, and each filter
  - [x] Extend the in-memory fake with the new queries
  - [x] Verify: `go test ./...` passes

- [x] **Final verification**
  - [x] `go vet ./...` and `go build ./...` pass
  - [x] All tests pass, with and without a database available
  - [x] Update `index.md` — set Phase 6 to Complete
  - [x] Move `docs/specs/active/housing-requests/phase-6-requests.md` →
        `docs/specs/implemented/housing-requests/phase-6-requests.md`

---

## Completion Criteria

- [x] All checklist items complete
- [x] A verified student can post, edit, pause, republish and take down a
      request; an unverified one is refused with a readable reason
- [x] Anyone can browse requests and filter by term, budget, distance,
      occupants, pets and furnished, with a total describing the whole matching
      set
- [x] A paused, draft or taken-down request is invisible to everyone but its
      poster
- [x] Someone else's request answers 404

---

## Implementation Notes

**Delivered as specified.** `housing_requests`, nine queries, seven operations,
and five seeded requests written to line up with the seeded listings — an owner
browsing them finds someone their actual room would suit.

Verified against the real API, not only by test:

- create → 201, publicly visible, `published_at` stamped
- a partial edit changed the budget and nothing else
- pause hid it (6 → 5), republish restored it, and `published_at` did not move
- take down → 204, and un-archiving answered 400: taking down is terminal
- a second seeded account got **404** on PATCH, DELETE and status
- the filters, by hand: an owner asking $900 sees 4 of 5 requests; a place 4 km
  out matches both the 6 km request and the one with no stated radius

**The filters are inverted from the listing ones, deliberately.** A listing's
price is what is asked and a request's budget is a ceiling, so `budget_min`
means "who can afford this rent". A listing's `distance_m` is where it is and a
request's `max_distance_m` is how far out they would go, so `distance_min`
means "who would accept a place this far out". One consequence is worth stating:
**a request with no stated radius passes every distance filter**, where a
listing with no recorded distance is excluded by one. No preference is not a
narrow preference. Both the SQL and the in-memory fake encode this, and a query
test covers it.

**`published_at` is stamped on creation here**, unlike listings — the
inconsistency Phase 5 found is not repeated. `CreateListing` remains as it was;
fixing it is a change to shipped behaviour and belongs to its own piece of work.

**The generated sort constant moved.** Adding a second `sort` enum made
oapi-codegen rename `gen.New` to `gen.ListListingsParamsSortNew`, which broke
`listings.go` until it was updated. Nothing else in the contract shifted.

**Views on a request** are settable only by the seed, matching listings: the
column exists so the seeded market looks lived-in, and a real request starts at
zero. `offers` is in the contract and always 0 until Phase 7.

**`cmd/seed` now reports "posts"** rather than "listings", since it writes both.
