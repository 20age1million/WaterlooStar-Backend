# Phase 15 — Conversations and messages

**Status:** Ready
**Feature:** [Messaging with Contact Privacy](./index.md)
**Objective:** A verified student can open a conversation with a listing's owner, both sides exchange messages with unread counts, and either side can close it.

---

## Scope

### In scope

- `conversations` and `messages`, and their migration pair.
- Eight operations under a new `messages` tag:

  | Method | Path | operationId | Who |
  |---|---|---|---|
  | POST | `/listings/{id}/conversations` | `startConversation` | verified, not the owner |
  | GET | `/me/conversations` | `listMyConversations` | session |
  | GET | `/me/conversations/unread` | `getUnreadCounts` | session |
  | GET | `/conversations/{id}` | `getConversation` | participant |
  | GET | `/conversations/{id}/messages` | `listMessages` | participant |
  | POST | `/conversations/{id}/messages` | `sendMessage` | verified participant |
  | POST | `/conversations/{id}/read` | `markConversationRead` | participant |
  | POST | `/conversations/{id}/close` | `closeConversation` | participant *(Q-2, decided 2026-10-16)* |

- The send rules: either participant, from the first message on, while the
  conversation is open *(Q-1, decided 2026-10-16)*; nothing after it is closed.
- The suspension and takedown rules for existing conversations *(Q-3, decided 2026-10-16)*.
- Rate-limit rules and allowlist entries for starting and sending.

### Out of scope

- Hiding the address, and the owner sharing it: Phase 16. In this phase the
  conversation's listing summary carries no address. The public listing still
  does, unchanged.
- Any accept, confirm or decline step *(Q-1, decided 2026-10-16)*. Opening a conversation
  reveals nothing *(decided 2026-10-16)*, so there is nothing for one to gate.
- Offers: Phase 17.
- Email: Phase 18.
- Admin endpoints or ledger entries for conversations *(Q-5, decided 2026-10-16)*.
- Editing or deleting messages *(Q-6, decided 2026-10-16)*.
- Blocking a user across listings *(Q-2, decided 2026-10-16)*.
- Attachments, photos, typing indicators, push or real-time delivery.

---

## Dependencies / Prerequisites

- Q-1 to Q-6 decided and recorded in `index.md`, Decisions (2026-10-16).
- Phases 0–14 on `main`. Specifically: `GetPublishedListing` with its
  suspension and takedown filters (Phases 10–11); `db.TxRunner` (Phase 10);
  `internal/db/dbtest` and the query guard (Phase 5); the limiter (Phase 8).

---

## Implementation Steps

- [ ] **Preparation**
  - [ ] Confirm the decisions on Q-1 to Q-6 are recorded in `index.md`, Decisions
  - [ ] Update `docs/specs/active/messaging/index.md` — set Phase 15 to In Progress

- [ ] **Contract**
  - [ ] Add a `messages` tag to `api/openapi.yaml` with a description: the two sides, participants only, either side can write and close, and the exact address is the owner's to share (Phase 16)
  - [ ] Add schemas:
    - `Conversation`: `id`, `status` (`open | closed`), `listing` (`ConversationListing`), `counterpart` (`ListingOwner`), `viewer_is_owner`, `opened_by_viewer`, `can_send`, `last_message` (`MessagePreview`, nullable), `unread_count`, `created_at`, `closed_at` (nullable), `closed_by_viewer` (nullable boolean), `counterpart_available`
    - `ConversationListing`: `id`, `title`, `neighbourhood`, `price_cents`, `start_date`, `end_date`, `available`. **No address field**
    - `Message`: `id`, `conversation_id`, `from_viewer`, `body`, `created_at`
    - `MessagePreview`: `from_viewer`, `body` (cut to 140 characters), `created_at`
    - `MessageInput`: `body`, string, 1–4000 characters after trimming
    - `ConversationPage`, `MessagePage`: `data` + `PageMeta`
    - `UnreadCounts`: `conversations` (with any unread), `messages`
  - [ ] Add the eight paths with their responses: 201/400/401/403/404/409/429 on start; 201/400/401/403/404/409/429 on send; 200/401/404 on close (closing a closed conversation answers 200 unchanged); 204/401/404 on read; 200/401/404 on the reads
  - [ ] `GET /me/conversations` takes `page`, `per_page`, `status` (optional enum) and `side` (optional `owner | enquirer`)
  - [ ] The 409 on start puts the existing conversation's id in `details.conversation_id`, so a client can open it. It says so whether that conversation is open or closed
  - [ ] Verify: both generators regenerate cleanly and idempotently. `go build ./...` fails until the handlers exist, which is the point

