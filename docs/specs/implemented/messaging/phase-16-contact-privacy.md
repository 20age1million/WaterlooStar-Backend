# Phase 16 — Contact privacy and address sharing

**Status:** Complete (2026-10-16, on `feature/messaging-privacy`)
**Feature:** [Messaging with Contact Privacy](./index.md)
**Objective:** The exact address and coordinates leave every public read and the search index, and the listing's owner can share them with one student by an explicit action inside their conversation.

---

## Scope

### In scope

- `Listing.address_line` becomes **owner-only**: it is present on the owner's
  own reads and the admin surface, and absent everywhere else.
- `Listing.lat` and `lng` are **null on public reads**. Today they are null on
  every row anyway, so the change is in force before the maps feature starts
  writing them.
- A public approximate-location field, `cross_streets`, added to `Listing`,
  `ListingInput` and `ListingUpdate` *(Q-7, decided 2026-10-16)*.
- `listings.search` rebuilt without `address_line` and with `cross_streets`, so
  `q` cannot find a listing by its exact address.
- **Sharing** *(decided 2026-10-16)*: one new operation,
  `POST /conversations/{id}/share-address` (`shareAddress`), by the owner only.
  It records `conversations.address_shared_at`. Sharing again changes nothing
  and answers 200. There is no un-share *(Q-8, decided 2026-10-16)*, and a share covers that
  one conversation only *(Q-9, decided 2026-10-16)*.
- `Conversation.address_shared_at` (nullable), `Conversation.can_share_address`,
  and `Conversation.place` (`ConversationPlace`: `address_line`, `lat`, `lng`).
  `place` is non-null exactly when the address has been shared, the listing is
  not taken down and its owner is not suspended *(Q-3, decided 2026-10-16)*.
- Only the exact address is revealed: no email, no phone *(decided
  2026-10-16)*.
- Correcting the contract descriptions that call the address "public for now"
  or "an open product decision", and `ListingOwner`'s "contact details stay
  private until both sides agree to talk".

### Out of scope

- Any phone number or email reveal *(decided 2026-10-16)*.
- Revoking a share *(Q-8, decided 2026-10-16)*, and a listing-wide "share with everyone"
  setting *(Q-9, decided 2026-10-16)*.
- Geocoding or fuzzing coordinates. The maps feature decides that, when its
  provider is chosen *(per `docs/ROADMAP.md`, "Providers still to be chosen")*.
- Offers opening conversations: Phase 17. Until then an offer's listing is
  public-shaped, so the request's poster sees no address. That is correct:
  the owner has not shared it with them.
- Telling the enquirer by email that the address was shared: Phase 18.

---

## Dependencies / Prerequisites

- Phase 15 complete.
- Q-3, Q-7, Q-8 and Q-9 decided and recorded in `index.md`, Decisions (2026-10-16).
- **The frontend has stopped reading `address_line` from public reads.** Today
  it reads it in `src/components/HousingCard.tsx` (the photo placeholder
  label), `src/app/housing/Compare.tsx` and the location line in
  `src/lib/format.ts`. Drone deploys both repositories from `main`
  independently, so if this phase merges first those surfaces show
  "undefined". `/housing/mine` and the edit form read `/me/listings`, which
  keeps the field, so they are unaffected.
- This phase does **not** wait for the frontend messaging phase. Promotion does
  *(decided 2026-10-16)*, so hiding the street name before the frontend can show
  a shared address costs no promoted student anything.

---

## Implementation Steps

- [x] **Preparation**
  - [x] Confirm Phase 15 is complete and that the decisions on Q-3, Q-7, Q-8 and Q-9 are recorded in `index.md`
  - [ ] Confirm with the developer that the frontend change in Dependencies is on frontend `main` - **deferred to merge**: it is frontend pull request #19 (`fix/public-address`), open, not yet on `main`. Implementation did not depend on it; merging this phase does. Recorded as the merge order in `CLAUDE.md`
  - [x] Update `docs/specs/active/messaging/index.md` — set Phase 16 to In Progress

