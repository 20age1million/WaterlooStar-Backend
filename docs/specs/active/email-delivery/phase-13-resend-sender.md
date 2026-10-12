# Phase 13 — Resend sender

**Status:** Ready
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

- [ ] **Preparation**
  - [ ] Update `index.md`: Phase 13 In Progress

- [ ] **Configuration**
  - [ ] `internal/config`: `ResendAPIKey`, `EmailFrom` (a parsed mailbox) and
        their validation
  - [ ] `.env.example`: both, explained
  - [ ] Verify: config tests for the default, a custom mailbox, an invalid one,
        a key without `re_`, and no key

- [ ] **Sender**
  - [ ] `internal/email/sender.go`: `Message.ID`; the message builders take it;
        the package comment names Resend and the log fallback
  - [ ] `internal/email/resend.go`: `ResendSender` and `NewResendSender`
  - [ ] `internal/httpapi/auth.go`: both call sites pass an ID derived from the
        token's hash
  - [ ] `cmd/api/main.go`: choose the sender and log the choice
  - [ ] `go mod tidy`: no new dependency
  - [ ] Verify: unit tests against a fake Resend server

- [ ] **Deployment**
  - [ ] `deploy/docker-compose.yml`: `RESEND_API_KEY` and `EMAIL_FROM` from
        the host env file, both optional
  - [ ] `.drone.yml`: the setup notes list them
  - [ ] Verify: `docker compose -f deploy/docker-compose.yml config` succeeds
        with and without them

- [ ] **Tests**
  - [ ] `internal/email/resend_test.go`, against an `httptest` server:
    - The request is `POST /emails` with the key as its bearer token and the
      ID as its idempotency key.
    - The body carries the From mailbox, the recipient, the subject and the
      text, and no HTML.
    - A 4xx, a 5xx and an unparseable answer each become an error naming the
      status. None of them carries the key, the link, the body or the
      recipient.
    - A server slower than the timeout fails the send, and so does an
      unreachable one.
  - [ ] Handler tests still pass with a recording sender. Messages carry an ID,
        and the raw token appears only in the link.
  - [ ] Verify: `go vet ./...`; `go test ./...`, the database suite included

- [ ] **By hand** (needs the developer's key and verified domain)
  - [ ] With the key in `.env`, register a real `uwaterloo.ca` address:
    - The email arrives from `no-reply@waterloostar.com`.
    - Its link verifies the account.
  - [ ] Request a password reset: the email arrives and its link works
  - [ ] Without the key: startup says the log is the sender, and the link
        appears in the log as before
  - [ ] The test account is removed from the local database afterwards

- [ ] **Final verification**
  - [ ] `go build ./...`, `go vet ./...`, `go test ./...` pass
  - [ ] `README.md` and `CLAUDE.md` updated
  - [ ] Update `index.md`: Phase 13 Complete
  - [ ] Move `docs/specs/active/email-delivery/phase-13-resend-sender.md` →
        `docs/specs/implemented/email-delivery/phase-13-resend-sender.md`

---

## Completion Criteria

- [ ] All checklist items completed and verified
- [ ] No test reaches Resend
- [ ] No key, link, body or recipient address in any error written by
      `ResendSender`
- [ ] Without a key, behaviour is unchanged from today
- [ ] No regressions in related areas
- [ ] `index.md` phase status set to Complete
- [ ] Phase spec doc moved to `docs/specs/implemented/email-delivery/`

---

## Implementation Notes

> *Added after completion.*