- [ ] **Schema**
  - [ ] Add `migrations/000011_conversations.up.sql` / `.down.sql`:
    - `conversations`: `id` uuid PK; `listing_id` → `listings` ON DELETE CASCADE; `owner_id` → `users`, denormalised from the listing (a listing never changes hands, as on `request_offers`); `enquirer_id` → `users`; `opened_by` → `users`; `closed_at`, `closed_by` → `users` (both nullable); `owner_read_at`, `enquirer_read_at`; `created_at`, `updated_at` with the `set_updated_at` trigger
    - Status is derived, not stored: `closed_at IS NULL` is open. One column cannot disagree with another
    - `UNIQUE (listing_id, enquirer_id)`: one conversation per place per student
    - CHECK `owner_id <> enquirer_id`; CHECK `opened_by IN (owner_id, enquirer_id)`; CHECK `(closed_at IS NULL) = (closed_by IS NULL)`; CHECK `closed_by IS NULL OR closed_by IN (owner_id, enquirer_id)`
    - `messages`: `id` uuid PK; `conversation_id` → `conversations` ON DELETE CASCADE; `sender_id` → `users`; `body` text with CHECK `length(btrim(body)) BETWEEN 1 AND 4000`; `created_at`. No `updated_at`: messages are append-only *(Q-6, decided 2026-10-16)*
    - Indexes: `messages (conversation_id, created_at DESC)`; `conversations (owner_id)`; `conversations (enquirer_id)`
    - A file comment explaining why unread and last activity are computed rather than stored, and that opening a conversation reveals nothing (the address is shared by the owner, Phase 16)
  - [ ] Verify: `go run ./cmd/migrate up`, then `down-one`, then `up` again, all clean

- [ ] **Queries**
  - [ ] Add `internal/db/queries/conversations.sql`:
    - `CreateConversation`, `CreateMessage`
    - `GetConversationByListingAndEnquirer`: for the 409
    - `GetConversationForParticipant`: by id **and** user. Returns no row unless the user is a participant, so the 404 lives in the SQL. It joins the listing, the counterpart, and the listing's public visibility (`status = 'published' AND removed_at IS NULL AND owner not suspended`) as `listing_available`
    - `ListConversationsForUser` / `CountConversationsForUser`: both sides, optional `status` and `side` filters. Last message through a lateral subquery; unread as a count of the other side's messages newer than the viewer's marker. Ordered by last activity (`coalesce(last message, created_at)`), newest first
    - `ListMessages` / `CountMessages`: newest first, `LIMIT`/`OFFSET`
    - `CloseConversation`: sets `closed_at = now()` and `closed_by` `WHERE id = $1 AND closed_at IS NULL`. No row means already closed, and the handler re-reads and answers 200
    - `MarkConversationRead`: sets `owner_read_at` or `enquirer_read_at` to `now()` by which participant is asking
    - `UnreadCountsForUser`
  - [ ] Suspension *(Q-3, decided 2026-10-16)*: the inbox and the conversation reads do **not** hide a conversation whose counterpart is suspended. They return `counterpart_suspended`, and the handler turns it into `counterpart_available: false` and `can_send: false`. These are the participant's own reads, like `/me/...`, which deliberately do not filter *(per `CLAUDE.md`)*
  - [ ] Add `dbtest.Conversation` and `dbtest.Message` fixtures in `internal/db/dbtest/fixtures.go`
  - [ ] Verify: `sqlc generate` succeeds and is idempotent