- [x] **Contract**
  - [x] In `api/openapi.yaml`, remove `address_line` from `Listing.required`, and rewrite its description: "Present only on your own listings and on the admin surface. Everyone else sees `neighbourhood` and `cross_streets`. The exact address reaches another student only when you share it in your conversation with them."
  - [x] Rewrite the `lat`/`lng` descriptions: null on public reads; part of the exact address
  - [x] Add `cross_streets` (string, max 80, e.g. "King St N & University Ave") to `Listing` (required, may be empty), `ListingInput` and `ListingUpdate` (optional)
  - [x] Change `ListingInput.address_line`'s description to say it is private and is the exact address, house number included
  - [x] Rewrite `ListingOwner`'s description: "Never their email. A listing's exact address reaches another student only when its owner shares it in a conversation."
  - [x] Add `ConversationPlace`, and `Conversation.place` (nullable), `Conversation.address_shared_at` (nullable date-time) and `Conversation.can_share_address` (boolean: the viewer is the owner, the conversation is open, the listing is available, and it has not been shared yet), each with a description of its condition
  - [x] Add `POST /conversations/{id}/share-address` (`shareAddress`, tag `messages`): no body; 200 `Conversation`; 401; 403 when the viewer is the enquirer ("Only the owner can share the address of their place."); 404 for a non-participant; 409 when the conversation is closed, the listing is not available, or the owner's `address_line` is empty. Its description says it cannot be undone *(Q-8, decided 2026-10-16)* and covers this conversation only *(Q-9, decided 2026-10-16)*
  - [x] Update the `messages` tag description: the exact address is shared by the owner, and by nothing else
  - [x] Update the `GET /listings` `q` description: it searches title, neighbourhood, cross streets and description, **not** the address
  - [x] Verify: generators regenerate cleanly and idempotently, and `gen.Listing.AddressLine` is now a pointer

- [x] **Schema**
  - [x] Add `migrations/000012_location_privacy.up.sql`:
    - `ALTER TABLE listings ADD COLUMN cross_streets varchar(80) NOT NULL DEFAULT ''`
    - drop and re-add the generated `search` column without `address_line`, with `cross_streets` at weight B. Re-create `listings_search_idx` (dropping the column drops its index)
    - `ALTER TABLE conversations ADD COLUMN address_shared_at timestamptz`. Nullable; set once by the owner. A comment saying nothing else may set it
    - a comment saying why the search column changes: a field that is hidden but still searchable can be confirmed by probing
  - [x] `.down.sql` restores the migration-4 expression, drops `cross_streets` and drops `address_shared_at`
  - [x] Update the comment on `address_line` in the new migration, since migration 3's "PUBLIC for now" comment cannot be edited
  - [x] Verify: migrate up, `down-one`, up. Existing rows keep their `address_line`

- [x] **Queries**
  - [x] `CreateListing` and `UpdateListing` in `internal/db/queries/listings.sql` take `cross_streets`. COALESCE works for it, because `''` is not NULL, so it can be cleared
  - [x] Add `ShareConversationAddress` to `internal/db/queries/conversations.sql`: `SET address_shared_at = now() WHERE id = $1 AND owner_id = $2 AND address_shared_at IS NULL AND closed_at IS NULL`. No row means already shared (the handler re-reads and answers 200) or refused (the handler says why)
  - [x] Add the reveal condition to `GetConversationForParticipant` and `ListConversationsForUser` as a `CASE` in the SQL. `place_address_line`, `place_lat` and `place_lng` are NULL unless `c.address_shared_at IS NOT NULL AND l.removed_at IS NULL AND owner.suspended_at IS NULL`. The condition lives in the SQL, as the published-only filter does, so no handler can forget it
  - [x] `dbtest.Listing` gains `CrossStreets`; `dbtest.Conversation` gains `AddressSharedAt`
  - [x] Verify: `sqlc generate`, idempotent

