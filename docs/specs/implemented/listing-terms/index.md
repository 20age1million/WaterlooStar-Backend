# Listing Terms — API Phase 12

**Status:** Complete
**Branch:** `feature/listing-terms`
**Ticket / Work Item:** N/A
**Owner(s):** WaterlooStar backend
**Date:** 2026-09-27

---

## Purpose / Goal

Let an owner say two things a listing cannot express today, and let search use
them. The first is whether a student may rent **part** of the listed dates, and
for how long at minimum. The second is **roughly what the bills outside the rent
cost**, so two places can be compared on what a student will actually pay each
month. Search also gains a per-month availability count, which the reworked
housing hub draws above its results.

---

## Problem Statement / Motivation

**Shorter stays.** A listing has one date range and nothing else. A student
subletting January to April can't say "two months is fine too", and a student
who needs January and February can't tell which January-to-April places would
take them. The co-op calendar makes this common: a student starting a
work term in March, or leaving in February, is looking for exactly the partial
stay that no listing admits to offering.

**The real monthly cost.** Rent is the only money figure, and it compares badly.
A $690 room where the tenant pays hydro and internet can cost more than an
$820 room with everything included, and nothing in a listing says so. The
`utilities` list says *which* bills are included, but not what the others cost.
It can't be derived from that list either: `gas` is one of its values, and most
places have no gas bill at all.

**Availability by month.** The reworked hub (a separate frontend feature) opens
with a strip of the twelve months, each showing how many places are open then
under the student's other filters. Computing that from the existing endpoint
would take twelve requests per page load.

Why now: the frontend's housing rework is specified alongside this feature and
depends on all three. The developer chose these semantics on 2026-09-27; see
Trade-offs.

---

## Proposed Solution / Design

Three nullable-or-defaulted columns on `listings`, exposed through the existing
listing schemas; one computed field; one new sort, one new filter and one new
read endpoint. No existing field changes meaning.

**Shorter stays.** `shorter_stays` (boolean, default false) and
`min_stay_months` (nullable integer). When `shorter_stays` is true the owner
accepts any stay of at least `min_stay_months` inside the listed dates, at the
same monthly rent. When false, the listing is **whole lease only**: its dates
are the only dates on offer. `min_stay_months` is required exactly when
`shorter_stays` is true, and lies between 1 and `lease_months`.

**Search stays full-cover.** The `start_after` / `end_before` filter keeps its
current meaning: a listing appears only if it covers the whole window searched
for. That was the developer's decision. A whole-lease-only listing that covers the
window still appears. The frontend labels it, and the new ranking puts it below
the places that can be taken for exactly that window.

**Ranking with a window.** When both dates are given and `sort=match`,
listings that can be taken for exactly the searched window rank first, then the
rest, each group newest first. "Exactly" means the listing's dates equal the
window, or `shorter_stays` is true and the window is at least `min_stay_months`
long. Today `match` falls through to newest first. Without a window it still
does.

**Bill estimate.** `bills_estimate_cents` (nullable integer): the owner's
estimate of the tenant's monthly share of every bill the rent does not include.
`0` means nothing extra to pay; null means the owner has not said. The listing
response gains `all_in_cents`, which is `price_cents + bills_estimate_cents`, or
null when the estimate is null. The API never guesses a figure the owner did not
give.

**Filtering and sorting on all-in cost.** A new `all_in_max_cents` filter keeps
listings whose all-in cost is known and at most that amount. A new sort,
`allInAsc`, orders by all-in cost, with unknown last. `price_max_cents` keeps
meaning rent alone.

**Availability.** `GET /listings/availability?year=2027` returns, for each of the
twelve months of that year, how many published listings are open at any point
in that month. It takes the same filters as `GET /listings` except the date
window and paging. It is public, like browsing.

### Key Components

- **`migrations/000010_listing_terms`**: the three columns and their CHECKs.
  Number 10 because `feature/admin-moderation` holds 8 and plans 9; see
  Constraints.
- **`internal/db/queries/listings.sql`**: the new filter and sort in
  `ListListings` / `CountListings`; the create and update statements carry the
  three columns; a new `ListingAvailabilityByMonth`.
- **`internal/httpapi/listings_validate.go`**: the cross-field rules.
- **`internal/httpapi/listings.go`, `mapping.go`**: `all_in_cents`, the new
  parameters, and the availability handler.
