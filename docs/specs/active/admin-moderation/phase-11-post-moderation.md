# Phase 11 — Post moderation

**Status:** Ready
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

- [ ] **Preparation**
  - [ ] Confirm Phase 9 is complete
  - [ ] Update `index.md` — set Phase 11 to In Progress

- [ ] **Schema**
  - [ ] Add `migrations/000009_post_moderation` — `removed_at`, `removed_by`
        (`ON DELETE SET NULL`) and `removed_reason` on both `listings` and
        `housing_requests`
  - [ ] Add the partial indexes the public reads need now that they filter on
        `removed_at IS NULL` alongside `status = 'published'`
  - [ ] Verify: migrate up and down cleanly; confirm the down migration leaves the
        Phase 5 and 6 indexes and CHECKs intact — dropping a column takes any
        constraint that mentions it, which migration 7 learned the hard way

- [ ] **Queries**
  - [ ] Add `internal/db/queries/admin_posts.sql` — `ListListingsForAdmin`,
        `CountListingsForAdmin`, `ListRequestsForAdmin`, `CountRequestsForAdmin`,
        `RemoveListing`, `RestoreListing`, `RemoveRequest`, `RestoreRequest`
  - [ ] The admin lists are the only queries in the repository that do not filter
        on `status = 'published'`. Say so in a comment above each, because the rule
        they break is the one the rest of the file exists to keep
  - [ ] Filters: status, removed or not, owner, and the same text search the public
        reads use
  - [ ] Add `removed_at IS NULL` to every public read in `listings.sql`,
        `requests.sql` and `offers.sql`
  - [ ] Restore clears the three columns and touches nothing else, so the post
        returns to the owner's status by having never lost it
  - [ ] Verify: query tests, including a removed listing disappearing from the
        public list and from the offers on a request

- [ ] **Contract and handlers**
  - [ ] Add the admin post paths to `api/openapi.yaml`
  - [ ] The admin post shape carries the moderation fields and the owner's email
        and suspension state — an operator judging a post needs to know whose it is
  - [ ] Add `internal/httpapi/admin_posts.go`
  - [ ] A reason is required to remove; restoring does not need one but is logged
        all the same
  - [ ] Removing an already-removed post is refused rather than silently
        overwriting the original reason
  - [ ] Each change and its ledger row in one transaction, with the previous
        status in `detail`
  - [ ] Verify: by hand — take down a seeded listing, confirm it 404s publicly, is
        gone from the hub and from any request it was offered against; confirm the
        owner sees it in `/me/listings` marked as removed; restore it and confirm
        it returns as published

- [ ] **The owner's side of a takedown**
  - [ ] The owner still sees a removed post in `/me/listings` and `/me/requests`,
        with the moderation state and reason visible — a post that vanishes without
        explanation is a support ticket
  - [ ] The owner's write path refuses to republish, update or change the status of
        a removed post. Without this, republishing silently overrides a moderator
  - [ ] Decide and record: whether the reason text is shown to the owner verbatim
        or only that it was removed. Default to showing it — a reason written to be
        read is a reason worth writing
  - [ ] Verify: as the owner, attempt to republish a removed listing and get a
        refusal that explains itself

- [ ] **Tests**
  - [ ] Query tests for each new query
  - [ ] `internal/httpapi/admin_posts_test.go`: the admin list across statuses,
        takedown, restore, the double-removal refusal, the reason requirement, and
        the ledger row
  - [ ] Handler tests proving the owner cannot republish a removed post
  - [ ] Regression tests proving a removed post leaves the public list, the detail
        endpoint and the offers on a request
  - [ ] Verify: `go test ./...` passes with and without a database

- [ ] **Documentation**
  - [ ] `README.md`: the full admin endpoint table and the new total
  - [ ] `CLAUDE.md`: the moderation axis, why it is not `status`, and that the
        admin lists are the one exception to the published-only rule
  - [ ] `docs/specs/INDEX.md`: move the feature from Active to Implemented and
        summarise the three phases
  - [ ] Verify: the phase tables and the gap list match what is built

- [ ] **Final verification**
  - [ ] `go build ./...` and `go vet ./...` pass
  - [ ] All tests pass, with and without a database
  - [ ] Update `index.md` — set Phase 11 to Complete
  - [ ] Move `docs/specs/active/admin-moderation/phase-11-post-moderation.md` →
        `docs/specs/implemented/admin-moderation/phase-11-post-moderation.md`
  - [ ] Move `docs/specs/active/admin-moderation/index.md` →
        `docs/specs/implemented/admin-moderation/index.md`

---

## Completion Criteria

- [ ] All checklist items completed and verified
- [ ] An admin can list every listing and request in any status, with search and
      filters, and see who owns each
- [ ] A taken-down post answers 404 publicly and is absent from the hub and from
      the offers on any request
- [ ] The owner can see their removed post and the reason, and cannot republish it
- [ ] Restoring returns the post to the status its owner had set, with no further
      action
- [ ] Every takedown and restore produced exactly one ledger row
- [ ] `index.md` and every phase doc moved to `docs/specs/implemented/admin-moderation/`
- [ ] `docs/specs/INDEX.md` lists the feature as implemented

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
