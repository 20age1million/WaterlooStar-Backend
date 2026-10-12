# Phase 14 — A new verification link

**Status:** Ready
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

- [ ] **Preparation**
  - [ ] Update `index.md`: Phase 14 In Progress

- [ ] **Contract**
  - [ ] `api/openapi.yaml`: the operation, its request body and its 202, 400
        and 429 responses, under the auth tag
  - [ ] Regenerate: `go tool oapi-codegen -config api/oapi-codegen.yaml api/openapi.yaml`
  - [ ] Verify: `go build ./...` fails only for the missing handler

- [ ] **Handler**
  - [ ] `internal/httpapi/auth.go`: `ResendVerification`, using
        `GetUserByEmail` and `issueVerificationEmail`. No new query.
  - [ ] Verify: `go build ./...`

- [ ] **Rate limiting**
  - [ ] `internal/ratelimit/policy.go`: `ResendPerEmail`, `ResendGlobal`
  - [ ] `internal/httpapi/ratelimit_mw.go`: the route on the allowlist,
        `keyByEmail`
  - [ ] Verify: the allowlist's own tests pass with the route on it

- [ ] **Tests**
  - [ ] `internal/httpapi` handler tests, with the fake querier and a
        recording sender:
    - An unverified account gets one message, and its previous links are
      consumed.
    - Unknown, verified and suspended addresses get nothing.
    - A sender that fails still answers 202.
    - The four cases answer with the same status and the same (empty) body.
    - A malformed address answers 400.
  - [ ] Rate-limit test: the fourth request for one address within the hour
        answers 429 with `Retry-After`. A different address is unaffected.
  - [ ] Verify: `go vet ./...`; `go test ./...`, the database suite included

- [ ] **By hand**
  - [ ] Against the running API: register, ask for a new link, then check:
    - The first link answers as spent.
    - The second verifies the account.
    - A fourth request answers 429.

- [ ] **Final verification**
  - [ ] `go build ./...`, `go vet ./...`, `go test ./...` pass
  - [ ] `README.md` and `CLAUDE.md` updated
  - [ ] Update `index.md`: Phase 14 Complete
  - [ ] Move `docs/specs/active/email-delivery/phase-14-resend-verification.md` →
        `docs/specs/implemented/email-delivery/phase-14-resend-verification.md`
  - [ ] *(Final phase)* Move `docs/specs/active/email-delivery/index.md` →
        `docs/specs/implemented/email-delivery/index.md`
  - [ ] Update `docs/specs/INDEX.md`

---

## Completion Criteria

- [ ] All checklist items completed and verified
- [ ] Every well-formed address gets the same answer
- [ ] A new link spends the old one
- [ ] The route is on the rate-limit allowlist
- [ ] No regressions in related areas
- [ ] `index.md` phase status set to Complete
- [ ] Phase spec doc moved to `docs/specs/implemented/email-delivery/`
- [ ] *(Final phase)* `index.md` moved to `docs/specs/implemented/email-delivery/`

---

## Implementation Notes

> *Added after completion.*