- [ ] **Handlers**
  - [ ] Add `internal/httpapi/conversations.go`:
    - start: 401 if anonymous; 403 if unverified ("Confirm your uwaterloo.ca address before messaging an owner."); `GetPublishedListing`, 404 if missing; 400 if the listing is your own; 409 if a conversation exists. Insert the conversation and the first message in one `s.tx` transaction. A unique violation that slips past the check (`isUniqueViolation`) also answers 409
    - inbox, unread counts, one conversation
    - close: either participant; 200 with the conversation; already closed answers 200 unchanged. A non-participant gets 404
    - mark read: 204
  - [ ] Add `internal/httpapi/messages.go`:
    - list: participants only
    - send: verified; conversation open; counterpart not suspended; listing not taken down *(Q-3, decided 2026-10-16)*. Otherwise 409 with a plain reason ("This conversation was closed.", "This account is no longer available.", "This listing is no longer available.")
  - [ ] Add `toConversation` and `toMessage` to `internal/httpapi/mapping.go`. `counterpart` uses the `ListingOwner` shape and never an email. `from_viewer` and `closed_by_viewer` are computed, so a user id other than the counterpart's never needs to be exposed
  - [ ] Trim message bodies and refuse an empty one with `validation_failed`, before the database CHECK would
  - [ ] Verify: by hand against the running API (`docker compose stop api` first, per `CLAUDE.md`). Account A messages B's listing; B sees one unread conversation; A sends a follow-up; B replies; the unread counts move and clear; B closes it and neither can send (409); A starting again on the same listing gets 409 naming it; account C gets 404 on the conversation and its messages

- [ ] **Rate limiting**
  - [ ] Add to `internal/ratelimit/policy.go`, each with a comment saying what it protects:
    - `ConversationStartPerUser` — Burst 10, Window 2h. A student contacting places in an evening, not a script messaging every listing
    - `MessagePerUser` — Burst 60, Window 10m. A conversation at typing speed. With no accept step *(Q-1, decided 2026-10-16)*, this and closing are what bound an unwanted correspondent
  - [ ] Add `POST /listings/:id/conversations` and `POST /conversations/:id/messages` to `limitedRoutes` in `internal/httpapi/ratelimit_mw.go`, `keyByUser`, with plain-English messages
  - [ ] Close, read and the reads stay unlimited: each acts only on conversations the caller is already in
  - [ ] Verify: `TestEveryLimitedRouteExists` passes with the new paths

- [ ] **Tests**
  - [ ] Add `internal/db/queries_conversations_test.go`: every new query against PostgreSQL. The unique pair; the closed-only-once guard on `CloseConversation`; the closed CHECKs; the participant filter returning no row for a stranger; unread counts from both sides; inbox ordering by last activity; a suspended counterpart flagged rather than hidden; a taken-down listing reported as unavailable
  - [ ] Add the in-memory fake: `internal/httpapi/fake_querier_conversations_test.go`
  - [ ] Add `internal/httpapi/conversations_test.go`: the start round trip; 401, 403, 404 (unpublished, removed, suspended owner) and 409 (open and closed); your own listing; both sides sending before any reply; close by either side, then sends refused; close by a stranger (404); read markers; the 429 on both limited routes
  - [ ] Verify: `go test ./...` passes with `PG_TEST_DSN` set and unset. `TestEveryQueryIsExercised` passes

- [ ] **Documentation**
  - [ ] `docs/api.md`: the eight endpoints in the table, the count (42 → 50), the two new rows in the rate-limits table, and a short "Messages" section: either side writes and closes; opening a conversation reveals nothing
  - [ ] `README.md`: anything it states about the endpoint surface or messaging
  - [ ] `CLAUDE.md`: Phase 15 in the "Where things stand" table, the endpoint count, the branch and pull request in flight, and any gotcha found
  - [ ] Verify: search the docs for "forty-two", "42", "once messaging ships" and "message the poster", and correct each

- [ ] **Final verification**
  - [ ] `go build ./...` and `go vet ./...` pass
  - [ ] All relevant tests pass, with and without a database
  - [ ] Update the handoff documentation so another agent can pick up from the files alone: `CLAUDE.md` (what is built, what is in flight, what comes next, decisions and gotchas from this phase), `README.md`, `docs/specs/INDEX.md` (messaging in Active, Phase 15 Complete), `docs/ROADMAP.md` status, and any other document this phase made stale
  - [ ] Update `index.md` — set Phase 15 to Complete
  - [ ] Move `docs/specs/active/messaging/phase-15-conversations.md` →
        `docs/specs/implemented/messaging/phase-15-conversations.md`

---

## Completion Criteria

- [ ] All checklist items completed and verified
- [ ] A verified student can open one conversation per listing they do not own, and only on a listing the public can see
- [ ] Both sides can send from the first message on, until either closes it
- [ ] Nothing can be sent after closing, and the enquirer cannot open a second conversation about the same listing
- [ ] Unread counts are right from both sides and clear on read
- [ ] A non-participant gets 404 on every conversation route
- [ ] Starting and sending answer 429 past their allowances
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
