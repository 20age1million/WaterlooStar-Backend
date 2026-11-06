# Phase 18 — Message notifications

**Status:** Ready
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

- [ ] **Preparation**
  - [ ] Confirm Phases 15–17 are complete and the decision on Q-11 is recorded in `index.md`
  - [ ] Update `docs/specs/active/messaging/index.md` — set Phase 18 to In Progress

- [ ] **Contract**
  - [ ] Add `email_notifications` (boolean) to `User`, shown only to the signed-in user on `GET /me`
  - [ ] Add `PATCH /me/preferences` (`updateMyPreferences`), with body `{ email_notifications: boolean }` → 200 `User`. Responses 400/401
  - [ ] Verify: generators regenerate cleanly and idempotently

- [ ] **Schema**
  - [ ] Add `migrations/000014_notification_preferences` up/down: `ALTER TABLE users ADD COLUMN email_notifications boolean NOT NULL DEFAULT true`
  - [ ] Verify: migrate up, `down-one`, up

- [ ] **Queries**
  - [ ] In `internal/db/queries/users.sql`: `SetEmailNotifications`
  - [ ] Have the conversation-start, share, send and offer paths return the recipient's email, username and preference, and (for send) the recipient's unread count in that conversation after the insert, so the handler needs no second read. Otherwise add `GetNotificationRecipient`
  - [ ] Verify: `sqlc generate`, idempotent

- [ ] **Email**
  - [ ] Add `ConversationOpenedMessage`, `OfferConversationMessage`, `AddressSharedMessage` and `ReplyWaitingMessage` to `internal/email/sender.go`, each taking the recipient, the sender's username, the listing title and the link
  - [ ] Message IDs come from `email.MessageID("conversation-opened", <conversation id bytes>)`, `"address-shared"` (conversation id) and `"reply-waiting"` (message id). A retried request then sends once, and the idempotency key derives from data that never leaves the service
  - [ ] Verify: unit tests on every builder assert there is no message body text and no `address_line` in the output

- [ ] **Handlers**
  - [ ] After the transaction commits, the start, share, send and offer paths send their notification. Never inside the transaction: a slow Resend must not hold locks
  - [ ] Send sends "Reply waiting" only when the recipient's unread count in that conversation is exactly one, i.e. this message
  - [ ] Skip the send if the recipient opted out, is suspended or is unverified
  - [ ] A failed send is logged with the conversation id and Resend's error, as `Register` does *(per `internal/httpapi/auth.go`)*. The response does not change
  - [ ] Add `internal/httpapi/me.go` (or extend the existing `/me` handler) for `PATCH /me/preferences`
  - [ ] Verify: by hand, with the `LogSender`. Start a conversation and see the opened log line; reply twice and see one reply line; read, reply again and see another; share and see the shared line; opt out and see nothing

- [ ] **Rate limiting**
  - [ ] Add to `internal/ratelimit/policy.go`:
    - `NotifyPerRecipient` — Burst 20, Window 24h. No one receives more than twenty notification emails a day, however many students write to them. The in-app inbox still shows everything
    - `NotifyGlobal` — set below Resend's daily cap, so notifications cannot use up the allowance verification and reset emails need
  - [ ] These are consulted with `Allow` in the handler, keyed on the recipient. They are **not** route limits, so they are not in `limitedRoutes`. Going over skips the email and never fails the request
  - [ ] Verify: a unit test on a fake clock shows the twenty-first notification to one recipient is skipped and the request still succeeds

- [ ] **Tests**
  - [ ] Query test for `SetEmailNotifications` (and `GetNotificationRecipient` if added)
  - [ ] Handler tests with a recording `email.Sender`: one email per event, to the right side; one reply email per unread spell; none when opted out or suspended; none past the cap; the 201/200 unchanged when the sender fails
  - [ ] Verify: `go test ./...` passes with and without `PG_TEST_DSN`. No test calls Resend *(per the Email Delivery constraints)*

- [ ] **Documentation**
  - [ ] `docs/api.md`: `PATCH /me/preferences`, and which events send email
  - [ ] `docs/setup.md`: nothing new to configure; notifications use `RESEND_API_KEY`, `EMAIL_FROM` and `APP_URL`. Say so
  - [ ] `README.md` and `CLAUDE.md`: the feature is complete
  - [ ] Verify: search for "Notification emails" and "come with the features that need them", and annotate them as shipped where the document is still live

- [ ] **Final verification**
  - [ ] `go build ./...` and `go vet ./...` pass
  - [ ] All relevant tests pass, with and without a database
  - [ ] Update the handoff documentation so another agent can pick up from the files alone: `CLAUDE.md` (what is built, what is in flight, what comes next, decisions and gotchas from this phase), `README.md`, `docs/specs/INDEX.md` (move Messaging from Active to Implemented), `docs/ROADMAP.md` (backend messaging is done, so promotion waits only on the frontend; backend: saves, views and questions are next), and any other document this phase made stale
  - [ ] Update `index.md` — set Phase 18 to Complete
  - [ ] Move `docs/specs/active/messaging/phase-18-message-notifications.md` →
        `docs/specs/implemented/messaging/phase-18-message-notifications.md`
  - [ ] *(Final phase only)* Move `docs/specs/active/messaging/index.md` →
        `docs/specs/implemented/messaging/index.md`

---

## Completion Criteria

- [ ] All checklist items completed and verified
- [ ] Opening a conversation, sharing an address and a first unread reply each send one email to the right side, with neither message text nor address
- [ ] Opting out stops them; the inbox is unaffected
- [ ] A failed or capped send never changes a response
- [ ] No regressions in related areas
- [ ] Handoff documentation current: `CLAUDE.md`, `README.md`, `docs/specs/INDEX.md`
- [ ] `index.md` phase status set to Complete
- [ ] Phase spec doc moved to `docs/specs/implemented/messaging/`
- [ ] *(Final phase only)* `index.md` moved to `docs/specs/implemented/messaging/`

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
