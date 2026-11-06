# Phase 18 — Message notifications

**Status:** Complete (2026-10-17, on `feature/messaging-notifications`)
**Feature:** [Messaging with Contact Privacy](./index.md)
**Objective:** A student learns by email that someone wants to talk to them, that an owner shared an address with them, and that a reply is waiting, without the email carrying the message or the address.

*(This whole phase follows Q-11, decided 2026-10-16. It is the final phase.)*

---

## Scope

### In scope

- Three notification emails, sent through the existing `email.Sender`
  *(per `docs/specs/implemented/email-delivery/index.md`)*:
  - **Conversation opened.** Sent to the side that did not open it. From a
    listing (Phase 15): "{username} wants to talk about {listing title}". From
    an offer (Phase 17): "{username} offered you {listing title}".
  - **Address shared.** Sent to the enquirer when the owner shares (Phase 16):
    "{username} shared the address of {listing title}". The email says the
    address is in the conversation. It does not contain it.
  - **Reply waiting.** Sent to the recipient of a message when it is the first
    one they have not read in that conversation: "{username} replied about
    {listing title}". Further messages send nothing until the recipient reads
    the conversation, so a busy thread sends at most one email per unread spell.
    A conversation's opening message is covered by "Conversation opened", not
    this.
- Body: who, which listing, and a link to `APP_URL/messages/{conversation id}`.
  **Never the message text. Never the address.** An inbox is not a private
  place, and a forwarded email would bypass the owner's choice to share.
- An opt-out: `users.email_notifications` (default true), and
  `PATCH /me/preferences` to change it *(Q-11, decided 2026-10-16)*.
- Caps, so the site cannot be used to flood someone's inbox and the mail bill
  stays bounded.

### Out of scope

- An email for every message, and digests.
- HTML email. Bodies stay plain text, as the existing ones are.
- An outbox with retries. Sending stays inside the request, which was the
  developer's decision for Email Delivery.
- Bounce handling, and Resend webhooks.
- Push and browser notifications.

---

## Dependencies / Prerequisites

- Phases 15–17 complete.
- Q-11 decided and recorded in `index.md`, Decisions (2026-10-16).
- Resend's free-tier daily cap checked against expected volume (`index.md`,
  Assumptions). The reply email is the one whose volume grows with use.

---

## Implementation Steps

- [x] **Preparation**
  - [x] Confirm Phases 15–17 are complete and the decision on Q-11 is recorded in `index.md`
  - [x] Update `docs/specs/active/messaging/index.md` — set Phase 18 to In Progress

- [x] **Contract**
  - [x] Add `email_notifications` (boolean) to `User`, shown only to the signed-in user on `GET /me`
  - [x] Add `PATCH /me/preferences` (`updateMyPreferences`), with body `{ email_notifications: boolean }` → 200 `User`. Responses 400/401
  - [x] Verify: generators regenerate cleanly and idempotently

- [x] **Schema**
  - [x] Add `migrations/000014_notification_preferences` up/down: `ALTER TABLE users ADD COLUMN email_notifications boolean NOT NULL DEFAULT true`
  - [x] Verify: migrate up, `down-one`, up

- [x] **Queries**
  - [x] In `internal/db/queries/users.sql`: `SetEmailNotifications`
  - [x] Have the conversation-start, share, send and offer paths return the recipient's email, username and preference, and (for send) the recipient's unread count in that conversation after the insert, so the handler needs no second read. Otherwise add `GetNotificationRecipient`
  - [x] Verify: `sqlc generate`, idempotent

- [x] **Email**
  - [x] Add `ConversationOpenedMessage`, `OfferConversationMessage`, `AddressSharedMessage` and `ReplyWaitingMessage` to `internal/email/sender.go`, each taking the recipient, the sender's username, the listing title and the link
  - [x] Message IDs come from `email.MessageID("conversation-opened", <conversation id bytes>)`, `"address-shared"` (conversation id) and `"reply-waiting"` (message id). A retried request then sends once, and the idempotency key derives from data that never leaves the service
  - [x] Verify: unit tests on every builder assert there is no message body text and no `address_line` in the output

