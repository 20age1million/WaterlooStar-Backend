# Phase 15 — Conversations and messages

**Status:** Complete
**Feature:** [Messaging with Contact Privacy](../../active/messaging/index.md)
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

- [x] **Preparation**
  - [x] Confirm the decisions on Q-1 to Q-6 are recorded in `index.md`, Decisions
  - [x] Update `docs/specs/active/messaging/index.md` — set Phase 15 to In Progress

- [x] **Contract**
  - [x] Add a `messages` tag to `api/openapi.yaml` with a description: the two sides, participants only, either side can write and close, and the exact address is the owner's to share (Phase 16)
  - [x] Add schemas:
    - `Conversation`: `id`, `status` (`open | closed`), `listing` (`ConversationListing`), `counterpart` (`ListingOwner`), `viewer_is_owner`, `opened_by_viewer`, `can_send`, `last_message` (`MessagePreview`, nullable), `unread_count`, `created_at`, `closed_at` (nullable), `closed_by_viewer` (nullable boolean), `counterpart_available`
    - `ConversationListing`: `id`, `title`, `neighbourhood`, `price_cents`, `start_date`, `end_date`, `available`. **No address field**
    - `Message`: `id`, `conversation_id`, `from_viewer`, `body`, `created_at`
    - `MessagePreview`: `from_viewer`, `body` (cut to 140 characters), `created_at`
    - `MessageInput`: `body`, string, 1–4000 characters after trimming
    - `ConversationPage`, `MessagePage`: `data` + `PageMeta`
    - `UnreadCounts`: `conversations` (with any unread), `messages`
  - [x] Add the eight paths with their responses: 201/400/401/403/404/409/429 on start; 201/400/401/403/404/409/429 on send; 200/401/404 on close (closing a closed conversation answers 200 unchanged); 204/401/404 on read; 200/401/404 on the reads
  - [x] `GET /me/conversations` takes `page`, `per_page`, `status` (optional enum) and `side` (optional `owner | enquirer`)
  - [x] The 409 on start puts the existing conversation's id in `details.conversation_id`, so a client can open it. It says so whether that conversation is open or closed
  - [x] Verify: both generators regenerate cleanly and idempotently. `go build ./...` fails until the handlers exist, which is the point

- [x] **Schema**
  - [x] Add `migrations/000011_conversations.up.sql` / `.down.sql`:
    - `conversations`: `id` uuid PK; `listing_id` → `listings` ON DELETE CASCADE; `owner_id` → `users`, denormalised from the listing (a listing never changes hands, as on `request_offers`); `enquirer_id` → `users`; `opened_by` → `users`; `closed_at`, `closed_by` → `users` (both nullable); `owner_read_at`, `enquirer_read_at`; `created_at`, `updated_at` with the `set_updated_at` trigger
    - Status is derived, not stored: `closed_at IS NULL` is open. One column cannot disagree with another
    - `UNIQUE (listing_id, enquirer_id)`: one conversation per place per student
    - CHECK `owner_id <> enquirer_id`; CHECK `opened_by IN (owner_id, enquirer_id)`; CHECK `(closed_at IS NULL) = (closed_by IS NULL)`; CHECK `closed_by IS NULL OR closed_by IN (owner_id, enquirer_id)`
    - `messages`: `id` uuid PK; `conversation_id` → `conversations` ON DELETE CASCADE; `sender_id` → `users`; `body` text with CHECK `length(btrim(body)) BETWEEN 1 AND 4000`; `created_at`. No `updated_at`: messages are append-only *(Q-6, decided 2026-10-16)*
    - Indexes: `messages (conversation_id, created_at DESC)`; `conversations (owner_id)`; `conversations (enquirer_id)`
    - A file comment explaining why unread and last activity are computed rather than stored, and that opening a conversation reveals nothing (the address is shared by the owner, Phase 16)
  - [x] Verify: `go run ./cmd/migrate up`, then `down-one`, then `up` again, all clean

