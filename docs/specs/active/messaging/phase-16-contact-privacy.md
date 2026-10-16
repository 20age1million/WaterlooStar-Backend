# Phase 16 — Contact privacy and address sharing

**Status:** Ready
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

- [ ] **Preparation**
  - [ ] Confirm Phase 15 is complete and that the decisions on Q-3, Q-7, Q-8 and Q-9 are recorded in `index.md`
  - [ ] Confirm with the developer that the frontend change in Dependencies is on frontend `main`
  - [ ] Update `docs/specs/active/messaging/index.md` — set Phase 16 to In Progress

- [ ] **Contract**
  - [ ] In `api/openapi.yaml`, remove `address_line` from `Listing.required`, and rewrite its description: "Present only on your own listings and on the admin surface. Everyone else sees `neighbourhood` and `cross_streets`. The exact address reaches another student only when you share it in your conversation with them."
  - [ ] Rewrite the `lat`/`lng` descriptions: null on public reads; part of the exact address
  - [ ] Add `cross_streets` (string, max 80, e.g. "King St N & University Ave") to `Listing` (required, may be empty), `ListingInput` and `ListingUpdate` (optional)
  - [ ] Change `ListingInput.address_line`'s description to say it is private and is the exact address, house number included
  - [ ] Rewrite `ListingOwner`'s description: "Never their email. A listing's exact address reaches another student only when its owner shares it in a conversation."
  - [ ] Add `ConversationPlace`, and `Conversation.place` (nullable), `Conversation.address_shared_at` (nullable date-time) and `Conversation.can_share_address` (boolean: the viewer is the owner, the conversation is open, the listing is available, and it has not been shared yet), each with a description of its condition
  - [ ] Add `POST /conversations/{id}/share-address` (`shareAddress`, tag `messages`): no body; 200 `Conversation`; 401; 403 when the viewer is the enquirer ("Only the owner can share the address of their place."); 404 for a non-participant; 409 when the conversation is closed, the listing is not available, or the owner's `address_line` is empty. Its description says it cannot be undone *(Q-8, decided 2026-10-16)* and covers this conversation only *(Q-9, decided 2026-10-16)*
  - [ ] Update the `messages` tag description: the exact address is shared by the owner, and by nothing else
  - [ ] Update the `GET /listings` `q` description: it searches title, neighbourhood, cross streets and description, **not** the address
  - [ ] Verify: generators regenerate cleanly and idempotently, and `gen.Listing.AddressLine` is now a pointer

- [ ] **Schema**
  - [ ] Add `migrations/000012_location_privacy.up.sql`:
    - `ALTER TABLE listings ADD COLUMN cross_streets varchar(80) NOT NULL DEFAULT ''`
    - drop and re-add the generated `search` column without `address_line`, with `cross_streets` at weight B. Re-create `listings_search_idx` (dropping the column drops its index)
    - `ALTER TABLE conversations ADD COLUMN address_shared_at timestamptz`. Nullable; set once by the owner. A comment saying nothing else may set it
    - a comment saying why the search column changes: a field that is hidden but still searchable can be confirmed by probing
  - [ ] `.down.sql` restores the migration-4 expression, drops `cross_streets` and drops `address_shared_at`
  - [ ] Update the comment on `address_line` in the new migration, since migration 3's "PUBLIC for now" comment cannot be edited
  - [ ] Verify: migrate up, `down-one`, up. Existing rows keep their `address_line`

- [ ] **Queries**
  - [ ] `CreateListing` and `UpdateListing` in `internal/db/queries/listings.sql` take `cross_streets`. COALESCE works for it, because `''` is not NULL, so it can be cleared
  - [ ] Add `ShareConversationAddress` to `internal/db/queries/conversations.sql`: `SET address_shared_at = now() WHERE id = $1 AND owner_id = $2 AND address_shared_at IS NULL AND closed_at IS NULL`. No row means already shared (the handler re-reads and answers 200) or refused (the handler says why)
  - [ ] Add the reveal condition to `GetConversationForParticipant` and `ListConversationsForUser` as a `CASE` in the SQL. `place_address_line`, `place_lat` and `place_lng` are NULL unless `c.address_shared_at IS NOT NULL AND l.removed_at IS NULL AND owner.suspended_at IS NULL`. The condition lives in the SQL, as the published-only filter does, so no handler can forget it
  - [ ] `dbtest.Listing` gains `CrossStreets`; `dbtest.Conversation` gains `AddressSharedAt`
  - [ ] Verify: `sqlc generate`, idempotent

