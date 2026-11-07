# Phase 17 — Offers open conversations

**Status:** Complete (2026-10-17, on `feature/messaging-offers`)
**Feature:** [Messaging with Contact Privacy](./index.md)
**Objective:** Making an offer opens a conversation with the request's poster, or joins the one they already have about that listing, so the poster can answer the offer there; the owner shares the address in it as in any other conversation.

*(This whole phase follows Q-10, decided 2026-10-16.)*

---

## Scope

### In scope

- `request_offers.conversation_id`. An offer points at the conversation it
  opened or joined. Several offers can point at one conversation: the same
  owner may offer the same listing against two requests by one student.
- `CreateOffer` becomes transactional. It writes the offer and either opens a
  conversation (opened by the owner, the poster as enquirer, about the offered
  listing) or joins the existing one. A non-empty note becomes a message from
  the owner.
- `RequestOffer.conversation` (`ConversationRef`: `id`, `status`,
  `poster_replied`), so a client can show "Reply" and link to the thread, and
  the Desk home can show offers still waiting on the student. `poster_replied`
  is true once the request's poster has sent a message in that conversation.
- A migration backfill: conversations for every live offer made before this
  phase.

### Out of scope

- Revealing anything. An offer is not a share. The poster sees the exact
  address only if the owner presses "Share exact address" in the conversation
  (Phase 16) *(decided 2026-10-16)*. There is no "share on offer" shortcut.
- An accept or decline status on offers, and any `/offers/{id}/accept` route
  *(Q-10, decided 2026-10-16)*. The poster answers by replying, or refuses by closing the
  conversation *(Q-2, decided 2026-10-16)*.
- Changing the conversation when an offer is withdrawn. The offer revealed
  nothing, so withdrawing it has nothing to take back, and the messages people
  wrote stay theirs.
- Changing the request when an offer is answered, for example pausing it or
  marking it filled. The poster can still pause it themselves.
- Notifying anyone by email: Phase 18.

---

## Dependencies / Prerequisites

- Phases 15 and 16 complete.
- Q-10 and Q-2 decided and recorded in `index.md`, Decisions (2026-10-16). The
  closed rows of the table below follow Q-2.

---

## Implementation Steps

- [x] **Preparation**
  - [x] Confirm Phases 15–16 are complete and the decision on Q-10 is recorded in `index.md`
  - [x] Update `docs/specs/active/messaging/index.md` — set Phase 17 to In Progress

- [x] **Contract**
  - [x] Add `ConversationRef` (`id`, `status`, `poster_replied`) and `RequestOffer.conversation` to `api/openapi.yaml`. It is nullable, because the column is `ON DELETE SET NULL`, but after the backfill every live offer has one
  - [x] Rewrite the `createOffer`, `listOffers` and `withdrawOffer` descriptions to say what happens to the conversation, and that an offer reveals no address. The `createOffer` 409 also covers "this student closed the conversation about this listing"
  - [x] Update the `messages` tag description: an owner's offer opens a conversation with the request's poster
  - [x] Verify: generators regenerate cleanly and idempotently