- [x] **Queries**
  - [x] Add `internal/db/queries/conversations.sql`:
    - `CreateConversation`, `CreateMessage`
    - `GetConversationByListingAndEnquirer`: for the 409
    - `GetConversationForParticipant`: by id **and** user. Returns no row unless the user is a participant, so the 404 lives in the SQL. It joins the listing, the counterpart, and the listing's public visibility (`status = 'published' AND removed_at IS NULL AND owner not suspended`) as `listing_available`
    - `ListConversationsForUser` / `CountConversationsForUser`: both sides, optional `status` and `side` filters. Last message through a lateral subquery; unread as a count of the other side's messages newer than the viewer's marker. Ordered by last activity (`coalesce(last message, created_at)`), newest first
    - `ListMessages` / `CountMessages`: newest first, `LIMIT`/`OFFSET`
    - `CloseConversation`: sets `closed_at = now()` and `closed_by` `WHERE id = $1 AND closed_at IS NULL`. No row means already closed, and the handler re-reads and answers 200
    - `MarkConversationRead`: sets `owner_read_at` or `enquirer_read_at` to `now()` by which participant is asking
    - `UnreadCountsForUser`
  - [x] Suspension *(Q-3, decided 2026-10-16)*: the inbox and the conversation reads do **not** hide a conversation whose counterpart is suspended. They return `counterpart_suspended`, and the handler turns it into `counterpart_available: false` and `can_send: false`. These are the participant's own reads, like `/me/...`, which deliberately do not filter *(per `CLAUDE.md`)*
  - [x] Add `dbtest.Conversation` and `dbtest.Message` fixtures in `internal/db/dbtest/fixtures.go`
  - [x] Verify: `sqlc generate` succeeds and is idempotent

- [x] **Handlers**
  - [x] Add `internal/httpapi/conversations.go`:
    - start: 401 if anonymous; 403 if unverified ("Confirm your uwaterloo.ca address before messaging an owner."); `GetPublishedListing`, 404 if missing; 400 if the listing is your own; 409 if a conversation exists. Insert the conversation and the first message in one `s.tx` transaction. A unique violation that slips past the check (`isUniqueViolation`) also answers 409
    - inbox, unread counts, one conversation
    - close: either participant; 200 with the conversation; already closed answers 200 unchanged. A non-participant gets 404
    - mark read: 204
  - [x] Add `internal/httpapi/messages.go`:
    - list: participants only
    - send: verified; conversation open; counterpart not suspended; listing not taken down *(Q-3, decided 2026-10-16)*. Otherwise 409 with a plain reason ("This conversation was closed.", "This account is no longer available.", "This listing is no longer available.")
  - [x] Add `toConversation` and `toMessage` to `internal/httpapi/mapping.go`. `counterpart` uses the `ListingOwner` shape and never an email. `from_viewer` and `closed_by_viewer` are computed, so a user id other than the counterpart's never needs to be exposed
  - [x] Trim message bodies and refuse an empty one with `validation_failed`, before the database CHECK would
  - [x] Verify: by hand against the running API (`docker compose stop api` first, per `CLAUDE.md`). Account A messages B's listing; B sees one unread conversation; A sends a follow-up; B replies; the unread counts move and clear; B closes it and neither can send (409); A starting again on the same listing gets 409 naming it; account C gets 404 on the conversation and its messages

- [x] **Rate limiting**
  - [x] Add to `internal/ratelimit/policy.go`, each with a comment saying what it protects:
    - `ConversationStartPerUser` — Burst 10, Window 2h. A student contacting places in an evening, not a script messaging every listing
    - `MessagePerUser` — Burst 60, Window 10m. A conversation at typing speed. With no accept step *(Q-1, decided 2026-10-16)*, this and closing are what bound an unwanted correspondent
  - [x] Add `POST /listings/:id/conversations` and `POST /conversations/:id/messages` to `limitedRoutes` in `internal/httpapi/ratelimit_mw.go`, `keyByUser`, with plain-English messages
  - [x] Close, read and the reads stay unlimited: each acts only on conversations the caller is already in
  - [x] Verify: `TestEveryLimitedRouteExists` passes with the new paths

