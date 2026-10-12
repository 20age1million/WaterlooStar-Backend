# Phase 14 — A new verification link

**Status:** Complete
**Feature:** [Email Delivery](./index.md)
**Objective:** A student whose verification link is lost or expired can ask for a new one, without the endpoint revealing who has an account.

---

## Scope

### In scope

- `POST /auth/verify/resend`, operation `resendVerification`:
  - The body is `{ "email": … }`.
  - It answers 202 with no body for every well-formed address.
  - It answers 400 when the address is missing or malformed, and 429 when an
    allowance is spent.
- The handler sends a new link only when the address belongs to an account
  that is unverified and not suspended. Sending one spends every outstanding
  link for that account (the existing `issueVerificationEmail`). In every
  other case nothing is sent:
  - an unknown address
  - an account that is already verified
  - a suspended account

  The handler logs which case applied, without the address. A failed send is
  logged too, and the answer is still 202.
- Rate limits, keyed by email like the reset request:
  - `ResendPerEmail`, 3 an hour.
  - `ResendGlobal`, 30 an hour.
  - The 429 message is "A new link has already been requested for that
    address. Try again in %s." It is charged whether or not the address has
    an account, so it reveals nothing.
- `Register`'s comment on a failed send names this endpoint.
- `README.md`: the endpoint table (forty-two endpoints) and the rate-limit
  table. `CLAUDE.md`.

### Out of scope

- The frontend's button. That is the frontend's phase, generated from this
  contract.
- Sending a new link to a signed-in, unverified student by session. The address
  is enough, and it works after the tab is closed.

---

## Dependencies / Prerequisites

- Phase 13, so the new link is actually delivered. The endpoint works with
  `LogSender` too.

---

## Implementation Steps

- [x] **Preparation**
  - [x] Update `index.md`: Phase 14 In Progress

- [x] **Contract**
  - [x] `api/openapi.yaml`: the operation, its request body and its 202, 400
        and 429 responses, under the auth tag
  - [x] Regenerate: `go tool oapi-codegen -config api/oapi-codegen.yaml api/openapi.yaml`
  - [x] Verify: `go build ./...` fails only for the missing handler

- [x] **Handler**
  - [x] `internal/httpapi/auth.go`: `ResendVerification`, using
        `GetUserByEmail` and `issueVerificationEmail`. No new query.
  - [x] Verify: `go build ./...`

- [x] **Rate limiting**
  - [x] `internal/ratelimit/policy.go`: `ResendPerEmail`, `ResendGlobal`
  - [x] `internal/httpapi/ratelimit_mw.go`: the route on the allowlist,
        `keyByEmail`
  - [x] Verify: the allowlist's own tests pass with the route on it

- [x] **Tests**
  - [x] `internal/httpapi` handler tests, with the fake querier and a
        recording sender:
    - An unverified account gets one message, and its previous links are
      consumed.
    - Unknown, verified and suspended addresses get nothing.
    - A sender that fails still answers 202.
    - The four cases answer with the same status and the same (empty) body.
    - A malformed address answers 400.
  - [x] Rate-limit test: the fourth request for one address within the hour
        answers 429 with `Retry-After`. A different address is unaffected.
  - [x] Verify: `go vet ./...`; `go test ./...`, the database suite included

- [x] **By hand**
  - [x] Against the running API: register, ask for a new link, then check:
    - The first link answers as spent.
    - The second verifies the account.
    - A fourth request answers 429.

- [x] **Final verification**
  - [x] `go build ./...`, `go vet ./...`, `go test ./...` pass
  - [x] `README.md` and `CLAUDE.md` updated
  - [x] Update `index.md`: Phase 14 Complete
  - [x] Move `docs/specs/active/email-delivery/phase-14-resend-verification.md` →
        `docs/specs/implemented/email-delivery/phase-14-resend-verification.md`
  - [x] *(Final phase)* Move `docs/specs/active/email-delivery/index.md` →
        `docs/specs/implemented/email-delivery/index.md`
  - [x] Update `docs/specs/INDEX.md`

---

## Completion Criteria

- [x] All checklist items completed and verified
- [x] Every well-formed address gets the same answer
- [x] A new link spends the old one
- [x] The route is on the rate-limit allowlist
- [x] No regressions in related areas
- [x] `index.md` phase status set to Complete
- [x] Phase spec doc moved to `docs/specs/implemented/email-delivery/`
- [x] *(Final phase)* `index.md` moved to `docs/specs/implemented/email-delivery/`

---

## Implementation Notes

**Key files changed:**
- `api/openapi.yaml`: `POST /auth/verify/resend` (`resendVerification`),
  taking the existing `EmailRequest`; 202, 400, 429, 500. Regenerated
  `internal/httpapi/gen/server.gen.go`.
- `internal/httpapi/auth.go`: `ResendVerification`. It looks the address up
  with `GetUserByEmail`, sends through the existing `issueVerificationEmail`
  (which spends every outstanding link first), and logs which case applied by
  user ID only. No new query. `Register`'s comment on a failed send now names
  the endpoint.
- `internal/ratelimit/policy.go`: `ResendPerEmail` (3 an hour) and
  `ResendGlobal` (30 an hour). `internal/httpapi/ratelimit_mw.go`: the route on
  the allowlist, `keyByEmail`.
- `internal/httpapi/resend_verification_test.go` (new):
  - a new link spends the old one, and the new one verifies;
  - the address matches regardless of case and spaces;
  - unknown, verified and suspended addresses get nothing;
  - every case, a failed send included, answers the same status and body;
  - a malformed or missing address answers 400;
  - the fourth request answers 429, and another address is unaffected.
- `internal/httpapi/auth_test.go`: the capturing mailer can fail on demand and
  count what it sent.
- `README.md` (endpoint table, rate limits, Email section), `CLAUDE.md`
  (forty-two endpoints, Phase 14, what comes next), `docs/specs/INDEX.md`.

**Divergences from plan:**
- **A missing address is refused in the handler.** The generated decoder
  refuses a malformed address, but `{}` decodes to an empty one, which would
  otherwise have answered 202. The test caught it. `POST /auth/password-reset`
  has the same gap (202 for `{}`). It is harmless, since nothing is sent, and
  was left alone as out of scope.
- The frontend button is not in this repository; see `CLAUDE.md`, "What comes
  next".

**Verification run:**
- `go build ./...`, `go vet ./...`: clean.
  `PG_TEST_DSN=… go test -count=1 ./...`: every package passes, the database
  suite and the allowlist's route checks included.
- By hand, against the built API on :8081 without a key (links in its log), so
  no real email was sent:
  - register 201; resend 202, and a second, different link in the log;
  - the first link 400, the second 200;
  - a resend for the now verified account 202, with no new link;
  - the fourth request within the hour 429;
  - an unknown address 202.
  - The log named the cases ("verified account", "unknown address") without
    the address. The test account was removed afterwards.