- [x] **Schema**
  - [x] Add `migrations/000013_offer_conversations.up.sql`:
    - `ALTER TABLE request_offers ADD COLUMN conversation_id uuid REFERENCES conversations (id) ON DELETE SET NULL`, with an index
    - backfill: for each offer where `withdrawn_at IS NULL`, `INSERT ... ON CONFLICT (listing_id, enquirer_id) DO NOTHING` a conversation (owner = offer owner, enquirer = request poster, opened by the owner, `created_at` = the offer's), then set every live offer's `conversation_id` to its pair's conversation. Insert each non-empty note as a message from the owner at the offer's `created_at`. `address_shared_at` stays NULL: the backfill shares nothing
  - [x] `.down.sql`: drop the column. Comment that the backfilled conversations stay, because removing them would delete messages people wrote afterwards
  - [x] Verify: migrate up against a database holding offers. Every live offer has a conversation, and none has a shared address. `down-one` and up again are clean

- [x] **Queries**
  - [x] In `internal/db/queries/conversations.sql`: `ReopenConversation` (clears `closed_at` and `closed_by` where `closed_by` is the given owner) *(Q-2, decided 2026-10-16)*
  - [x] Reuse `GetConversationByListingAndEnquirer`, `CreateConversation` and `CreateMessage` from Phase 15
  - [x] In `internal/db/queries/offers.sql`: `SetOfferConversation`. `ListOffersForRequest` also returns the conversation's id, whether it is closed, and whether the poster has sent a message in it (left join, `EXISTS`)
  - [x] Verify: `sqlc generate`, idempotent

- [x] **Handlers**
  - [x] `CreateOffer` in `internal/httpapi/offers.go` runs in one `s.tx` transaction, with this rule for an existing conversation between the listing and the poster:

    | Existing conversation | Result |
    |---|---|
    | none | a new conversation, opened by the owner; the note, if any, is its first message |
    | open, whoever opened it | joined; the note, if any, is posted as a message |
    | closed by the owner | reopened, and joined *(Q-2, decided 2026-10-16)* |
    | closed by the poster | 409: "That student has closed the conversation about this listing." *(Q-2, decided 2026-10-16)* |

    No row of this table touches `address_shared_at`.
  - [x] `WithdrawOffer` is unchanged apart from its description. The conversation stays as it is
  - [x] Map `RequestOffer.conversation`
  - [x] A conversation opened by an offer with an empty note has no messages. Its inbox row shows the offer as the last activity (`created_at`), and the client labels it from the offer
  - [x] Verify: by hand. Owner B offers a listing on student A's request; A sees the offer with its `conversation` and `poster_replied: false`; A replies in the conversation and `poster_replied` becomes true; `place` is still null; B shares the address and A sees `place`; B offers a second listing and withdraws it, and its conversation stays readable with `place: null`; A closes a third offer's conversation and B offering that listing again gets 409

- [x] **Tests**
  - [x] Query tests for the new and changed queries, including the backfill run against seeded offers in `internal/db/queries_offers_test.go`
  - [x] Handler tests in `internal/httpapi/offers_test.go`: every row of the table above; withdrawal leaving the conversation untouched; two offers sharing one conversation; no offer path setting `address_shared_at` (extends the Phase 16 share-only guard); the visibility rule from Phase 7 still holding (one owner cannot see another's offers or conversations)
  - [x] Update `internal/httpapi/fake_querier_offers_test.go`
  - [x] Verify: `go test ./...` passes with and without `PG_TEST_DSN`

- [x] **Documentation**
  - [x] `docs/api.md`: offers now open conversations; the poster answers an offer by replying; an offer reveals nothing
  - [x] `docs/ROADMAP.md` and `CLAUDE.md`: answering an offer has shipped; remove "waits on messaging"
  - [x] Verify: search for "No accept or decline", "cannot act on them yet", "accept an offer" and "waits on messaging", and update or annotate each in the documents that are still live (implemented specs are history and stay as they are)

- [x] **Final verification**
  - [x] `go build ./...` and `go vet ./...` pass
  - [x] All relevant tests pass, with and without a database
  - [x] Update the handoff documentation so another agent can pick up from the files alone: `CLAUDE.md` (what is built, what is in flight, what comes next, decisions and gotchas from this phase), `README.md`, `docs/specs/INDEX.md`, and any other document this phase made stale
  - [x] Update `index.md` — set Phase 17 to Complete
  - [x] Move `docs/specs/active/messaging/phase-17-offer-conversations.md` →
        `docs/specs/implemented/messaging/phase-17-offer-conversations.md`

---

## Completion Criteria

- [x] All checklist items completed and verified
- [x] Every live offer has a conversation, including offers made before this phase
- [x] The poster can answer an offer in its conversation, and `poster_replied` reflects it
- [x] No offer path, including the backfill, reveals the address; only the owner's share does
- [x] Owners still cannot see each other's offers or conversations
- [x] No regressions in related areas
- [x] Handoff documentation current: `CLAUDE.md`, `README.md`, `docs/specs/INDEX.md`
- [x] `index.md` phase status set to Complete
- [x] Phase spec doc moved to `docs/specs/implemented/messaging/`

---

## Implementation Notes

Completed 2026-10-17 on `feature/messaging-offers`, stacked on
`feature/messaging-privacy` (pull request #18), which is stacked on
`feature/messaging` (#17).

**Key files changed:**

- `api/openapi.yaml`: new schema `ConversationRef` (`id`, `status`,
  `poster_replied`); `RequestOffer.conversation` (required key, nullable);
  `RequestOffer` and `RequestOffer.note` descriptions; `createOffer`,
  `listOffers` and `withdrawOffer` descriptions; the `createOffer` 409
  description (also "the poster closed the conversation", with
  `details.conversation_id`); the `messages` tag; `Conversation.opened_by_viewer`
  (an owner-opened conversation came from an offer and may have no messages).
  Additive only: nothing removed, renamed or deprecated.
- `internal/httpapi/gen/server.gen.go`, `internal/db/sqlcgen/*`: regenerated.
  `RequestOffer.ConversationID` is `*uuid.UUID`.
- `migrations/000013_offer_conversations.up.sql` / `.down.sql`:
  `request_offers.conversation_id` (`ON DELETE SET NULL`, indexed) and the
  backfill; the down drops the column and keeps the conversations.
- `internal/db/queries/conversations.sql`: `ReopenConversation`.
- `internal/db/queries/offers.sql`: `SetOfferConversation`;
  `ListOffersForRequest` gains `conversation_closed_at` and `poster_replied`
  (a left join to `conversations`, an `EXISTS` over the poster's messages).
- `internal/httpapi/offers.go`: `CreateOffer` refuses an offer against your own
  request, then calls `makeOffer`, one `s.tx` transaction implementing the
  table above, retried once on a unique violation (two offers racing to open
  the same conversation); `toRequestOffer` and `conversationRef` map the
  offer; the create response is read back through `ListOffersForRequest`.
- `internal/db/dbtest/dbtest.go`: `NewAt(t, version)` and
  `Staged.MigrateTo`, a test database held at an earlier schema version so a
  test can seed rows before a migration; `New` shares its setup.
- Tests: `internal/db/queries_offers_test.go` (3 new: the offer's conversation
  columns, reopening, and the backfill run against seeded offers, including
  down-one and up again); `internal/httpapi/offers_test.go` (9 new);
  `internal/httpapi/contact_privacy_test.go` (`TestNoMigrationSetsAddressSharedAt`;
  the existing public-shape guard reordered so the offer joins the enquirer's
  conversation); the fakes in `fake_querier_offers_test.go` and
  `fake_querier_conversations_test.go`.
- Documentation: `CLAUDE.md`, `docs/api.md`, `docs/ROADMAP.md`,
  `docs/specs/INDEX.md`, `index.md`.

**Divergences from plan:**

- **The backfill leaves out offers hidden by moderation.** The plan said every
  offer with `withdrawn_at IS NULL`. Decided by Claude under the developer's
  delegation, 2026-10-16, taking the most conservative reading: an offer whose
  listing or request a moderator took down, or whose owner or poster is
  suspended, gets no conversation (`conversation_id` stays NULL). Opening one
  would put a suspended account's note into a student's inbox as a new
  message. Offers on listings their owner paused or archived are included, as
  Phase 15 decided for conversations. `RequestOffer.conversation` is nullable
  and says so.
- **You cannot offer against your own request** (400, `validation_failed`).
  Phase 7 never refused it; a conversation cannot be with yourself
  (`conversations_two_people`). The frontend never shows the offer panel to
  the poster, so nothing it does changes. Such old offers get no conversation
  from the backfill. Decided by Claude under the developer's delegation,
  2026-10-16.
- **The backfill posts no note into a closed conversation.** It joins it (the
  offer points at it, and reads `status: closed`) but writes nothing there:
  the live path refuses an offer into a conversation the poster closed, and
  the backfill does not get round that either. Blank notes post nothing. A
  note posted into an existing open conversation keeps the offer's time.
  Decided by Claude under the developer's delegation, 2026-10-16.
- **The backfill is idempotent.** The down migration keeps the backfilled
  messages, so the message insert skips a note already there; down-one and up
  again posts nothing twice. The plan did not ask for it.
- **The backfill uses a temporary table and drops it at the end.** sqlc reads
  the migrations too, and without the `DROP` it generated an `OfferBackfill`
  model. Recorded as a gotcha in `CLAUDE.md`.
- **The 409 for a poster-closed conversation carries
  `details.conversation_id`**, as the second-start 409 of Phase 15 does. The
  owner is a participant and may read it.
- **No new rate limit.** No new endpoint; an offer's note is a message bounded
  by the offer limit (30 per hour), not the message limit.
- **The create response is read back through `ListOffersForRequest`**, so it
  carries `poster_replied` (true when the offer joins a conversation the
  poster already wrote in). If the listing left public view in the instant
  after the write, it falls back to the offer row with `poster_replied: false`.
- **`listing` in `RequestOffer` stays a plain `$ref`.** Wrapping it to add a
  description would have changed the generated shape for no gain.
- **The hand check ran against the API from source on port 8090** and a
  scratch database (`waterloostar_p17test`, dropped afterwards), as in Phases
  15 and 16: the Docker `api` container serves the frontend's tests.

**Verification run:**

- `go tool oapi-codegen -config api/oapi-codegen.yaml api/openapi.yaml` and
  `go tool sqlc generate`: clean; a second run is byte-identical (checksums
  compared).
- On `waterloostar_p17test`: `migrate goto 12`, `cmd/seed`, then six offers
  inserted by SQL (one live with a note, one live with a blank note, one
  withdrawn, one against the owner's own request, one from an owner then
  suspended, one to a poster then suspended). `up` (version 13, not dirty):
  two conversations, both opened by the owner at the offer's time, none
  shared; one message (the non-blank note); the other four offers without a
  conversation. `down-one` (version 12): the column gone, the two
  conversations and the message kept. `up` again: the same two links, still
  one message. The database was dropped afterwards.
- `go build ./...` and `go vet ./...`: pass.
- `go test ./... -count=1` with `PG_TEST_DSN` pointing at the Docker
  PostgreSQL on 5433 (its `postgres` maintenance database): every package
  `ok`; 249 top-level tests pass, 0 skipped, 0 failed (13 new: 3 database, 10
  handler). `TestEveryQueryIsExercised` and `TestEveryLimitedRouteExists` pass.
- `go test ./... -count=1` without `PG_TEST_DSN`: every package `ok`; 187 pass
  and 62 database tests skip.
- The race detector was not run: it needs a C toolchain this machine does not
  have (CI runs `-race` over `internal/ratelimit`).
- By hand, a script against the API from source (20 checks, all pass): B
  offers a listing on A's request (201, `conversation` open, `poster_replied`
  false, no `address_line`); A sees the offer with the same conversation, the
  note as its last message and `place: null`; A replies and `poster_replied`
  turns true, `place` still null; B shares and A sees the exact address; B
  offers a second listing and withdraws it, and its conversation stays open
  and readable with `place: null`; a third, note-less offer opens an empty
  conversation; A closes it, and B offering that listing on A's second request
  gets 409 naming it; a third account sees none of B's offers and gets 404 on
  B's conversation; the offer backfilled by the migration carries a
  conversation.