- [ ] **Handlers and mapping**
  - [ ] Change `toListing` in `internal/httpapi/mapping.go` to `toListing(row, owner, photos, exact bool)`, following `toUser(row, includeEmail)`. When `exact` is false, `AddressLine` is nil and `Lat`/`Lng` are null
  - [ ] Set `exact` explicitly at every call site:
    - `false`: `listings.go` (`ListListings`, `GetListing`); `offers.go` (`ListOffers` and the `CreateOffer` response)
    - `true`: `listings_write.go` (`ListMyListings`, and the owner's create, update, status and delete responses); `admin_posts.go` (both admin listing reads)
  - [ ] Map `cross_streets` in, out and through validation (`internal/httpapi/listings_validate.go`): trimmed, at most 80 characters
  - [ ] Add `shareAddress` to `internal/httpapi/conversations.go`: verified; the owner only (403 for the enquirer, 404 for a stranger); 409 with a plain reason when closed, unavailable or the address is empty; otherwise 200 with the conversation, `place` filled
  - [ ] Map `place`, `address_shared_at` and `can_share_address` in `toConversation`
  - [ ] Verify: by hand. `GET /listings/{id}` has no `address_line`; `GET /me/listings` has it; `GET /listings?q=<an exact street>` no longer matches on it; a new conversation has `place: null` and stays that way after both sides send messages; the enquirer calling share gets 403; the owner sharing fills `place` for the enquirer; a second share answers 200 unchanged; the same enquirer's conversation about a different listing still has `place: null`; an admin taking the listing down nulls `place`, and restoring it brings it back

- [ ] **Rate limiting**
  - [ ] None. Sharing is bounded by the conversations the owner is already in, and is a once-per-conversation write

- [ ] **Tests**
  - [ ] Query tests: the rebuilt `search` does not match an address-only term; `cross_streets` does match; `ShareConversationAddress` sets once, refuses a non-owner, refuses a closed conversation; the conversation reads null the place when unshared, taken down or suspended
  - [ ] Handler tests, in `internal/httpapi/listings_test.go` and a new `internal/httpapi/contact_privacy_test.go`:
    - a **regression guard**: decode the raw JSON of every public read (list, detail, offer list, offer create, inbox, conversation before sharing) and assert there is no `address_line` key and the coordinates are null
    - a **share-only guard**: starting, sending, closing and marking read never set `address_shared_at` or fill `place`
    - the owner and admin reads still carry the address
    - `place` appears only after the owner shares, only in that conversation, and no response anywhere carries an email or phone field
  - [ ] Update the in-memory fakes for the new parameters and columns
  - [ ] Verify: `go test ./...` passes with and without `PG_TEST_DSN`

- [ ] **Documentation**
  - [ ] `CLAUDE.md`, "Rules that bind": add "**The exact address is never on a public read, and only the owner reveals it.** `address_line` and coordinates come only from owner reads, admin reads and a conversation whose owner pressed share (`address_shared_at`); `toListing`'s `exact` flag is set explicitly at every call site. The search index does not contain the address. No email or phone is ever revealed." Under the decisions, replace "The current public address is an implementation gap" with what shipped
  - [ ] `docs/ROADMAP.md`, "Address privacy": record that it shipped, and in which phase
  - [ ] `docs/specs/INDEX.md`: "Decisions settled — 2026-10-14" no longer calls it a known gap
  - [ ] `docs/api.md`: the privacy rule and the share endpoint, beside "Browsing is public by design"; the endpoint count (50 → 51)
  - [ ] Verify: search the repository for "Public for now", "open product decision", "implementation gap" and "both sides agree", and correct each in live documents

- [ ] **Frontend check (this repository's responsibility to confirm, not to fix)**
  - [ ] Confirm frontend `main` no longer reads `addressLine` on public surfaces before this merges
  - [ ] Record the follow-ups for that repository, as listed in `index.md`, "Frontend impact": regenerate types; reword "Phone and email stay hidden until you both agree to talk" and "exact address shared once you message"; the form's `address_line` hint and placeholder become private; add a cross-streets field; the owner's "Share exact address" action with a confirmation

- [ ] **Final verification**
  - [ ] `go build ./...` and `go vet ./...` pass
  - [ ] All relevant tests pass, with and without a database
  - [ ] Update the handoff documentation so another agent can pick up from the files alone: `CLAUDE.md` (what is built, what is in flight, what comes next, decisions and gotchas from this phase), `README.md`, `docs/specs/INDEX.md`, `docs/ROADMAP.md`, and any other document this phase made stale
  - [ ] Update `index.md` — set Phase 16 to Complete
  - [ ] Move `docs/specs/active/messaging/phase-16-contact-privacy.md` →
        `docs/specs/implemented/messaging/phase-16-contact-privacy.md`

---

## Completion Criteria

- [ ] All checklist items completed and verified
- [ ] No public read carries `address_line` or non-null coordinates, and a test fails if one does
- [ ] `q` cannot match a listing on its exact address
- [ ] Owner and admin reads still carry the exact address
- [ ] Only the owner's share fills `place`, in that one conversation; no other action does, and a test fails if one does
- [ ] A shared place is withheld while the listing is taken down or its owner suspended, and returns on reinstatement
- [ ] No response reveals an email or phone number
- [ ] The frontend's public surfaces did not regress on deploy
- [ ] No regressions in related areas
- [ ] Handoff documentation current: `CLAUDE.md`, `README.md`, `docs/specs/INDEX.md`
- [ ] `index.md` phase status set to Complete
- [ ] Phase spec doc moved to `docs/specs/implemented/messaging/`

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
