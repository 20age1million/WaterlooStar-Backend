# Phase 6 — Requests

**Status:** Ready
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

- [ ] **Preparation**
  - [ ] Confirm Phase 5 is complete
  - [ ] Update `index.md` — set Phase 6 to In Progress

- [ ] **Schema**
  - [ ] Add a migration pair creating `housing_requests`: poster, title, body,
        `budget_cents`, `start_date`, `end_date`, `lease_months`, `term_tag`,
        `occupants`, `pets`, `furnished_preferred`, `max_distance_m`,
        `neighbourhood`, `status`, `published_at`, `views`, timestamps
  - [ ] Constrain what the API validates: a positive budget, an end after the
        start, occupants between 1 and 12, a status in the four known values
  - [ ] Add the generated `search` tsvector over title and body, with a GIN
        index, and the partial indexes browse and "my requests" will use
  - [ ] Verify: `go run ./cmd/migrate up` then `down` leaves the schema as it
        was found

- [ ] **Queries**
  - [ ] Add `internal/db/queries/requests.sql` — `ListRequests`,
        `CountRequests`, `GetPublishedRequest`, `GetRequestForOwner`,
        `ListRequestsByPoster`, `CreateRequest`, `UpdateRequest`,
        `SetRequestStatus`
  - [ ] Filters follow the Phase 3 pattern: `sqlc.narg(...) IS NULL OR ...`, so
        one prepared statement serves every combination
  - [ ] Verify: query tests cover each one, including a filter combination that
        matches nothing

- [ ] **Contract**
  - [ ] Add the request schemas and paths to `api/openapi.yaml`, mirroring the
        listing operations in naming and error shapes
  - [ ] Verify: `go tool oapi-codegen` regenerates with no hand edits, and the
        generators are idempotent

- [ ] **Handlers**
  - [ ] Add `internal/httpapi/requests.go` — browse and detail, with the filter
        mapping
  - [ ] Add `internal/httpapi/requests_write.go` — create, edit, status, take
        down, own list; `RequireVerified` on create; 404 for someone else's
  - [ ] Reuse the listing lifecycle rules: draft → published → paused →
        published, archived terminal
  - [ ] Verify: by hand against the running API — post, edit, pause, confirm it
        leaves public results, republish, take down

- [ ] **Seed**
  - [ ] Extend `internal/db/seed/seed.go` with a handful of requests across
        terms and budgets, posted by the existing seeded students
  - [ ] Verify: `go run ./cmd/seed` is still idempotent

- [ ] **Tests**
  - [ ] Query tests for every new query (Phase 5's guard enforces this)
  - [ ] Handler tests in `internal/httpapi/requests_test.go`: validation,
        ownership, lifecycle, and each filter
  - [ ] Extend the in-memory fake with the new queries
  - [ ] Verify: `go test ./...` passes

- [ ] **Final verification**
  - [ ] `go vet ./...` and `go build ./...` pass
  - [ ] All tests pass, with and without a database available
  - [ ] Update `index.md` — set Phase 6 to Complete
  - [ ] Move `docs/specs/active/housing-requests/phase-6-requests.md` →
        `docs/specs/implemented/housing-requests/phase-6-requests.md`

---

## Completion Criteria

- [ ] All checklist items complete
- [ ] A verified student can post, edit, pause, republish and take down a
      request; an unverified one is refused with a readable reason
- [ ] Anyone can browse requests and filter by term, budget, distance,
      occupants, pets and furnished, with a total describing the whole matching
      set
- [ ] A paused, draft or taken-down request is invisible to everyone but its
      poster
- [ ] Someone else's request answers 404