- **`api/openapi.yaml`**: the fields on `Listing`, `ListingInput` and
  `ListingUpdate`; the parameters; `/listings/availability` and its schema.
- **`internal/db/seed`**: some seeded listings take shorter stays and bill
  estimates, so development shows every state.

### Data / Control Flow

- An owner posts or edits a listing with `shorter_stays`, `min_stay_months` and
  `bills_estimate_cents`. Validation checks each field alone and against
  `lease_months` and the other two, against the listing as it would stand after
  a partial update, as Phase 4 already does.
- A student browses with a window and `sort=match`. The SQL filters to
  full-cover as before, then orders the exact-window group first.
- Every listing response carries the three fields and `all_in_cents`.
- The hub asks `/listings/availability` once with its current filters and draws
  the twelve bars.

---

## Layers / Areas Affected

| Layer / Area | Change |
|---|---|
| Database schema | `listings.shorter_stays`, `min_stay_months`, `bills_estimate_cents`, with CHECKs |
| Migrations | One pair, `000010_listing_terms` |
| DB access layer | Create/update carry the columns; new filter and sort; new availability query |
| API handlers | New fields mapped; new parameters; `GET /listings/availability` |
| DTOs / contracts | `api/openapi.yaml`: three input fields, four output fields, two parameters, one sort value, one path, one schema |
| Validation | Cross-field rules for the stay and the estimate |
| Seed data | Varied values across the six seeded listings |
| Tests | Query tests for the filter, the ranking and availability; handler tests for validation, mapping and the endpoint |
| Rate limiting | None. The write endpoints already limited cover the new fields |
| Configuration / Deployment | None |

---

## Phase Tracker

| Phase | Title | Status | Location |
|---|---|---|---|
| 12 — Listing terms | Shorter stays, bill estimates, all-in cost, match ranking, monthly availability | Complete | `docs/specs/implemented/listing-terms/` |

Single phase; the checklist is below.

---

## Implementation Steps

- [x] **Preparation**
  - [x] Confirm the migration number: the next free number after `main` and
        every branch that merges before this one. At writing, that is 10 —
        confirmed: `main` holds 1–9 after Admin and Moderation merged
  - [x] Set this feature to In Progress in `docs/specs/INDEX.md` (went straight to
        Implemented in the phase commit)

- [x] **Schema**
  - [x] `migrations/000010_listing_terms.up.sql`: `shorter_stays boolean NOT NULL
        DEFAULT false`, `min_stay_months integer`, `bills_estimate_cents integer`
  - [x] CHECK: `min_stay_months` is null exactly when `shorter_stays` is false
  - [x] CHECK: `min_stay_months` between 1 and `lease_months`
  - [x] CHECK: `bills_estimate_cents` null or between 0 and 100000 ($1,000)
  - [x] Existing rows take the defaults: whole lease only, estimate unknown.
        No backfill
  - [x] Down migration drops the three columns and nothing else
  - [x] Verify: `up`, `down-one`, `up` on the development database. Not `down`,
        which rolls back every migration

- [x] **Contract**
  - [x] `Listing`: `shorter_stays` (required), `min_stay_months` (nullable),
        `bills_estimate_cents` (nullable), `all_in_cents` (nullable, read-only)
  - [x] `ListingInput` and `ListingUpdate`: the three writable fields,
        optional, with the rules in their descriptions
  - [x] `GET /listings`: `all_in_max_cents`; `allInAsc` added to the `sort` enum;
        the `match` description states the window ranking
  - [x] `GET /listings/availability`: `year` (required, 2020–2100) and every
        `GET /listings` filter except `start_after`, `end_before`, `page`,
        `per_page`, `sort`; returns `ListingAvailability`,
        `{year, months: [{month: 1..12, open: int}]}`, always twelve entries
  - [x] Verify: both generators run cleanly and twice without a diff

- [x] **Queries**
  - [x] `ListListings` / `CountListings`: `all_in_max` filter excluding unknown
        all-in; `allInAsc` ordering, unknown last; `match` ordering by the
        exact-window predicate when both dates are given
  - [x] Create and update statements carry the three columns
  - [x] `ListingAvailabilityByMonth`: published listings under the shared
        filters, counted per month of the year where
        `start_date <= last day of month AND end_date >= first day of month`,
        with months that have none returned as zero
  - [x] Verify: query tests; `TestEveryQueryIsExercised` names nothing new

