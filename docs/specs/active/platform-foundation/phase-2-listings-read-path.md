# Phase 2 — Listings Read Path

**Status:** Ready
**Feature:** [platform-foundation](./index.md)
**Objective:** Give "Housing Available" a structured schema and serve it — list and detail endpoints returning real rows, so the frontend can stop importing its sample data.

---

## Scope

### In scope

- `listings` table with every field the three screens display, as structured columns
- `listing_photos` table, created but unpopulated
- Seed data reproducing the six prototype fixtures so the screens look unchanged
- `GET /listings` — paginated, newest first, published only
- `GET /listings/{id}` — full detail, standard 404 envelope for an unknown id
- Owner summary embedded in both responses

### Out of scope

- Filtering, searching and sorting — the whole of the next feature
- Creating, editing or deleting a listing
- Photo upload
- "Looking for Housing" requests
- Saves, view counting, and the question thread
- Geographic search; `lat`/`lng` are stored but only returned, not queried against

---

## Dependencies / Prerequisites

- Phase 1 complete: `users` exists, so `listings.owner_id` has something to reference

---

## Implementation Steps

- [ ] **Preparation**
  - [ ] Confirm Phase 1 is marked Complete in `index.md`
  - [ ] Update `index.md` — set Phase 2 to In Progress

- [ ] **Schema**
  - [ ] Add `migrations/000003_listings.up.sql` / `.down.sql` creating `listings`:
        `id` uuid, `owner_id` uuid references `users`, `title`, `body`, `conditions` text[],
        `price_cents` int, `deposit_cents` int null, `start_date` date, `end_date` date,
        `lease_months` int, `term_tag`, `bedrooms_total` int, `bedroom_of` int null,
        `bathrooms` numeric, `bath_shared` bool, `furnished` bool, `utilities` text[],
        `parking` bool, `pets` bool, `address_line`, `neighbourhood`, `lat` numeric,
        `lng` numeric, `distance_m` int, `status` text default `published`,
        `views` int, `replies` int, `saves` int, `created_at`, `updated_at`
  - [ ] Comment `address_line` in the migration: public for now, pending the
        public-versus-private address decision recorded in `index.md`
  - [ ] Add `listing_photos` (`id`, `listing_id`, `url`, `position`, `created_at`)
  - [ ] Index `status` with `created_at desc` for the default listing order
  - [ ] Verify: `make migrate-up` then `make migrate-down` both succeed

- [ ] **Seed data**
  - [ ] Add `internal/db/seed/seed.go` (or a `make seed` target) inserting one verified seed user
        per distinct poster in the prototype fixtures, then the six listings from
        `Waterloostar-web/src/data/housing.ts` with their display strings decomposed into columns
  - [ ] Decompose deliberately: `'Jan 1 – Apr 30 · 4-month sublet'` becomes `start_date`,
        `end_date` and `lease_months`; `'1 of 4 bed'` becomes `bedroom_of` and `bedrooms_total`;
        `'Internet incl.'` becomes an entry in `utilities`
  - [ ] Make the seed idempotent so it can be re-run
  - [ ] Verify: `make seed` twice leaves exactly six listings

- [ ] **Queries**
  - [ ] Add `internal/db/queries/listings.sql` — list published with limit and offset, count
        published, get one by id with its owner, list photos for a listing
  - [ ] Verify: `sqlc generate` produces compiling Go

- [ ] **Contract**
  - [ ] Extend `api/openapi.yaml` with `GET /listings` and `GET /listings/{id}`
  - [ ] Add the `Listing` schema — money in cents as integers, dates as `date`, a nested
        `owner` summary (`id`, `display_name`, `initials`, `avatar_url`, `verified`), and
        `photos` as an array
  - [ ] Add `ListingPage` wrapping `data` with the `PageMeta` defined in Phase 0
  - [ ] Keep every field name in the product's own vocabulary — nothing named `renter`
  - [ ] Verify: `oapi-codegen` regenerates and the interface compiles

- [ ] **Handlers**
  - [ ] Implement the list handler with `page` and `per_page`, a sane default and a hard maximum
  - [ ] Implement the detail handler, returning the standard 404 envelope for an unknown or
        non-published id rather than any partial response
  - [ ] Both endpoints are public — no auth middleware
  - [ ] Verify: curl both endpoints and confirm the shapes match the contract

- [ ] **Tests**
  - [ ] `internal/httpapi/listings_test.go` — list returns seeded rows with correct page meta;
        `per_page` is clamped; detail returns a known listing; an unknown UUID returns 404 in the
        standard envelope; a malformed UUID returns 400
  - [ ] Verify: `go test ./...` passes

- [ ] **Final verification**
  - [ ] `go build ./...`, `go vet ./...` and `go test ./...` all pass
  - [ ] Generators produce no diff when re-run
  - [ ] `api/openapi.yaml` describes every endpoint the service serves
  - [ ] Update `index.md` — set Phase 2 to Complete
  - [ ] Move `docs/specs/active/platform-foundation/phase-2-listings-read-path.md` →
        `docs/specs/implemented/platform-foundation/phase-2-listings-read-path.md`
  - [ ] *(Final phase)* Move `docs/specs/active/platform-foundation/index.md` →
        `docs/specs/implemented/platform-foundation/index.md`

---

## Completion Criteria

- [ ] All checklist items completed and verified
- [ ] `GET /listings` returns the six seeded listings with correct pagination metadata
- [ ] `GET /listings/{id}` returns full structured detail, and an unknown id returns 404
- [ ] No response field exposes the internal *renter* vocabulary
- [ ] No regressions in related areas
- [ ] `index.md` phase status set to Complete
- [ ] Phase spec doc moved to `docs/specs/implemented/platform-foundation/`
- [ ] *(Final phase)* `index.md` moved to `docs/specs/implemented/platform-foundation/`

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