- [x] **Tests**
  - [x] Add `internal/db/queries_conversations_test.go`: every new query against PostgreSQL. The unique pair; the closed-only-once guard on `CloseConversation`; the closed CHECKs; the participant filter returning no row for a stranger; unread counts from both sides; inbox ordering by last activity; a suspended counterpart flagged rather than hidden; a taken-down listing reported as unavailable
  - [x] Add the in-memory fake: `internal/httpapi/fake_querier_conversations_test.go`
  - [x] Add `internal/httpapi/conversations_test.go`: the start round trip; 401, 403, 404 (unpublished, removed, suspended owner) and 409 (open and closed); your own listing; both sides sending before any reply; close by either side, then sends refused; close by a stranger (404); read markers; the 429 on both limited routes
  - [x] Verify: `go test ./...` passes with `PG_TEST_DSN` set and unset. `TestEveryQueryIsExercised` passes

- [x] **Documentation**
  - [x] `docs/api.md`: the eight endpoints in the table, the count (42 → 50), the two new rows in the rate-limits table, and a short "Messages" section: either side writes and closes; opening a conversation reveals nothing
  - [x] `README.md`: anything it states about the endpoint surface or messaging
  - [x] `CLAUDE.md`: Phase 15 in the "Where things stand" table, the endpoint count, the branch and pull request in flight, and any gotcha found
  - [x] Verify: search the docs for "forty-two", "42", "once messaging ships" and "message the poster", and correct each

- [x] **Final verification**
  - [x] `go build ./...` and `go vet ./...` pass
  - [x] All relevant tests pass, with and without a database
  - [x] Update the handoff documentation so another agent can pick up from the files alone: `CLAUDE.md` (what is built, what is in flight, what comes next, decisions and gotchas from this phase), `README.md`, `docs/specs/INDEX.md` (messaging in Active, Phase 15 Complete), `docs/ROADMAP.md` status, and any other document this phase made stale
  - [x] Update `index.md` — set Phase 15 to Complete
  - [x] Move `docs/specs/active/messaging/phase-15-conversations.md` →
        `docs/specs/implemented/messaging/phase-15-conversations.md`

---

## Completion Criteria

- [x] All checklist items completed and verified
- [x] A verified student can open one conversation per listing they do not own, and only on a listing the public can see
- [x] Both sides can send from the first message on, until either closes it
- [x] Nothing can be sent after closing, and the enquirer cannot open a second conversation about the same listing
- [x] Unread counts are right from both sides and clear on read
- [x] A non-participant gets 404 on every conversation route
- [x] Starting and sending answer 429 past their allowances
- [x] No regressions in related areas
- [x] Handoff documentation current: `CLAUDE.md`, `README.md`, `docs/specs/INDEX.md`
- [x] `index.md` phase status set to Complete
- [x] Phase spec doc moved to `docs/specs/implemented/messaging/`

---

## Implementation Notes

Completed 2026-10-16 on `feature/messaging`.

**Key files changed:**

- `api/openapi.yaml`: the `messages` tag; eight operations
  (`startConversation`, `listMyConversations`, `getUnreadCounts`,
  `getConversation`, `listMessages`, `sendMessage`, `markConversationRead`,
  `closeConversation`); schemas `Conversation`, `ConversationListing`,
  `Message`, `MessagePreview`, `MessageInput`, `ConversationPage`,
  `MessagePage`, `UnreadCounts`.
- `internal/httpapi/gen/server.gen.go`, `internal/db/sqlcgen/*`: regenerated.
- `migrations/000011_conversations.up.sql` / `.down.sql`: `conversations` and
  `messages`, with the unique pair, the four CHECKs, the body-length CHECK and
  the three indexes.
- `internal/db/queries/conversations.sql`: eleven queries.
- `internal/db/dbtest/fixtures.go`: `Conversation` and `Message` fixtures.
- `internal/httpapi/conversations.go`: start, inbox, unread counts, one
  conversation, mark read, close.
- `internal/httpapi/messages.go`: list and send; `sendRefusal`, the one rule
  behind both the 409s and `can_send`; `messageBody`, the trim and length
  check.
- `internal/httpapi/mapping.go`: `conversationRow` (one shape for both
  participant reads), `toConversation`, `toMessage`, `preview`.
- `internal/ratelimit/policy.go`: `ConversationStartPerUser` (10 per 2 h) and
  `MessagePerUser` (60 per 10 min).