- [x] **Validation and handlers**
  - [x] `shorter_stays: true` without `min_stay_months` → 400 `validation_failed` on
        `min_stay_months`; `min_stay_months` with `shorter_stays: false`, or
        outside 1..`lease_months`, likewise
  - [x] `bills_estimate_cents` below 0 or above 100000 → 400 `validation_failed`
  - [x] A partial update is validated against the whole listing as it would
        stand, so shortening `lease_months` below an existing minimum stay is
        refused
  - [x] Map the four fields; `all_in_cents` computed in one place
  - [x] The availability handler, public, reusing the listing filter mapping
  - [x] Verify by hand: post a listing with each combination; browse with a
        window and `sort=match`; read availability with and without filters

- [x] **Seed data**
  - [x] At least two seeded listings take shorter stays with different minimums;
        at least three carry an estimate, one of them `0`; one stays unknown

- [x] **Tests**
  - [x] Query: full-cover filter unchanged for both kinds of listing; the
        match ranking puts exact-window listings first, and a flexible listing
        whose minimum exceeds the window does not count as exact; `all_in_max`
        and `allInAsc` with unknown estimates; availability counts at month
        boundaries (a listing ending Jan 31 is not open in February) and the
        twelve-entry shape
  - [x] Handler: every validation rule; the partial-update rule; mapping of
        `all_in_cents` including null; availability is public
  - [x] Extend the in-memory fake querier
  - [x] Verify: `go test ./...` passes with and without `PG_TEST_DSN`

- [x] **Documentation**
  - [x] `README.md`: the endpoint table and a paragraph on shorter stays and
        all-in cost
  - [x] `CLAUDE.md`: the phase table; the rule that the API never invents a bill
        figure

- [x] **Final verification**
  - [x] `go build ./...`, `go vet ./...`, all tests pass
  - [x] Update this file's Phase Tracker to Complete
  - [x] Move `docs/specs/active/listing-terms/index.md` →
        `docs/specs/implemented/listing-terms/index.md`
  - [x] Update `docs/specs/INDEX.md`

---

## Completion Criteria

- [x] All checklist items completed and verified
- [x] An owner can offer shorter stays with a minimum, and whole-lease-only is the default
- [x] Every listing response carries the three fields and `all_in_cents`
- [x] A windowed `match` search ranks exact-window listings first
- [x] `all_in_max_cents` and `allInAsc` behave with unknown estimates as specified
- [x] `/listings/availability` returns twelve months under the shared filters
- [x] No regressions: existing filters, sorts and responses unchanged apart from the added fields
- [x] Spec moved to `implemented/`, `INDEX.md` updated

---

## Implementation Notes

**Key files changed:**
- `migrations/000010_listing_terms.{up,down}.sql`: the three columns and three
  named CHECKs.
- `internal/db/queries/listings.sql`: `all_in_max` on `ListListings` and
  `CountListings`; `allInAsc` and the window-aware `match` in the ordering;
  the three columns in `CreateListing` and `UpdateListing`; the new
  `ListingAvailabilityByMonth`.
- `api/openapi.yaml`: four fields on `Listing`, three on `ListingInput` and
  `ListingUpdate`, `all_in_max_cents`, `allInAsc`, the `match` description,
  `GET /listings/availability` and `ListingAvailability`.
- `internal/httpapi/listings_validate.go`: the cross-field rules,
  `minStayChange`, and the update mapping. `listings_write.go`: create.
  `mapping.go`: the four fields and `allInCents`. `listings.go`: the filter
  and `GetListingAvailability`.
- `internal/db/seed/seed.go`: shorter stays from 2 and from 4 months; estimates
  of $15, $95, $0 and $85; two listings left unstated.
- `internal/db/dbtest/fixtures.go`: `LeaseMonths` and the three terms as
  options.
- Tests: `internal/db/queries_listing_terms_test.go`,
  `internal/httpapi/listing_terms_test.go`; the fake mirrors the new SQL;
  `queries_listings_test.go`'s `browse` passes the new filter to the count.
- `README.md`, `CLAUDE.md`, `docs/specs/INDEX.md`.

**Divergences from plan:**
- **The two nullable columns are updated through explicit "set" flags, not
  COALESCE.** `UpdateListing` COALESCEs every column, so a PATCH could never
  clear one. Turning shorter stays off has to set `min_stay_months` to NULL or
  the CHECK refuses the row, so `set_min_stay` and `set_bills_estimate` were
  added. The same flaw already affects `deposit_cents` and `distance_m`, which
  cannot be cleared once set. That is recorded in `CLAUDE.md` and left for the
  developer: it predates this feature and is outside its spec.
