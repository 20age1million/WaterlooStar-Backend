# Phase 13 — Resend sender

**Status:** Complete
**Feature:** [Email Delivery](./index.md)
**Objective:** Verification and password-reset emails are delivered through Resend when a key is configured, and the log stays the sender when one is not.

---

## Scope

### In scope

- `ResendSender` in `internal/email/resend.go`, implementing `Sender`:
  - It calls Resend's public email API directly with `net/http`
    (`POST https://api.resend.com/emails`). No SDK: it is one documented call,
    and doing it ourselves keeps control of what an error may contain.
  - From is the configured mailbox. The recipient, the subject and a
    plain-text body come from the `Message`.
  - `Message.ID` is sent as the `Idempotency-Key` header.
  - Each send has a 10-second timeout.
  - An error names Resend's status and error name. It never contains the key,
    the link or the body. The recipient's address is also kept out of it, so
    the log names the user by ID only.
- `Message.ID`. `VerificationMessage` and `PasswordResetMessage` take it, and
  the call sites in `internal/httpapi/auth.go` pass a value derived from the
  token's hash, never the raw token.
- Configuration:
  - `RESEND_API_KEY` is optional. When set, it must start `re_`.
  - `EMAIL_FROM` defaults to `WaterlooStar <no-reply@waterloostar.com>`. It is
    parsed as a mailbox at startup, and an invalid one stops the service with
    a readable message.
- `cmd/api` chooses `ResendSender` when the key is set, and `LogSender`
  otherwise. Startup logs which one, with a warning in production when it is
  the log.
- `.env.example`, `deploy/docker-compose.yml` (both settings passed through,
  both optional) and the setup notes at the top of `.drone.yml`.
- `README.md` and `CLAUDE.md`:
  - The "Running it" note becomes "without a key, links are printed to the
    log".
  - The stale "In flight: `feature/listing-terms`" line goes, since that
    branch is merged.
  - The record of why Clerk was dropped.

### Out of scope

- The resend endpoint (Phase 14). "Resend" the provider and "resend" a link are
  unrelated.
- HTML bodies, delivery status and webhooks, retries, notification emails.

---

## Dependencies / Prerequisites