- `internal/httpapi/ratelimit_mw.go`: the two routes on the allowlist.
- Tests: `internal/db/queries_conversations_test.go` (9 tests),
  `internal/httpapi/fake_querier_conversations_test.go`,
  `internal/httpapi/conversations_test.go` (11 tests), and two fields on the
  fake in `internal/httpapi/fake_querier_test.go`.
- Documentation: `docs/api.md` (endpoints, count 42 → 50, a Messages section,
  two rate-limit rows), `README.md`, `CLAUDE.md`, `docs/ROADMAP.md`,
  `docs/specs/INDEX.md`, `index.md`.

**Divergences from plan:**

- **The last message is a `LEFT JOIN messages` on the newest message's id,
  not a lateral subquery.** sqlc typed the lateral subquery's columns as
  non-null, so a conversation with no messages would have failed to scan.
  Joining the table itself makes them pointers. Same result, same index.
  Recorded as a gotcha in `CLAUDE.md`.
- **A `listing_removed` column beside `listing_available`.** The spec's send
  rule is "listing not taken down" (Q-3), but `listing_available` is also
  false for a listing its owner paused or archived. Whether pausing or
  archiving should stop a conversation was not settled by the decisions, so it
  was **decided by Claude under delegation**, taking the conservative reading
  of Q-3: only a moderator's takedown (and a suspended counterpart) makes a
  conversation read-only. An owner who archives a place once it is let still
  needs to talk to the student taking it. `listing.available` still reports
  that the public can no longer see the listing.
- **`GET /me/conversations` answers 400 for an unknown `status` or `side`.**
  The generated binding accepts any string for an enum query parameter, so the
  handler checks it. The spec listed 200/401/404 for the reads.
- **The 140-character preview is cut in Go** (`preview` in `mapping.go`, by
  characters), not in the SQL, so the fake and the database cannot disagree
  about it.
- **The start's 400 for "your own listing" uses `validation_failed`**, the code
  the offers path uses for its semantic 400s.
- **An extra guard, `TestMessagesHaveNoUpdateOrDelete`**, mirrors the ledger's:
  it fails if any generated query edits or deletes a message (Q-6).
- **The hand check ran against the API from source on port 8090** and a
  scratch database (`waterloostar_msgtest`, migrated, then dropped), not by
  stopping the Docker `api` container: that container is serving the
  frontend's browser tests, and recreating it would read the real
  `RESEND_API_KEY` from `.env`.
- **Known gap, unchanged from the other write paths:** a suspended account's
  access token lives up to 15 minutes, and nothing in this phase checks the
  caller's own suspension. The counterpart's suspension is checked in the SQL.

**Verification run:**

- `go tool oapi-codegen -config api/oapi-codegen.yaml api/openapi.yaml` and
  `go tool sqlc generate`: clean, and a second run changes nothing (checksums
  compared).
- `go run ./cmd/migrate up`, `down-one`, `up` against `waterloostar_msgtest`:
  all `ok`, ending at version 11, not dirty.
- `go build ./...` and `go vet ./...`: pass.
- `go test ./... -count=1` with `PG_TEST_DSN` pointing at the Docker
  PostgreSQL on 5433: every package `ok`; 224 top-level tests pass, 0 skipped,
  0 failed. That includes the 54 database tests in `internal/db` (9 new) and
  one in `cmd/admin`, and 11 new handler tests. `TestEveryQueryIsExercised` and
  `TestEveryLimitedRouteExists` pass.
- `go test ./... -count=1` without `PG_TEST_DSN`: every package `ok`; 169 pass
  and those 55 database tests skip.
- The race detector was not run: it needs a C toolchain this machine does not
  have (CI runs `-race` over `internal/ratelimit`).
- By hand, against the API from source: A messages B's listing (201, no
  `address_line` or `email` in the body); B's unread is 1/1; A follows up
  before any reply (201) and B's unread is 1/2; B replies (201) and A's unread
  is 1/1; each marks read (204) and their counts clear; three messages, newest
  first; stranger C gets 404 on the conversation, its messages, sending and
  closing; B closes (200, `closed_by_viewer: true`); both sends answer 409; A
  starting again answers 409 naming the same conversation; A can still read
  it (200).
