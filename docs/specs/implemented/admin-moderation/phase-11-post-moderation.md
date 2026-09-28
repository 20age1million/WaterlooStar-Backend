# Phase 11 — Post moderation

**Status:** Complete
**Feature:** [Admin and Moderation](./index.md)
**Objective:** An admin can see every listing and request whatever its status, take one down with a reason, and restore it to the state its owner left it in.

---

## Scope

### In scope

- The admin read surface over posts: every listing and every request, in any
  status, with search and filters and the owner attached.
- Takedown and restore, as a moderation axis independent of the owner's `status`.
- The rule that an owner cannot republish over a takedown.
- Offers following the listing they point at, as they already do.

### Out of scope

- Editing a post's content as an admin. Removing a bad address is a takedown; a
  moderator rewriting someone's listing is a different product decision.
- Permanently deleting a post. Ruled out for the feature.
- Bulk actions. One post at a time until there is evidence of volume.
- Reporting — a student flagging a post. That is a user-facing feature with its
  own queue, its own abuse surface and its own spec.
- Automated moderation of any kind.

---

## Dependencies / Prerequisites

- Phase 9 — the guard and the ledger.
- Phase 10 — nothing technically, but an operator who can take a post down and
  cannot suspend the account that posted it is half-equipped.
- The listing write path (Phase 4) and requests (Phase 6), whose `status` columns
  this phase deliberately leaves alone.

---

## Implementation Steps

- [x] **Preparation**
  - [x] Confirm Phase 9 is complete
  - [x] Update `index.md` — set Phase 11 to In Progress (went straight to Complete
        in the phase commit)

- [x] **Schema**
  - [x] Add `migrations/000009_post_moderation` — `removed_at`, `removed_by`
        (`ON DELETE SET NULL`) and `removed_reason` on both `listings` and
        `housing_requests`
  - [x] Add the partial indexes the public reads need now that they filter on
        `removed_at IS NULL` alongside `status = 'published'`
  - [x] Verify: migrate up and down cleanly; confirm the down migration leaves the
        Phase 5 and 6 indexes and CHECKs intact — dropping a column takes any
        constraint that mentions it, which migration 7 learned the hard way

- [x] **Queries**
  - [x] Add `internal/db/queries/admin_posts.sql` — `ListListingsForAdmin`,
        `CountListingsForAdmin`, `ListRequestsForAdmin`, `CountRequestsForAdmin`,
        `RemoveListing`, `RestoreListing`, `RemoveRequest`, `RestoreRequest`
  - [x] The admin lists are the only queries in the repository that do not filter
        on `status = 'published'`. Say so in a comment above each, because the rule
        they break is the one the rest of the file exists to keep
  - [x] Filters: status, removed or not, owner, and the same text search the public
        reads use
  - [x] Add `removed_at IS NULL` to every public read in `listings.sql`,
        `requests.sql` and `offers.sql`
  - [x] Restore clears the three columns and touches nothing else, so the post
        returns to the owner's status by having never lost it
  - [x] Verify: query tests, including a removed listing disappearing from the
        public list and from the offers on a request

- [x] **Contract and handlers**
  - [x] Add the admin post paths to `api/openapi.yaml`
  - [x] The admin post shape carries the moderation fields and the owner's email
        and suspension state — an operator judging a post needs to know whose it is
  - [x] Add `internal/httpapi/admin_posts.go`
  - [x] A reason is required to remove; restoring does not need one but is logged
        all the same
  - [x] Removing an already-removed post is refused rather than silently
        overwriting the original reason
  - [x] Each change and its ledger row in one transaction, with the previous
        status in `detail`
  - [x] Verify: by hand — take down a seeded listing, confirm it 404s publicly, is
        gone from the hub and from any request it was offered against; confirm the
        owner sees it in `/me/listings` marked as removed; restore it and confirm
        it returns as published

- [x] **The owner's side of a takedown**
  - [x] The owner still sees a removed post in `/me/listings` and `/me/requests`,
        with the moderation state and reason visible — a post that vanishes without
        explanation is a support ticket
  - [x] The owner's write path refuses to republish, update or change the status of
        a removed post. Without this, republishing silently overrides a moderator
  - [x] Decide and record: whether the reason text is shown to the owner verbatim
        or only that it was removed. Default to showing it — a reason written to be
        read is a reason worth writing
  - [x] Verify: as the owner, attempt to republish a removed listing and get a
        refusal that explains itself

- [x] **Tests**
  - [x] Query tests for each new query
  - [x] `internal/httpapi/admin_posts_test.go`: the admin list across statuses,
        takedown, restore, the double-removal refusal, the reason requirement, and
        the ledger row
  - [x] Handler tests proving the owner cannot republish a removed post
  - [x] Regression tests proving a removed post leaves the public list, the detail
        endpoint and the offers on a request
  - [x] Verify: `go test ./...` passes with and without a database

