# Phase 13 — Clerk sender

**Status:** Ready
**Feature:** [Email Delivery](./index.md)
**Objective:** Verification and password-reset emails are delivered through Clerk when a key is configured, and the log stays the sender when one is not.

---

## Scope

### In scope

- `ClerkSender` in `internal/email/clerk.go`, implementing `Sender`:
  - The recipient by address. From is the configured mailbox. The subject and
    a plain-text body come from the `Message`.
  - `Message.ID` is passed as the idempotency key.
  - Each send has a 10-second timeout.
  - An error names Clerk's status and error code. It never contains the key,
    the recipient's link or the body.
- `Message.ID`. `VerificationMessage` and `PasswordResetMessage` take it, and
  the call sites in `internal/httpapi/auth.go` pass a value derived from the
  token's hash, never the raw token.
- Configuration:
  - `CLERK_SECRET_KEY` is optional.
  - `EMAIL_FROM` defaults to `WaterlooStar <no-reply@waterloostar.com>`. It is
    parsed as a mailbox at startup, and an invalid one stops the service with
    a readable message.
- `cmd/api` chooses `ClerkSender` when the key is set, and `LogSender`
  otherwise. Startup logs which one, with a warning in production when it is
  the log.
- `github.com/clerk/clerk-sdk-go/v3` added at an exact version.
- `.env.example`, `deploy/docker-compose.yml` (both settings passed through,
  both optional) and the setup notes at the top of `.drone.yml`.
- `README.md` and `CLAUDE.md`. The "Running it" note becomes "without a key,
  links are printed to the log". The stale "In flight: `feature/listing-terms`"
  line goes, since that branch is merged.

### Out of scope

- The resend endpoint (Phase 14).
- HTML bodies, delivery status, retries, notification emails.

---

## Dependencies / Prerequisites

- `main` at or after the Listing Terms merge (PR #11).
- To verify by hand, which is the only way to confirm the assumptions:
  - A Clerk secret key in the local `.env` (in place since 2026-10-11, a
    development `sk_test_` key), from the developer's Clerk application. That
    application is used only to send emails. None of Clerk's authentication is installed, and
    `clerk init` is not run in either repository.
  - `waterloostar.com` verified as a sending domain in that application.

  The developer provides both. Without them, everything else in this phase
  can be built and tested, and the hand check is recorded as not run.

---

## Implementation Steps

- [ ] **Preparation**
  - [ ] Confirm the SDK's client configuration takes a custom backend URL, HTTP
        client and key, so tests can point it at `httptest`. If it cannot,
        the sender makes the HTTP call itself to the same endpoint and shape.
        That divergence is recorded in the Implementation Notes.
  - [ ] Update `index.md`: Phase 13 In Progress

- [ ] **Dependency**
  - [ ] `go get github.com/clerk/clerk-sdk-go/v3@<exact version>`; `go mod tidy`
  - [ ] Verify: `go build ./...`

- [ ] **Configuration**
  - [ ] `internal/config`: `ClerkSecretKey`, `EmailFrom` (a parsed mailbox)
        and their validation
  - [ ] `.env.example`: both, explained
  - [ ] Verify: config tests for the default, a custom mailbox, an invalid one,
        and no key

- [ ] **Sender**
  - [ ] `internal/email/sender.go`: `Message.ID`; the message builders take it;
        the package comment names Clerk and the log fallback
  - [ ] `internal/email/clerk.go`: `ClerkSender` and `NewClerkSender`
  - [ ] `internal/httpapi/auth.go`: both call sites pass an ID derived from the
        token's hash
  - [ ] `cmd/api/main.go`: choose the sender and log the choice
  - [ ] Verify: unit tests against a fake Clerk server

- [ ] **Deployment**
  - [ ] `deploy/docker-compose.yml`: `CLERK_SECRET_KEY` and `EMAIL_FROM`
        from the host env file, both optional
  - [ ] `.drone.yml`: the setup notes list them
  - [ ] Verify: `docker compose -f deploy/docker-compose.yml config` succeeds
        with and without them

- [ ] **Tests**
  - [ ] `internal/email/clerk_test.go`, against an `httptest` server:
    - The request reaches the send endpoint with the key as its bearer token.
    - The body carries the address, the From mailbox, the subject, the text
      and the idempotency key.
    - A 4xx and a 5xx each become an error naming the status, with neither
      the key nor the body in it.
    - A server slower than the timeout fails the send.
  - [ ] Handler tests still pass with a recording sender. Messages carry an ID,
        and the raw token appears only in the link.
  - [ ] Verify: `go vet ./...`; `go test ./...`, the database suite included

- [ ] **By hand** (needs the developer's key)
  - [ ] With the key in `.env`, register a real `uwaterloo.ca` address:
    - The email arrives from `no-reply@waterloostar.com`.
    - Its link verifies the account.
  - [ ] Request a password reset: the email arrives and its link works
  - [ ] Without the key: startup says the log is the sender, and the link
        appears in the log as before

- [ ] **Final verification**
  - [ ] `go build ./...`, `go vet ./...`, `go test ./...` pass
  - [ ] `README.md` and `CLAUDE.md` updated
  - [ ] Update `index.md`: Phase 13 Complete
  - [ ] Move `docs/specs/active/email-delivery/phase-13-clerk-sender.md` →
        `docs/specs/implemented/email-delivery/phase-13-clerk-sender.md`

---

## Completion Criteria

- [ ] All checklist items completed and verified
- [ ] No test reaches Clerk
- [ ] No key, link or body in any error or log line written by `ClerkSender`
- [ ] Without a key, behaviour is unchanged from today
- [ ] No regressions in related areas
- [ ] `index.md` phase status set to Complete
- [ ] Phase spec doc moved to `docs/specs/implemented/email-delivery/`

---

## Implementation Notes

> *Added after completion.*