- `main` at or after the Listing Terms merge (PR #11).
- To verify by hand, which is the only way to confirm a real delivery:
  - A Resend API key (`re_…`) in the local `.env` as `RESEND_API_KEY`.
  - `waterloostar.com` verified as a sending domain in Resend, through the
    DNS records Resend lists.

  The developer provides both. Without them, everything else in this phase
  can be built and tested, and the hand check is recorded as not run.

---

## Implementation Steps

- [x] **Preparation**
  - [x] Update `index.md`: Phase 13 In Progress

- [x] **Configuration**
  - [x] `internal/config`: `ResendAPIKey`, `EmailFrom` (a parsed mailbox) and
        their validation
  - [x] `.env.example`: both, explained
  - [x] Verify: config tests for the default, a custom mailbox, an invalid one,
        a key without `re_`, and no key

- [x] **Sender**
  - [x] `internal/email/sender.go`: `Message.ID`; the message builders take it;
        the package comment names Resend and the log fallback
  - [x] `internal/email/resend.go`: `ResendSender` and `NewResendSender`
  - [x] `internal/httpapi/auth.go`: both call sites pass an ID derived from the
        token's hash
  - [x] `cmd/api/main.go`: choose the sender and log the choice
  - [x] `go mod tidy`: no new dependency
  - [x] Verify: unit tests against a fake Resend server

- [x] **Deployment**
  - [x] `deploy/docker-compose.yml`: `RESEND_API_KEY` and `EMAIL_FROM` from
        the host env file, both optional
  - [x] `.drone.yml`: the setup notes list them
  - [x] Verify: `docker compose -f deploy/docker-compose.yml config` succeeds
        with and without them

- [x] **Tests**
  - [x] `internal/email/resend_test.go`, against an `httptest` server:
    - The request is `POST /emails` with the key as its bearer token and the
      ID as its idempotency key.
    - The body carries the From mailbox, the recipient, the subject and the
      text, and no HTML.
    - A 4xx, a 5xx and an unparseable answer each become an error naming the
      status. None of them carries the key, the link, the body or the
      recipient.
    - A server slower than the timeout fails the send, and so does an
      unreachable one.
  - [x] Handler tests still pass with a recording sender. Messages carry an ID,
        and the raw token appears only in the link.
  - [x] Verify: `go vet ./...`; `go test ./...`, the database suite included

- [x] **By hand** (needs the developer's key and verified domain)
  - [x] With the key in `.env`, register a real `uwaterloo.ca` address:
    - The email arrives from `no-reply@waterloostar.com`.
    - Its link verifies the account.
  - [x] Request a password reset: the email arrives and its link works
  - [x] Without the key: startup says the log is the sender, and the link
        appears in the log as before
  - [x] The test account is removed from the local database afterwards

- [x] **Final verification**
  - [x] `go build ./...`, `go vet ./...`, `go test ./...` pass
  - [x] `README.md` and `CLAUDE.md` updated
  - [x] Update `index.md`: Phase 13 Complete
  - [x] Move `docs/specs/active/email-delivery/phase-13-resend-sender.md` →
        `docs/specs/implemented/email-delivery/phase-13-resend-sender.md`

---

## Completion Criteria

- [x] All checklist items completed and verified
- [x] No test reaches Resend
- [x] No key, link, body or recipient address in any error written by
      `ResendSender`
- [x] Without a key, behaviour is unchanged from today
- [x] No regressions in related areas
- [x] `index.md` phase status set to Complete
- [x] Phase spec doc moved to `docs/specs/implemented/email-delivery/`

---

## Implementation Notes

**Key files changed:**
- `internal/email/resend.go` (new): `ResendSender`, one `POST /emails` with
  `net/http`. `Idempotency-Key` is the message ID. The client has a 10-second
  timeout. Errors carry the status, Resend's error name and its message, with
  the key, the recipient and any URL redacted. A non-JSON answer is reduced to
  its status.
- `internal/email/resend_test.go` (new): the request's shape, four error
  answers, a timeout, an unreachable server, and `MessageID`. Every test runs
  against `httptest`.
- `internal/email/sender.go`: `Message.ID`, `MessageID` (a second SHA-256 of
  the stored token hash, so the database value never leaves the service), the
  builders take the ID, and the package comment.
- `internal/httpapi/auth.go`: both call sites pass `MessageID("verify"|"reset",
  hash)`. `auth_test.go`: a test that each email carries an ID and that the raw
  token appears only in the link.
- `internal/config/config.go` and its test: `ResendAPIKey` (must start `re_`,
  never echoed), `EmailFrom` (a parsed mailbox, default
  `WaterlooStar <no-reply@waterloostar.com>`).
- `cmd/api/main.go`: `newMailer` chooses the sender and logs it, with a warning
  in production without a key.
- `.env.example`, `deploy/docker-compose.yml`, `.drone.yml`: the two settings.
- `README.md` (an Email section), `CLAUDE.md`.

**Divergences from plan:**
- **The provider changed mid-phase, from Clerk to Resend.** The spec was
  revised and committed first (`docs(email-delivery): switch the provider from
  Clerk to Resend`). A complete Clerk sender, written to the original spec,
  passed its tests. Then, by hand, Clerk's `POST /v1/email` answered 404
  `resource_not_found` with a development key and with a production key on
  `clerk.waterloostar.com`, while a read-only call with the same key succeeded.
  The endpoint is not enabled for the account. The Clerk sender and its SDK
  were removed; `go.mod` is unchanged from `main`.
- **No SDK for Resend.** Its Go SDK reports 4xx answers as bare strings without
  a status. The direct call was planned in the revised spec.
- **Resend's message is kept, redacted,** rather than dropped. It is what says
  why a send was refused (for example "domain is not verified"), and
  `validation_error` alone covers several causes.
- **The first real registration went to the wrong API.** A local Docker stack
  (`docker-compose.yml`, `Makefile`; not part of this phase and not in this
  commit) had been started on :8080 before the key was in `.env`, so it held
  the port and logged the link instead of sending it. The API started for the
  check failed to bind, and only its log showed it. Recreating the container
  with the key fixed it. Lesson: confirm which process answers :8080 before a
  hand check.

**Verification run:**
- `go build ./...`, `go vet ./...`: clean.
  `PG_TEST_DSN=… go test -count=1 ./...`: every package passes, the database
  suite included.
- `docker compose -f deploy/docker-compose.yml config` resolves with and
  without the key.
- By hand, with the developer's send-only Resend key and `waterloostar.com`
  verified in Resend:
  - A probe to `delivered@resend.dev` was accepted (200, with an ID) and showed
    on the developer's dashboard.
  - Registering `y37yan@uwaterloo.ca` delivered "Confirm your WaterlooStar
    account" from `no-reply@waterloostar.com` to the developer's inbox. Its
    link verified the account.
  - A reset request delivered "Reset your WaterlooStar password". Its link was
    redeemed 76 seconds after sending, and the new password was set.
  - Without a key, startup logs that the log is the sender, and a
    registration's link appears there.
  - No key appeared in any log. The test account was removed afterwards; the
    database is back to its six seeded users.
- The developer saw the verify page offer "Log in" with no known password,
  because the test account was registered through the API with a random
  password. Asked, they chose to keep the flow as it is: the password is
  chosen at sign-up.