- [x] **Handlers**
  - [x] After the transaction commits, the start, share, send and offer paths send their notification. Never inside the transaction: a slow Resend must not hold locks
  - [x] Send sends "Reply waiting" only when the recipient's unread count in that conversation is exactly one, i.e. this message
  - [x] Skip the send if the recipient opted out, is suspended or is unverified
  - [x] A failed send is logged with the conversation id and Resend's error, as `Register` does *(per `internal/httpapi/auth.go`)*. The response does not change
  - [x] Add `internal/httpapi/me.go` (or extend the existing `/me` handler) for `PATCH /me/preferences`
  - [x] Verify: by hand, with the `LogSender`. Start a conversation and see the opened log line; reply twice and see one reply line; read, reply again and see another; share and see the shared line; opt out and see nothing

- [x] **Rate limiting**
  - [x] Add to `internal/ratelimit/policy.go`:
    - `NotifyPerRecipient` — Burst 20, Window 24h. No one receives more than twenty notification emails a day, however many students write to them. The in-app inbox still shows everything
    - `NotifyGlobal` — set below Resend's daily cap, so notifications cannot use up the allowance verification and reset emails need
  - [x] These are consulted with `Allow` in the handler, keyed on the recipient. They are **not** route limits, so they are not in `limitedRoutes`. Going over skips the email and never fails the request
  - [x] Verify: a unit test on a fake clock shows the twenty-first notification to one recipient is skipped and the request still succeeds

- [x] **Tests**
  - [x] Query test for `SetEmailNotifications` (and `GetNotificationRecipient` if added)
  - [x] Handler tests with a recording `email.Sender`: one email per event, to the right side; one reply email per unread spell; none when opted out or suspended; none past the cap; the 201/200 unchanged when the sender fails
  - [x] Verify: `go test ./...` passes with and without `PG_TEST_DSN`. No test calls Resend *(per the Email Delivery constraints)*

- [x] **Documentation**
  - [x] `docs/api.md`: `PATCH /me/preferences`, and which events send email
  - [x] `docs/setup.md`: nothing new to configure; notifications use `RESEND_API_KEY`, `EMAIL_FROM` and `APP_URL`. Say so
  - [x] `README.md` and `CLAUDE.md`: the feature is complete
  - [x] Verify: search for "Notification emails" and "come with the features that need them", and annotate them as shipped where the document is still live

- [x] **Final verification**
  - [x] `go build ./...` and `go vet ./...` pass
  - [x] All relevant tests pass, with and without a database
  - [x] Update the handoff documentation so another agent can pick up from the files alone: `CLAUDE.md` (what is built, what is in flight, what comes next, decisions and gotchas from this phase), `README.md`, `docs/specs/INDEX.md` (move Messaging from Active to Implemented), `docs/ROADMAP.md` (backend messaging is done, so promotion waits only on the frontend; backend: saves, views and questions are next), and any other document this phase made stale
  - [x] Update `index.md` — set Phase 18 to Complete
  - [x] Move `docs/specs/active/messaging/phase-18-message-notifications.md` →
        `docs/specs/implemented/messaging/phase-18-message-notifications.md`
  - [x] *(Final phase only)* Move `docs/specs/active/messaging/index.md` →
        `docs/specs/implemented/messaging/index.md`

---

## Completion Criteria

- [x] All checklist items completed and verified
- [x] Opening a conversation, sharing an address and a first unread reply each send one email to the right side, with neither message text nor address
- [x] Opting out stops them; the inbox is unaffected
- [x] A failed or capped send never changes a response
- [x] No regressions in related areas
- [x] Handoff documentation current: `CLAUDE.md`, `README.md`, `docs/specs/INDEX.md`
- [x] `index.md` phase status set to Complete
- [x] Phase spec doc moved to `docs/specs/implemented/messaging/`
- [x] *(Final phase only)* `index.md` moved to `docs/specs/implemented/messaging/`

