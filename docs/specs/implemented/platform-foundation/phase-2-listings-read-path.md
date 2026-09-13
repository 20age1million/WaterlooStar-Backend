# Phase 2 — Listings Read Path

**Status:** Complete
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

- [x] **Preparation**
  - [x] Confirm Phase 1 is marked Complete in `index.md`
  - [x] Update `index.md` — set Phase 2 to In Progress

- [x] **Schema**
  - [x] Add `migrations/000003_listings.up.sql` / `.down.sql` creating `listings`:
        `id` uuid, `owner_id` uuid references `users`, `title`, `body`, `conditions` text[],
        `price_cents` int, `deposit_cents` int null, `start_date` date, `end_date` date,
        `lease_months` int, `term_tag`, `bedrooms_total` int, `bedroom_of` int null,
        `bathrooms` numeric, `bath_shared` bool, `furnished` bool, `utilities` text[],
        `parking` bool, `pets` bool, `address_line`, `neighbourhood`, `lat` numeric,
        `lng` numeric, `distance_m` int, `status` text default `published`,
        `views` int, `replies` int, `saves` int, `created_at`, `updated_at`
  - [x] Comment `address_line` in the migration: public for now, pending the
        public-versus-private address decision recorded in `index.md`
  - [x] Add `listing_photos` (`id`, `listing_id`, `url`, `position`, `created_at`)
  - [x] Index `status` with `created_at desc` for the default listing order
  - [x] Verify: `make migrate-up` then `make migrate-down` both succeed

- [x] **Seed data**
  - [x] Add `internal/db/seed/seed.go` (or a `make seed` target) inserting one verified seed user
        per distinct poster in the prototype fixtures, then the six listings from
        `Waterloostar-web/src/data/housing.ts` with their display strings decomposed into columns
  - [x] Decompose deliberately: `'Jan 1 – Apr 30 · 4-month sublet'` becomes `start_date`,
        `end_date` and `lease_months`; `'1 of 4 bed'` becomes `bedroom_of` and `bedrooms_total`;
        `'Internet incl.'` becomes an entry in `utilities`
  - [x] Make the seed idempotent so it can be re-run
  - [x] Verify: `make seed` twice leaves exactly six listings

- [x] **Queries**
  - [x] Add `internal/db/queries/listings.sql` — list published with limit and offset, count
        published, get one by id with its owner, list photos for a listing
  - [x] Verify: `sqlc generate` produces compiling Go

- [x] **Contract**
  - [x] Extend `api/openapi.yaml` with `GET /listings` and `GET /listings/{id}`
  - [x] Add the `Listing` schema — money in cents as integers, dates as `date`, a nested
        `owner` summary (`id`, `display_name`, `initials`, `avatar_url`, `verified`), and
        `photos` as an array
  - [x] Add `ListingPage` wrapping `data` with the `PageMeta` defined in Phase 0
  - [x] Keep every field name in the product's own vocabulary — nothing named `renter`
  - [x] Verify: `oapi-codegen` regenerates and the interface compiles

- [x] **Handlers**
  - [x] Implement the list handler with `page` and `per_page`, a sane default and a hard maximum
  - [x] Implement the detail handler, returning the standard 404 envelope for an unknown or
        non-published id rather than any partial response
  - [x] Both endpoints are public — no auth middleware
  - [x] Verify: curl both endpoints and confirm the shapes match the contract

- [x] **Tests**
  - [x] `internal/httpapi/listings_test.go` — list returns seeded rows with correct page meta;
        `per_page` is clamped; detail returns a known listing; an unknown UUID returns 404 in the
        standard envelope; a malformed UUID returns 400
  - [x] Verify: `go test ./...` passes

- [x] **Final verification**
  - [x] `go build ./...`, `go vet ./...` and `go test ./...` all pass
  - [x] Generators produce no diff when re-run
  - [x] `api/openapi.yaml` describes every endpoint the service serves
  - [x] Update `index.md` — set Phase 2 to Complete
  - [x] Move `docs/specs/active/platform-foundation/phase-2-listings-read-path.md` →
        `docs/specs/implemented/platform-foundation/phase-2-listings-read-path.md`
  - [x] *(Final phase)* Move `docs/specs/active/platform-foundation/index.md` →
        `docs/specs/implemented/platform-foundation/index.md`

---

## Completion Criteria

- [x] All checklist items completed and verified
- [x] `GET /listings` returns the six seeded listings with correct pagination metadata
- [x] `GET /listings/{id}` returns full structured detail, and an unknown id returns 404
- [x] No response field exposes the internal *renter* vocabulary
- [x] No regressions in related areas
- [x] `index.md` phase status set to Complete
- [x] Phase spec doc moved to `docs/specs/implemented/platform-foundation/`
- [x] *(Final phase)* `index.md` moved to `docs/specs/implemented/platform-foundation/`