- [x] **Handlers and mapping**
  - [x] Change `toListing` in `internal/httpapi/mapping.go` to `toListing(row, owner, photos, exact bool)`, following `toUser(row, includeEmail)`. When `exact` is false, `AddressLine` is nil and `Lat`/`Lng` are null
  - [x] Set `exact` explicitly at every call site:
    - `false`: `listings.go` (`ListListings`, `GetListing`); `offers.go` (`ListOffers` and the `CreateOffer` response)
    - `true`: `listings_write.go` (`ListMyListings`, and the owner's create, update, status and delete responses); `admin_posts.go` (both admin listing reads)
  - [x] Map `cross_streets` in, out and through validation (`internal/httpapi/listings_validate.go`): trimmed, at most 80 characters
  - [x] Add `shareAddress` to `internal/httpapi/conversations.go`: verified; the owner only (403 for the enquirer, 404 for a stranger); 409 with a plain reason when closed, unavailable or the address is empty; otherwise 200 with the conversation, `place` filled
  - [x] Map `place`, `address_shared_at` and `can_share_address` in `toConversation`
  - [x] Verify: by hand. `GET /listings/{id}` has no `address_line`; `GET /me/listings` has it; `GET /listings?q=<an exact street>` no longer matches on it; a new conversation has `place: null` and stays that way after both sides send messages; the enquirer calling share gets 403; the owner sharing fills `place` for the enquirer; a second share answers 200 unchanged; the same enquirer's conversation about a different listing still has `place: null`; an admin taking the listing down nulls `place`, and restoring it brings it back

- [x] **Rate limiting**
  - [x] ~~None.~~ Revised during implementation (see Implementation Notes): `ShareAddressPerUser`, 20 per hour per user, on the allowlist, so `TestEveryLimitedRouteExists` covers it. Sharing is still bounded by the conversations the owner is already in; this is the backstop against a looping client

- [x] **Tests**
  - [x] Query tests: the rebuilt `search` does not match an address-only term; `cross_streets` does match; `ShareConversationAddress` sets once, refuses a non-owner, refuses a closed conversation; the conversation reads null the place when unshared, taken down or suspended
  - [x] Handler tests, in `internal/httpapi/listings_test.go` and a new `internal/httpapi/contact_privacy_test.go`:
    - a **regression guard**: decode the raw JSON of every public read (list, detail, offer list, offer create, inbox, conversation before sharing) and assert there is no `address_line` key and the coordinates are null
    - a **share-only guard**: starting, sending, closing and marking read never set `address_shared_at` or fill `place`
    - the owner and admin reads still carry the address
    - `place` appears only after the owner shares, only in that conversation, and no response anywhere carries an email or phone field
  - [x] Update the in-memory fakes for the new parameters and columns
  - [x] Verify: `go test ./...` passes with and without `PG_TEST_DSN`

- [x] **Documentation**
  - [x] `CLAUDE.md`, "Rules that bind": add "**The exact address is never on a public read, and only the owner reveals it.** `address_line` and coordinates come only from owner reads, admin reads and a conversation whose owner pressed share (`address_shared_at`); `toListing`'s `exact` flag is set explicitly at every call site. The search index does not contain the address. No email or phone is ever revealed." Under the decisions, replace "The current public address is an implementation gap" with what shipped
  - [x] `docs/ROADMAP.md`, "Address privacy": record that it shipped, and in which phase
  - [x] `docs/specs/INDEX.md`: "Decisions settled — 2026-10-14" no longer calls it a known gap
  - [x] `docs/api.md`: the privacy rule and the share endpoint, beside "Browsing is public by design"; the endpoint count (50 → 51)
  - [x] Verify: search the repository for "Public for now", "open product decision", "implementation gap" and "both sides agree", and correct each in live documents

- [x] **Frontend check (this repository's responsibility to confirm, not to fix)**
  - [ ] Confirm frontend `main` no longer reads `addressLine` on public surfaces before this merges - **deferred to merge**: frontend pull request #19 (`fix/public-address`) makes `addressLine` optional and stops reading it on public surfaces; it must merge and deploy first
  - [x] Record the follow-ups for that repository, as listed in `index.md`, "Frontend impact": regenerate types; reword "Phone and email stay hidden until you both agree to talk" and "exact address shared once you message"; the form's `address_line` hint and placeholder become private; add a cross-streets field; the owner's "Share exact address" action with a confirmation

- [x] **Final verification**
  - [x] `go build ./...` and `go vet ./...` pass
  - [x] All relevant tests pass, with and without a database
  - [x] Update the handoff documentation so another agent can pick up from the files alone: `CLAUDE.md` (what is built, what is in flight, what comes next, decisions and gotchas from this phase), `README.md`, `docs/specs/INDEX.md`, `docs/ROADMAP.md`, and any other document this phase made stale
  - [x] Update `index.md` — set Phase 16 to Complete
  - [x] Move `docs/specs/active/messaging/phase-16-contact-privacy.md` →
        `docs/specs/implemented/messaging/phase-16-contact-privacy.md`

---

## Completion Criteria

- [x] All checklist items completed and verified
- [x] No public read carries `address_line` or non-null coordinates, and a test fails if one does
- [x] `q` cannot match a listing on its exact address
- [x] Owner and admin reads still carry the exact address
- [x] Only the owner's share fills `place`, in that one conversation; no other action does, and a test fails if one does
- [x] A shared place is withheld while the listing is taken down or its owner suspended, and returns on reinstatement
- [x] No response reveals an email or phone number
- [ ] The frontend's public surfaces did not regress on deploy - **deferred to merge**: guaranteed by the merge order (frontend #19 first), checked after deploy
- [x] No regressions in related areas
- [x] Handoff documentation current: `CLAUDE.md`, `README.md`, `docs/specs/INDEX.md`
- [x] `index.md` phase status set to Complete
- [x] Phase spec doc moved to `docs/specs/implemented/messaging/`

---

## Implementation Notes

Completed 2026-10-16 on `feature/messaging-privacy`, stacked on
`feature/messaging` (pull request #17).

**Key files changed:**

- `api/openapi.yaml`: `Listing.address_line` leaves `required` and is
  owner-only; `cross_streets` added to `Listing` (required, may be empty),
  `ListingInput` and `ListingUpdate`; `lat`/`lng` described as part of the
  exact address; `ListingOwner`, the `messages` tag and both `q` descriptions
  (public and admin) rewritten; `ConversationPlace`; `Conversation.place`,
  `address_shared_at`, `can_share_address`; `POST
  /conversations/{id}/share-address` (`shareAddress`).
- `internal/httpapi/gen/server.gen.go`, `internal/db/sqlcgen/*`: regenerated.
  `gen.Listing.AddressLine` is now `*string` with `omitempty`, so a public read
  has no `address_line` key at all.
- `migrations/000012_location_privacy.up.sql` / `.down.sql`:
  `listings.cross_streets`; `search` rebuilt without `address_line` and with
  `cross_streets` at weight B, `listings_search_idx` re-created;
  `conversations.address_shared_at`; a `COMMENT ON COLUMN` replacing migration
  3's "PUBLIC for now".
- `internal/db/queries/listings.sql`: `CreateListing` and `UpdateListing` take
  `cross_streets`.
- `internal/db/queries/conversations.sql`: `ShareConversationAddress`; the
  reveal columns `place_address_line`, `place_lat`, `place_lng` on
  `GetConversationForParticipant` and `ListConversationsForUser`.
- `internal/httpapi/mapping.go`: `toListing(row, owner, photos, exact)`;
  `toConversation` maps `place`, `address_shared_at`, `can_share_address`.
- `internal/httpapi/listings.go`, `offers.go` (`exact` false);
  `listings_write.go`, `admin_posts.go` (`exact` true).
- `internal/httpapi/conversations.go`: `ShareAddress` and `shareRefusal`.
- `internal/httpapi/listings_validate.go`, `listings_write.go`:
  `cross_streets` trimmed, at most 80 characters, through create, edit and
  validation.
- `internal/ratelimit/policy.go`, `internal/httpapi/ratelimit_mw.go`:
  `ShareAddressPerUser` and its allowlist entry.
- `internal/db/dbtest/fixtures.go`: `ListingOptions` gains `AddressLine`,
  `CrossStreets`, `Lat`, `Lng`.
- Tests: `internal/db/queries_location_privacy_test.go` (4 tests),
  `internal/httpapi/contact_privacy_test.go` (8 tests), and the fakes in
  `fake_querier_test.go` and `fake_querier_conversations_test.go`.
- Documentation: `CLAUDE.md`, `README.md`, `docs/api.md`, `docs/admin.md`,
  `docs/ROADMAP.md`, `docs/specs/INDEX.md`, `index.md`.

**Divergences from plan:**

- **The reveal condition is a `LEFT JOIN`, not a `CASE`.** sqlc typed each
  `CASE ... END` column as `interface{}`. Joining `listings pl` a second time,
  `ON pl.id = c.listing_id AND c.address_shared_at IS NOT NULL AND
  l.removed_at IS NULL AND o.suspended_at IS NULL`, gives the same condition,
  in the SQL, with `*string`/`*float64` columns: the Phase 15 gotcha about
  left joins, applied deliberately. Recorded as a gotcha in `CLAUDE.md`.
- **`ShareConversationAddress` is `:execrows` and repeats every condition**,
  not only the four in the plan: the listing published and not taken down,
  its address non-blank, and neither participant suspended. The handler checks
  them first to say why; the query is the guarantee if something changes
  between the read and the write.
- **A rate limit on sharing.** The phase doc said none; the implementation
  brief for this phase asked for the new write endpoint to be rate limited on
  the allowlist. `ShareAddressPerUser` is 20 per hour per user. A re-share is a
  no-op that still costs, so a looping client meets it; no real owner does.
- **Share refusals beyond the plan's three.** Decided by Claude under the
  developer's delegation, 2026-10-16, taking the most conservative reading:
  sharing is also refused (409) when the **enquirer's** account is suspended
  ("This account is no longer available."), and when the listing is paused,
  archived or a draft, not only taken down (the plan's "not available"). The
  409 messages come from `shareRefusal`, which `can_share_address` also uses,
  so the flag and the endpoint cannot disagree.
- **`can_share_address` also requires the viewer to be verified**, as
  `can_send` does, because the endpoint refuses an unverified owner with 403.
  It does not check that the address is non-blank (the endpoint does, with
  409): every write path already refuses a blank address.
- **The order of checks in `shareAddress`:** a stranger gets 404 first, then
  the enquirer gets 403, then an unverified owner gets 403; then a
  conversation already shared answers 200 unchanged, even if it has since
  closed (sharing again is not a new share); then the 409s.
- **A shared place stays visible after the conversation is closed.** Closing
  un-shares nothing (Q-8: there is no un-share), and the spec's reveal
  condition does not mention closing. Decided by Claude under the developer's
  delegation, 2026-10-16.
- **The owner sees `place` too**, not only the enquirer: the spec's condition
  does not depend on the viewer, and the address is the owner's own. Both
  participants of that one conversation, and nobody else.
- **The admin listing search no longer matches the exact address**, because it
  reads the same `search` column. The admin `q` description says so.
- **`ListOffers` passes `exact` false even for the offering owner's own
  offers**, as the plan says; the owner's own address is on `/me/listings`.
- **One extra guard, `TestOnlyTheShareQuerySetsAddressSharedAt`**, reads the
  query sources and fails if any query other than `ShareConversationAddress`
  assigns or inserts `address_shared_at`.
- **The hand check ran against the API from source on port 8090** and a
  scratch database (`waterloostar_p16test`, migrated, then dropped), as in
  Phase 15: the Docker `api` container serves the frontend's tests, and
  recreating it would read the real `RESEND_API_KEY`.
- **The frontend precondition is deferred to merge, not to implementation.**
  Frontend pull request #19 (`fix/public-address`) stops reading
  `addressLine` on public surfaces and is open, not yet on `main`. This phase
  must not merge until it has merged and deployed.

**Verification run:**

- `go tool oapi-codegen -config api/oapi-codegen.yaml api/openapi.yaml` and
  `go tool sqlc generate`: clean; a second run is byte-identical (checksums
  compared).
- On `waterloostar_p16test`: `migrate goto 11`, one user and one listing at
  "742 Evergreen Terrace" inserted, `up` (version 12; the row keeps its
  address, `cross_streets` is `''`, and `search` no longer matches
  "Evergreen"), `down-one` (version 11; the address is searchable again),
  `up` (version 12; `listings_search_idx` present). All `ok`, never dirty.
  The database was dropped afterwards.
- `go build ./...` and `go vet ./...`: pass.
- `go test ./... -count=1` with `PG_TEST_DSN` pointing at the Docker
  PostgreSQL on 5433 (its `postgres` maintenance database): every package
  `ok`; 236 top-level tests pass, 0 skipped, 0 failed. That includes 58
  database tests in `internal/db` (4 new) and one in `cmd/admin`, and 8 new
  handler tests. `TestEveryQueryIsExercised` and `TestEveryLimitedRouteExists`
  pass.
- `go test ./... -count=1` without `PG_TEST_DSN`: every package `ok`; 177 pass
  and those 59 database tests skip.
- The race detector was not run: it needs a C toolchain this machine does not
  have (CI runs `-race` over `internal/ratelimit`).
- By hand, a script against the API from source (16 checks, all pass): the
  owner's create carries the address; `GET /listings/{id}` has no
  `address_line`, null coordinates and the cross streets; `GET /listings` has
  no `address_line`; `GET /me/listings` has it; `q=Evergreen` matches nothing
  and `q=University` matches the listing; a new conversation, and the same one
  after both sides send, has `place: null`; the owner's `can_share_address` is
  true; the enquirer sharing gets 403 with the envelope; the owner sharing
  fills `place` for both; a second share answers 200 with the same
  `address_shared_at`; the same enquirer's conversation about another listing
  has `place: null`; an admin takedown nulls `place` and keeps
  `address_shared_at`; restoring brings `place` back.