---

## Implementation Notes

Completed 2026-10-17 on `feature/messaging-notifications`, stacked on
`feature/messaging-offers` (pull request #19), which is stacked on
`feature/messaging-privacy` (#18) and `feature/messaging` (#17). This is the
feature's final phase.

**Key files changed:**

- `api/openapi.yaml`: `User.email_notifications` (optional, own record only,
  like `email`); `PATCH /me/preferences` (`updateMyPreferences`) with the new
  `PreferencesUpdate` schema, answering 200 `User`, 400, 401, 429, 500; the
  email behaviour in the `messages` tag and in the `startConversation`,
  `sendMessage`, `shareAddress` and `createOffer` descriptions. Additive only.
- `internal/httpapi/gen/server.gen.go`, `internal/db/sqlcgen/*`: regenerated.
- `migrations/000014_notification_preferences.up.sql` / `.down.sql`:
  `users.email_notifications boolean NOT NULL DEFAULT true`.
- `internal/db/queries/users.sql`: `SetEmailNotifications`.
- `internal/db/queries/conversations.sql`: `GetNotificationRecipient` - the
  other participant's address, verification, suspension and switch, the
  sender's username, the listing's title, and `unread_through_message`.
- `internal/email/sender.go`: `ConversationOpenedMessage`,
  `OfferConversationMessage`, `AddressSharedMessage`, `ReplyWaitingMessage`,
  each `(id, to, from, listingTitle, link)`, and `oneLine`, which keeps
  user-written text on one line in a subject.
- `internal/httpapi/notifications.go` (new): `notify`, `notificationMessage`,
  `WaitForNotifications`.
- `internal/httpapi/me.go` (new): `UpdateMyPreferences`.
- `internal/httpapi/conversations.go`, `messages.go`, `offers.go`: the
  notification after each write; `makeOffer` returns a `madeOffer` saying
  whether it opened the conversation and which message its note became.
- `internal/httpapi/mapping.go`: `toUser` sets `email_notifications` on the
  user's own record.
- `internal/httpapi/router.go`: `Server.notifications`, a `sync.WaitGroup`.
- `cmd/api/main.go`: shutdown waits for notification emails in flight, within
  the shutdown deadline; the log sender's startup line names notification
  emails too.
- `internal/ratelimit/policy.go`: `PreferencesPerUser` (20 per hour),
  `NotifyPerRecipient` (20 per 24 h) and `NotifyGlobal` (30 per 24 h), with the
  Resend budget arithmetic. `internal/httpapi/ratelimit_mw.go`: the
  preferences route on the allowlist; the two notification caps are not route
  limits and are not there.
- `internal/db/dbtest/dbtest.go`: `Staged.User` and `Staged.Suspend`, raw-SQL
  seeding for a database held at an earlier version (see divergences).
- Tests: `internal/email/notifications_test.go` (4),
  `internal/db/queries_notifications_test.go` (3),
  `internal/httpapi/notifications_test.go` (12),
  `internal/httpapi/fake_querier_notifications_test.go`. The harness in
  `auth_test.go` and `ratelimit_test.go` waits for background notifications
  after every request. `internal/db/queries_offers_test.go`'s staged backfill
  test seeds users through `Staged.User`.
- Documentation: `CLAUDE.md`, `README.md`, `docs/api.md` (a Notification
  emails section, the endpoint, the rate-limit row, count 51 to 52),
  `docs/setup.md`, `docs/development.md`, `docs/ROADMAP.md`,
  `docs/specs/INDEX.md`, `index.md` (moved to `implemented/` with this
  phase), the Email Delivery spec's out-of-scope line annotated as shipped,
  and each phase doc's Feature link.

**Which event sends which email:**

| Event | To, subject | Message ID |
|---|---|---|
| `POST /listings/{id}/conversations` | the owner: "{username} wants to talk about {title}" | `conversation-opened` over the conversation id |
| `POST /requests/{id}/offers` that opens a conversation | the poster: "{username} offered you {title}" | `conversation-opened` over the conversation id |
| `POST /conversations/{id}/share-address`, first share only | the enquirer: "{username} shared the address of {title}" | `address-shared` over the conversation id |
| `POST /conversations/{id}/messages`, or an offer's note posted in an existing conversation, when it is the first message the recipient has not read | the other participant: "{username} replied about {title}" | `reply-waiting` over the message id |

Skipped when the recipient opted out, is suspended or is unverified, or a cap
is reached; every skip is logged with the conversation and recipient ids.

**Resend budget.** Resend's free tier allows **100 emails a day and 3,000 a
month**, for everything the service sends (checked 2026-10-17 against
Resend's "Account quotas and limits" page). A token bucket of burst B
refilling over 24 hours lets through at most 2B in any 24 hours: a full
bucket, then a full refill. So `NotifyGlobal` (30) bounds notifications to at
most 60 in any 24 hours, leaving at least 40 of the 100 for verification and
reset links, and to about 960 in a 31-day month (30 + 30 x 31). One student
receives at most 20 at once and at most 40 in any 24 hours
(`NotifyPerRecipient`). A restart forgets the buckets and starts them full, so
a deploy can let through one extra burst that day. The account-mail routes
have their own allowances, which together already exceed 100 a day in the
worst case; that predates this phase and is unchanged.

**Divergences from plan:**

- **The send runs in the background, once the response is decided.** The plan
  kept sending "inside the request", as verification is, meaning no outbox and
  no retries; the implementation brief required that sending not block the
  request. `notify` reads the recipient and decides synchronously, after the
  transaction commits, then hands `Sender.Send` to a goroutine with its own
  15-second context. It must not reuse the request's context: gin pools and
  reuses its `*gin.Context` once the handler returns. There is still no outbox
  and no retry. A failure is logged with the conversation id, the recipient's
  user id and the sender's error (never the address, which `ResendSender`
  already redacts from its errors) and changes nothing.
  `Server.WaitForNotifications` lets shutdown and the tests wait for sends in
  flight.
- **One read, `GetNotificationRecipient`, rather than widening the four write
  queries.** The plan allowed it. It runs after the commit: one indexed read
  per notifying write.
- **The reply rule counts unread messages up to and including this one**
  (`unread_through_message`), not every unread message at the moment of the
  read. With "exactly one unread now", two messages sent together could each
  see the other and neither would email. This is the plan's rule ("the first
  one they have not read") made safe against that race.
- **Decided by Claude under the developer's delegation, 2026-10-16, taking the
  most conservative option** (fewest emails, nothing new revealed):
  - An offer that **joins** an existing conversation (open, or reopened after
    the owner had closed it) sends no "offered you" email. Its note is a
    message there and follows the reply rule; an offer with no note into an
    existing conversation sends nothing. "Offered you" is sent only when the
    offer opens the conversation, and it shares the `conversation-opened` ID,
    since a conversation is opened once.
  - Sharing again sends nothing (it changes nothing).
  - Any signed-in account can change the switch, verified or not: turning
    email off must never need anything first.
  - `email_notifications` is optional in `PreferencesUpdate`: a field left out
    is left as it is, so an empty body answers 200 unchanged. The plan's body
    `{ email_notifications: boolean }` works as written, and the endpoint stays
    a true PATCH if a second preference is added.
  - `NotifyGlobal` is **30 per 24 hours**. The plan said only "below Resend's
    daily cap"; 30 is the largest round number whose worst case (60 a day)
    leaves account mail at least 40 of the 100.
  - The global bucket is peeked before the recipient's is spent, so a
    service-wide refusal does not also cost the recipient a token.
  - The sender's own suspension is not checked: a suspended account's access
    token lives up to 15 minutes, the known gap Phase 15 recorded, and the
    recipient checks are the ones that protect anyone.
- **A rate limit on `PATCH /me/preferences`** (`PreferencesPerUser`, 20 per
  hour), which the plan did not list, at the implementation brief's request
  that every new write route be limited. It is on the allowlist, so
  `TestEveryLimitedRouteExists` covers it.
- **Every email ends with how to opt out** ("turn off email notifications in
  your account settings on WaterlooStar"). The frontend has no settings page
  yet; its messaging phase must add the switch.
- **Subjects are kept to one line.** `oneLine` collapses line breaks and
  control characters in the username and title and cuts a title at 80
  characters.
- **The Phase 17 backfill test broke and was fixed.** It holds a database at
  version 12 and seeded users through the generated queries, which read
  `users.*` as migration 14 leaves it. It now seeds and suspends users with
  raw SQL through `Staged.User` and `Staged.Suspend`. Recorded as a gotcha in
  `CLAUDE.md`: a staged test may use a generated query only on tables no later
  migration changes.
- **The hand check ran against the API from source on port 8090** and a
  scratch database (`waterloostar_p18test`, dropped afterwards), with
  `RESEND_API_KEY` unset and no `.env` in the worktree, as in Phases 15-17.
- **Found, not fixed (out of scope):** `POST /listings` without `bath_type`
  answers 500, because nothing refuses it before the database's
  `listings_bath_type_known` CHECK does. Found when the hand-check script
  first left the field out.

**Verification run:**

- `go tool oapi-codegen -config api/oapi-codegen.yaml api/openapi.yaml` and
  `go tool sqlc generate`: clean; a second run is byte-identical (checksums
  compared).
- On `waterloostar_p18test`: `migrate up` (version 14, not dirty;
  `email_notifications` is `boolean`, `NOT NULL`, default `true`),
  `down-one` (version 13, the column gone), `up` (version 14, not dirty).
  Dropped afterwards.
- `go build ./...` and `go vet ./...`: pass.
- `go test ./... -count=1` with `PG_TEST_DSN` pointing at the Docker
  PostgreSQL on 5433 (its `postgres` maintenance database): every package
  `ok`; 268 top-level tests pass, 0 skipped, 0 failed (19 new: 4 email, 3
  database, 12 handler). `TestEveryQueryIsExercised` and
  `TestEveryLimitedRouteExists` pass.
- `go test ./... -count=1` without `PG_TEST_DSN`: every package `ok`; 203
  pass and 65 database tests skip.
- The no-text, no-address rule is enforced three ways:
  `TestNotificationsTakeNoMessageTextOrAddress` (every builder's signature is
  the five strings and nothing else), `TestEveryNotificationBuilderIsChecked`
  (a new builder in `sender.go` fails until it is added to the checks) and
  `TestNotificationsCarryNoMessageTextOrAddress` in both `internal/email` and
  `internal/httpapi`; the latter runs all four events with marked message
  text, a marked offer note and the exact address, and finds none of them,
  and no `@`, in any email.
- Mutation check: disabling the suspended and unverified skips, and loosening
  the reply rule to "at least one unread", each made the handler tests fail.
- The race detector was not run: it needs a C toolchain this machine does not
  have (CI runs `-race` over `internal/ratelimit`).
- No test calls Resend: every handler test records email with
  `capturingMailer`, and the email-package tests call no sender.
- By hand, a script against the API from source with the log sender (the
  startup line read "email: no RESEND_API_KEY, so verification and reset links
  and notification emails are written to this log"; 11 checks, all pass): the
  enquirer starts a conversation and one "wants to talk" email is logged for
  the owner; the owner replies twice and one "replied" email is logged for the
  enquirer; the enquirer reads, the owner replies, and another is logged; the
  owner shares and one "shared the address" email is logged, without the
  address; the enquirer opts out (200, `email_notifications: false`), reads,
  and the owner's next message logs no email while the enquirer's unread count
  shows it, with "notification not sent: turned off" in the log; no logged
  notification carries the message text or the address.
