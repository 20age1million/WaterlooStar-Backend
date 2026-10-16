# Phase 17 — Offers open conversations

**Status:** Ready
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

- [ ] **Preparation**
  - [ ] Confirm Phases 15–16 are complete and the decision on Q-10 is recorded in `index.md`
  - [ ] Update `docs/specs/active/messaging/index.md` — set Phase 17 to In Progress

- [ ] **Contract**
  - [ ] Add `ConversationRef` (`id`, `status`, `poster_replied`) and `RequestOffer.conversation` to `api/openapi.yaml`. It is nullable, because the column is `ON DELETE SET NULL`, but after the backfill every live offer has one
  - [ ] Rewrite the `createOffer`, `listOffers` and `withdrawOffer` descriptions to say what happens to the conversation, and that an offer reveals no address. The `createOffer` 409 also covers "this student closed the conversation about this listing"
  - [ ] Update the `messages` tag description: an owner's offer opens a conversation with the request's poster
  - [ ] Verify: generators regenerate cleanly and idempotently

- [ ] **Schema**
  - [ ] Add `migrations/000013_offer_conversations.up.sql`:
    - `ALTER TABLE request_offers ADD COLUMN conversation_id uuid REFERENCES conversations (id) ON DELETE SET NULL`, with an index
    - backfill: for each offer where `withdrawn_at IS NULL`, `INSERT ... ON CONFLICT (listing_id, enquirer_id) DO NOTHING` a conversation (owner = offer owner, enquirer = request poster, opened by the owner, `created_at` = the offer's), then set every live offer's `conversation_id` to its pair's conversation. Insert each non-empty note as a message from the owner at the offer's `created_at`. `address_shared_at` stays NULL: the backfill shares nothing
  - [ ] `.down.sql`: drop the column. Comment that the backfilled conversations stay, because removing them would delete messages people wrote afterwards
  - [ ] Verify: migrate up against a database holding offers. Every live offer has a conversation, and none has a shared address. `down-one` and up again are clean

- [ ] **Queries**
  - [ ] In `internal/db/queries/conversations.sql`: `ReopenConversation` (clears `closed_at` and `closed_by` where `closed_by` is the given owner) *(Q-2, decided 2026-10-16)*
  - [ ] Reuse `GetConversationByListingAndEnquirer`, `CreateConversation` and `CreateMessage` from Phase 15
  - [ ] In `internal/db/queries/offers.sql`: `SetOfferConversation`. `ListOffersForRequest` also returns the conversation's id, whether it is closed, and whether the poster has sent a message in it (left join, `EXISTS`)
  - [ ] Verify: `sqlc generate`, idempotent

- [ ] **Handlers**
  - [ ] `CreateOffer` in `internal/httpapi/offers.go` runs in one `s.tx` transaction, with this rule for an existing conversation between the listing and the poster:

    | Existing conversation | Result |
    |---|---|
    | none | a new conversation, opened by the owner; the note, if any, is its first message |
    | open, whoever opened it | joined; the note, if any, is posted as a message |
    | closed by the owner | reopened, and joined *(Q-2, decided 2026-10-16)* |
    | closed by the poster | 409: "That student has closed the conversation about this listing." *(Q-2, decided 2026-10-16)* |

    No row of this table touches `address_shared_at`.
  - [ ] `WithdrawOffer` is unchanged apart from its description. The conversation stays as it is
  - [ ] Map `RequestOffer.conversation`
  - [ ] A conversation opened by an offer with an empty note has no messages. Its inbox row shows the offer as the last activity (`created_at`), and the client labels it from the offer
  - [ ] Verify: by hand. Owner B offers a listing on student A's request; A sees the offer with its `conversation` and `poster_replied: false`; A replies in the conversation and `poster_replied` becomes true; `place` is still null; B shares the address and A sees `place`; B offers a second listing and withdraws it, and its conversation stays readable with `place: null`; A closes a third offer's conversation and B offering that listing again gets 409

- [ ] **Tests**
  - [ ] Query tests for the new and changed queries, including the backfill run against seeded offers in `internal/db/queries_offers_test.go`
  - [ ] Handler tests in `internal/httpapi/offers_test.go`: every row of the table above; withdrawal leaving the conversation untouched; two offers sharing one conversation; no offer path setting `address_shared_at` (extends the Phase 16 share-only guard); the visibility rule from Phase 7 still holding (one owner cannot see another's offers or conversations)
  - [ ] Update `internal/httpapi/fake_querier_offers_test.go`
  - [ ] Verify: `go test ./...` passes with and without `PG_TEST_DSN`

- [ ] **Documentation**
  - [ ] `docs/api.md`: offers now open conversations; the poster answers an offer by replying; an offer reveals nothing
  - [ ] `docs/ROADMAP.md` and `CLAUDE.md`: answering an offer has shipped; remove "waits on messaging"
  - [ ] Verify: search for "No accept or decline", "cannot act on them yet", "accept an offer" and "waits on messaging", and update or annotate each in the documents that are still live (implemented specs are history and stay as they are)

- [ ] **Final verification**
  - [ ] `go build ./...` and `go vet ./...` pass
  - [ ] All relevant tests pass, with and without a database
  - [ ] Update the handoff documentation so another agent can pick up from the files alone: `CLAUDE.md` (what is built, what is in flight, what comes next, decisions and gotchas from this phase), `README.md`, `docs/specs/INDEX.md`, and any other document this phase made stale
  - [ ] Update `index.md` — set Phase 17 to Complete
  - [ ] Move `docs/specs/active/messaging/phase-17-offer-conversations.md` →
        `docs/specs/implemented/messaging/phase-17-offer-conversations.md`

---

## Completion Criteria

- [ ] All checklist items completed and verified
- [ ] Every live offer has a conversation, including offers made before this phase
- [ ] The poster can answer an offer in its conversation, and `poster_replied` reflects it
- [ ] No offer path, including the backfill, reveals the address; only the owner's share does
- [ ] Owners still cannot see each other's offers or conversations
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