- **Turning shorter stays off clears the minimum without being told to.** The
  spec's rule ("a minimum with shorter stays off is refused") holds for what is
  stored, but a PATCH of `{"shorter_stays": false}` alone now succeeds rather
  than demanding `min_stay_months: null` as well.
- **The listing test fixture gained a lease-length option.** It hard-coded 4
  months, which the new minimum-stay CHECK would refuse for longer leases.
- **`all_in_cents` is required in the schema and always present**, null
  included, so a client never has to tell "absent" from "unknown".

**Verification run:**
- `go build ./...`, `go vet ./...`: clean. Both generators idempotent.
- `go test ./...` with and without `PG_TEST_DSN`: pass, including
  `TestEveryQueryIsExercised`. The `browse` helper's list-and-count check caught
  that it was not passing the new filter to the count; fixed in the helper.
- Mutation check: making `allInCents` treat an unstated estimate as $0 fails
  `TestListingTermsAreStoredAndReported`. Restored.
- Migrations on the development database: `up` to 10, `down-one` to 9 (columns
  and constraints identical to before, compared by name), `up` to 10, then the
  seed reloaded.
- By hand against a running API on the reseeded data:
  - A Jan–Apr `match` browse lists the three exact-window listings first; a
    Jan–Feb browse puts only the 2-month minimum first.
  - `allInAsc` orders $760, $785, $860, $1,195, then the two unstated.
  - Availability for 2027 is 4/4/4/4, 3/3/3/3, 2/2/2/2, and 2/2/2/2, 1/1/1/1,
    0/0/0/0 under an all-in ceiling of $900.
  - Posting shorter stays from 3 months with a $60 estimate reported an all-in
    of $780. A minimum past the lease was refused on `min_stay_months`. One
    PATCH turning both off cleared the minimum, the estimate and the all-in.
  - The check listing was archived.

---

## Trade-offs / Alternatives Considered

- **Full-cover search only**: chosen by the developer. Showing places that
  cover only part of the window was offered and declined. A student sees places
  they can live in for the whole window, and the ranking separates "exactly your
  dates" from "whole lease only".
- **Shorter stays as a switch plus a minimum**: chosen by the developer, over a
  switch alone (no floor for the owner) and over a separate short-stay rent
  (more form, more search). Rent stays one monthly figure.
- **Owner-entered bill estimate**: chosen by the developer, over site-wide
  averages (a guess presented as a fact) and over averages as a fallback (real
  and guessed figures mixed without the student knowing which). Blank means
  unknown, and unknown is never counted as zero.
- **One estimate, not one per utility**: an owner knows "about $80 for your
  share" far more often than each bill separately, and the student needs the total.
- **Availability as its own endpoint** rather than twelve list calls from the
  frontend, or a field on every list response that most callers would not use.
- **`all_in_cents` computed, not stored**: it is two columns added together, and
  storing it would give it a way to drift.

---

## Assumptions

- A monthly figure is precise enough for a minimum stay. Nobody asks for a
  17-day minimum on a four-month sublet.
- Owners who include every bill will enter `0` when the form asks. If most leave
  it blank, all-in cost will be sparse, and that is visible, not wrong.
- The availability query is cheap at this site's size: one grouped scan of
  published listings under the same filters as the list.

---

## Constraints

- **Migration order.** `feature/admin-moderation` holds migration 8 and plans 9,
  and golang-migrate will not apply a lower number after a higher one. This
  feature takes 10 and must merge **after** Admin and Moderation. If the order
  changes, renumber at implementation, before anything is merged.
- The contract comes first: `api/openapi.yaml` before any handler.
- The frontend's housing rework consumes this. The contract shape above is what
  it will generate types from, so changes to it after acceptance go through both
  specs.

---

## Success Criteria / Definition of Done

- The three states of a listing — whole lease only, shorter stays from N
  months, and each with or without a bill estimate — can be posted, edited,
  read and searched.
- Query and handler tests cover every rule above.
- All phase checklist items completed and verified.

---

## References (internal only)

- Search semantics this extends: `docs/specs/implemented/discovery/index.md`
- Write path and partial-update validation: `docs/specs/implemented/listing-write-path/index.md`
- The consumer: `../WaterlooStar-Frontend/docs/specs/active/housing-rework/index.md`