- [x] **Documentation**
  - [x] `README.md`: the full admin endpoint table and the new total
  - [x] `CLAUDE.md`: the moderation axis, why it is not `status`, and that the
        admin lists are the one exception to the published-only rule
  - [x] `docs/specs/INDEX.md`: move the feature from Active to Implemented and
        summarise the three phases
  - [x] Verify: the phase tables and the gap list match what is built

- [x] **Final verification**
  - [x] `go build ./...` and `go vet ./...` pass
  - [x] All tests pass, with and without a database
  - [x] Update `index.md` — set Phase 11 to Complete
  - [x] Move `docs/specs/active/admin-moderation/phase-11-post-moderation.md` →
        `docs/specs/implemented/admin-moderation/phase-11-post-moderation.md`
  - [x] Move `docs/specs/active/admin-moderation/index.md` →
        `docs/specs/implemented/admin-moderation/index.md`

---

## Completion Criteria

- [x] All checklist items completed and verified
- [x] An admin can list every listing and request in any status, with search and
      filters, and see who owns each
- [x] A taken-down post answers 404 publicly and is absent from the hub and from
      the offers on any request
- [x] The owner can see their removed post and the reason, and cannot republish it
- [x] Restoring returns the post to the status its owner had set, with no further
      action
- [x] Every takedown and restore produced exactly one ledger row
- [x] `index.md` and every phase doc moved to `docs/specs/implemented/admin-moderation/`
- [x] `docs/specs/INDEX.md` lists the feature as implemented

---

## Implementation Notes

**Key files changed:**
- `migrations/000009_post_moderation.{up,down}.sql`: `removed_at`, `removed_by`,
  `removed_reason` on `listings` and `housing_requests`, a CHECK that the time
  and reason are set together, and one partial index per table for the public
  reads (`status = 'published' AND removed_at IS NULL`, by `created_at`).
- `internal/db/queries/admin_posts.sql`: `ListListingsForAdmin`,
  `CountListingsForAdmin`, `ListRequestsForAdmin`, `CountRequestsForAdmin`,
  `RemoveListing`, `RestoreListing`, `RemoveRequest`, `RestoreRequest`.
- `internal/db/queries/listings.sql`, `requests.sql`, `offers.sql`:
  `removed_at IS NULL` on every public read, beside the Phase 10 suspension
  filter, and on the offer counts inside the request reads.
- `internal/db/posts.go`: `RemovePost` and `RestorePost` through `Audited`,
  shared by both tables.
- `internal/httpapi/admin_posts.go`: the six handlers. `listings_write.go` and
  `requests_write.go`: the ownership helpers refuse a removed post with a 409
  that quotes the reason, and the six refusal mappers carry it. `offers.go`:
  a removed listing cannot be offered. `mapping.go`: `removal` on listings and
  requests.
- `api/openapi.yaml`: the six admin paths; `PostRemoval`, `AdminOptionalReason`,
  `AdminPostOwner`, `AdminListing(Page)`, `AdminRequest(Page)`; an optional
  `removal` on `Listing` and `HousingRequest`; a 409 on the six owner writes.
- Tests: `internal/db/queries_moderation_test.go`,
  `internal/httpapi/admin_posts_test.go`; the fake mirrors the filter and the
  eight queries.
- `README.md`, `CLAUDE.md`, `docs/specs/INDEX.md`.

**Decision recorded (the spec asked for one):** the owner sees the moderator's
reason verbatim, in `removal.reason` on their own reads and in the 409 message
when they try to change the post. The spec's default.

**Divergences from plan:**
- **`removal` is optional on the public schemas**, not a new required field. It
  is set only when a post is removed, and a removed post never reaches a public
  read, so public responses are byte-for-byte unchanged.
- **Restoring without a reason records "Restored by an admin".** The spec makes
  the reason optional; the ledger's CHECK does not allow an empty one.
- **The owner's DELETE is refused too.** It archives, which is a status change,
  and the spec refuses status changes on a removed post.
- **Offering a removed listing is refused (400).** The spec only said offers
  follow their listing, which the read filter handles. Refusing a new offer of a
  hidden place follows the same reasoning as refusing an unpublished one.
- **The regression tests are in `admin_posts_test.go` and
  `queries_moderation_test.go`**, not in the listings, requests and offers
  handler files, for the reason recorded in Phase 10.

**Verification run:**
- `go build ./...`, `go vet ./...`: clean. Both generators idempotent.
- `go test ./...` with and without `PG_TEST_DSN`: pass.
- Migrations on the development database: `up` to 9, `down-one` to 8, `up`
  to 9. The indexes and constraints on both tables after `down-one` are
  identical to those before `up`, compared by name.
- By hand against a running API: took down one of `priyas`'s seeded listings as
  `meil`. Public count 6 → 5, its detail 404. Priya's `/me/listings` shows it
  `published` with the reason. Her republish got a 409 quoting the reason.
  Restore → count 6, detail 200.