---

## Implementation Notes

**Key files changed:**

- `migrations/000003_listings.*` — `listings` and `listing_photos`, with check constraints on
  price, date ordering, counters, and each enumerated column. `address_line` carries the
  comment recording that it is public pending the deferred decision.
- `internal/db/queries/listings.sql` — list, count, get-by-id (all filtering
  `status = 'published'` in SQL so no handler can forget), photos by listing and by batch,
  plus create/delete for the seed.
- `internal/db/seed/seed.go`, `cmd/seed/main.go` — the six prototype fixtures decomposed into
  columns. Refuses to run unless `ENVIRONMENT=development`.
- `internal/httpapi/listings.go` — both handlers, pagination bounds, photo batching.
- `internal/httpapi/mapping.go` — `toListing`, which formats nothing.
- `internal/httpapi/router.go` — `GinServerOptions.ErrorHandler` added (see divergence 4).
- `sqlc.yaml` — `date` and `pg_catalog.numeric` overrides so rows carry Go primitives.
- `api/openapi.yaml` — `GET /listings`, `GET /listings/{id}`, `Listing`, `ListingOwner`,
  `ListingPhoto`, `ListingPage`.
- `README.md` — seed step, and a note that `make` is optional.

**Divergences from plan:**

1. **More columns than the checklist listed.** The spec named `bath_shared`; the fixtures
   distinguish "Shared bath", "Ensuite" and "1 bath", so it became `bath_type`
   (shared/private/ensuite). Likewise `unit_type` (room/studio/unit) was needed for "Studio",
   and `laundry` for "Laundry". `distance_m` is metres rather than km so the radius filter in
   the next feature compares integers.
2. **Commute fields added.** `commute_minutes` / `commute_mode` carry "9 min walk" and
   "18 min by bus"; `minutes_to_transit` and `minutes_to_grocery` carry the detail page's other
   two travel times. All nullable placeholders until a routing provider is chosen in Phase 8 —
   without them the frontend would have to keep inventing that copy.
3. **`lat`/`lng` are seeded as null.** Populating them needs geocoding, which is deferred. The
   columns and the contract fields exist; the map placeholder does not read them yet.
4. **A second error hook was missing.** `GinServerOptions.ErrorHandler` handles path and query
   parameter binding, which is a *different* hook from the strict handler's
   `RequestErrorHandlerFunc` replaced in Phase 1 — that one only covers the body. Until this
   was set, a malformed UUID in the path escaped the documented envelope. Caught by
   `TestGetListingMalformedIDReturns400`.
5. **`make` is not installed on the development machine.** The Makefile written in Phase 0 and
   documented in the README is unrunnable on Windows without it; Phase 0 was verified with
   direct `go run` commands, so this only surfaced now. The Makefile is kept for CI and
   Unix machines, and the README now gives the plain commands alongside.
6. **The seed rewrites rather than upserts.** Every listing is deleted and recreated so that
   editing a fixture is reflected exactly; seed users are created only when absent.

**Verification run:**

- `go build ./...`, `go vet ./...`, `go test ./...` pass. Generators idempotent.
- Migration `up` → version 3 with `listings` and `listing_photos`; `down-one` removes both;
  `up` again → version 3.
- `go run ./cmd/seed` twice → 6 listings, 8 users. Idempotent.
- `GET /listings` → `meta {page 1, per_page 20, total 6, total_pages 1}`, six rows, each with
  an embedded verified owner and `photos: []` rather than null.
- Ordering is newest first, matching the fixtures' `postedDaysAgo`.
- `GET /listings?page=2&per_page=4` → `total_pages 2`, 2 rows. `per_page=9999` → clamped to
  100. `per_page=0` → default 20. A page past the end is an empty 200, not an error.
- `GET /listings/{id}` returns structured detail: `bedroom_of 1` of `bedrooms_total 4`,
  `bath_type shared`, `utilities [internet hydro water heat]`, four `conditions`, the full
  body, and commute times 9/2/6.
- Unknown id → `404 {"code":"not_found",…}`. Malformed id → `400 {"code":"bad_request",
  "message":"That id is not a valid UUID.",…}`, in the standard envelope.
- Draft, paused and archived listings are absent from the list and 404 on the detail route —
  indistinguishable from a listing that never existed.
- Both endpoints answer an anonymous request with 200.
- `renter` and `rentee` appear nowhere in a path, field name or response body. The single
  match in `openapi.yaml` is the vocabulary note stating the rule.
