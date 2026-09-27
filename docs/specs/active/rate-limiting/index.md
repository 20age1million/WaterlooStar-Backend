# Rate Limiting — API Phase 8

**Status:** Ready
**Branch:** `feature/rate-limiting`
**Ticket / Work Item:** N/A
**Owner(s):** WaterlooStar backend
**Date:** 2026-09-26

---

## Purpose / Goal

Put a cost on guessing. Login, registration and password reset stop accepting
unlimited attempts, and writes stop accepting unlimited posts, so a single actor
cannot brute-force an account or flood the site faster than a person can read it.

---

## Problem Statement / Motivation

There is no rate limit anywhere in this API. `POST /auth/login` accepts as many
attempts as anyone cares to make, against addresses that are not secret and not
guesswork: a `uwaterloo.ca` address is `firstname.lastname@` or a known eight-
character userid. An attacker does not need to discover who to attack, and
bcrypt at cost 12 slows each attempt to tens of milliseconds, which is a
throttle on their hardware, not a defence.

The same hole sits under three other doors. Password reset can be triggered
without limit — and once email delivery lands, that becomes someone else's inbox
flooded on demand, at our cost and under our name. Registration can be scripted
to fill the users table. The write endpoints can be scripted to fill the hub with
listings faster than anyone can take them down.

Why now: the admin feature immediately following this one makes an **admin
password the most valuable secret on the site**. Handing out keys before fitting a
lock is the wrong order, and it was the developer's call to reorder them.

---

## Proposed Solution / Design

An in-process limiter, in front of the handlers that can be abused, keyed on
**who is asking** rather than where they appear to be asking from.

### The blind spot that shapes the whole design

This API has no public URL. It sits on a private Docker network and the Next.js
server is its only client, calling it from inside the compose network. **Every
request therefore arrives from one IP** — the web container's. A per-IP limiter
inside this service would treat the entire internet as a single client and throttle
the whole site the moment one person retried a password.

So:

- **The limiter keys on an identifier**: the email being logged into, the address
  being registered, the account being reset, the authenticated user id. That is
  the thing worth protecting, and it is visible here.
- **Per-IP limiting belongs at the reverse proxy** in front of waterloostar.com,
  which is the only place that sees real client addresses. That is an operational
  step, documented here, not Go code.
- **The contract gains an optional forwarded-client-IP header** which the limiter
  uses as an extra dimension when present. Trusting a header is only acceptable
  because this service is unreachable from the internet. The frontend sending it
  is a follow-up in that repository; nothing here waits on it.

### Key Components

- **`internal/ratelimit`**: the limiter itself. Token buckets in a bounded map,
  with an injected clock so tests are deterministic and contain no sleeps.
- **`internal/httpapi/ratelimit_mw.go`**: the gin middleware, the key extraction
  per route, and the 429 that speaks the standard error envelope.
- **`internal/ratelimit/policy.go`**: the limits, as named constants in one file.
  A limit nobody can find is a limit nobody will tune.
- **`api/openapi.yaml`**: a `rate_limited` error code, the 429 response on the
  affected operations, and the `Retry-After` header.
- **`deploy/README.md`** (or the existing deploy notes): the nginx rule for
  per-IP limiting at the proxy.

### Data / Control Flow

- A request reaches a limited route. The middleware derives its key — the email
  from the body for login and register, the user id from the principal for writes.
- The bucket for that key is consulted. Enough tokens: the request proceeds. Not
  enough: **429** with the standard envelope, a `Retry-After` header, and a
  message that names the wait in plain words.
- **A successful login consumes nothing.** Only a failed attempt costs a token.
  This is the security model that matters — you throttle guessing, not using — and
  it also means normal use and the end-to-end test suite never meet the limiter.
- **There is no lockout.** Buckets refill on a clock. An attacker cannot lock a
  student out of their own account by guessing at it, which a lockout would let
  them do for free.
- A second, global bucket per endpoint sits behind the per-key ones, so cycling
  through a thousand different addresses is caught even though no single key
  trips.
- Keys expire. The map is swept and bounded, because a limiter that allocates a
  bucket per unique input is itself the denial-of-service.

### What is limited

| Route | Key | Shape of the limit |
|---|---|---|
| `POST /auth/login` | email | Tight, **failures only** |
| `POST /auth/register` | email + global | Tight per address, global cap on account creation |
| `POST /auth/password-reset` | email + global | Tight — this one sends mail on someone else's behalf |
| `POST /auth/password-reset/{token}` | token | Cap on confirmation attempts |
| `POST /auth/verify/{token}` | token | Cap on confirmation attempts |
| `POST /listings`, `POST /requests`, `POST /requests/{id}/offers` | user id | Generous — a person posting, not a script |
| Everything else | — | Unlimited |

`POST /auth/refresh` is **deliberately not limited**. The frontend's middleware
refreshes on navigation, which means a browsing session makes refresh calls at a
rate no human types at; throttling it would sign people out for using the site.
The refresh token is already single-use and rotated, which is the stronger control.

---

## Layers / Areas Affected

| Layer / Area | Change |
|---|---|
| Database schema | **None.** No counters, no lockout column, no migration |
| DB access layer | None |
| Service layer | New `internal/ratelimit` package: buckets, policy, sweeping |
| API handlers | Middleware on the affected routes; login reports failure to the limiter |
| DTOs / contracts | `rate_limited` error code, 429 responses, `Retry-After` |
| Auth | Failed logins become a counted event; nothing about tokens changes |
| Tests | Unit tests over an injected clock; handler tests for each limited route and for the 429 envelope |
| Deployment / Infra | None in the stack. One documented nginx rule at the reverse proxy |
| Configuration | **None.** Limits are constants in code, not environment variables |
| Operational | The limiter logs when a key trips, so an attack is visible in the API log |

---

## Trade-offs / Alternatives Considered

- **In-process over Redis.** One API container serves the site. Redis would add a
  service, a network hop and a dependency to the deploy for a guarantee nothing
  currently needs. The cost is stated in Assumptions: a restart forgets the
  counters, and a second replica would halve the effective limit. Both are
  acceptable; neither is silent.
- **Keyed on identity, not IP.** Forced by the topology, and better anyway: the
  email is what is under attack, and an attacker's address is cheap to change
  while a target's address is not.
- **Failures only, on login.** A limiter that counts successes punishes the person
  who mistyped once and then logged in, and would throttle the Playwright suite,
  which signs in repeatedly on purpose.
- **Refill, never lock out.** A lockout hands the attacker a way to deny a real
  student their account, which is a worse outcome than the guessing it prevents.
- **429 with a plain-English wait, not a silent delay.** Tarpitting hides the
  limit from the attacker and also from the confused student; the frontend's login
  form already renders the API's message verbatim, so a clear sentence needs no
  frontend change at all.
- **A global backstop per endpoint** as well as per-key buckets, because per-key
  alone is bypassed by varying the key.
- **No limit on refresh**, for the reason above. Recorded as a decision so it does
  not look like an omission.
- **Limits as constants, not configuration.** A tunable limit needs an operator
  who knows what to tune it to; nobody does yet. Changing one is a one-line commit
  and a deploy, which is honest for a value that should change rarely.

---

## Assumptions

- One API instance serves production. If a second is ever added, each keeps its
  own buckets and the effective limit doubles — recorded in the operational notes
  so it is a known consequence and not a surprise.
- A restart clearing the buckets is acceptable. An attacker cannot restart the
  container, and a deploy is rare.
- The reverse proxy in front of waterloostar.com can express a per-IP rule. It is
  nginx behind 1Panel, which can.
- bcrypt at cost 12 remains the password hash, so the limiter is a second layer
  rather than the only one.
- Normal use never meets these limits. Anything that does is a bug in the limits,
  and the verification steps below are how that gets found before a student does.

---

## Constraints

- **This repository is specified on its own.** The optional forwarded-IP header is
  offered to the frontend, not required of it.
- **No new configuration and no new services.** The deployed stack must not gain an
  environment variable, a secret, a container or a port.
- **No schema change.** No migration in this feature.
- **A 429 must not reveal whether an account exists.** The response for an unknown
  address and a known one are identical.
- **Every 429 uses the one error envelope** in `internal/apierror`, like every
  other failure in this API.
- **The limiter must not be able to grow without bound.** A bounded map and an
  expiry sweep are part of the deliverable, not a later optimisation.
- **No test may sleep.** The clock is injected; a suite that waits for real seconds
  will be deleted the first time it flakes in CI.
- Session refresh must keep working unchanged. If a browsing session can be
  throttled into signing out, this feature has failed.

---

## Success Criteria / Definition of Done

- Repeated wrong passwords against one address stop being accepted and answer 429
  with a `Retry-After` and a message naming the wait; the right password still
  works once the bucket refills.
- A correct login, repeated, is never throttled.
- Cycling through many addresses trips the global backstop.
- Registration and password reset are both capped per address and overall.
- A signed-in user cannot post listings or requests faster than the generous
  per-user cap.
- `POST /auth/refresh` is never throttled, and the frontend's session refresh is
  unaffected: signing in, browsing for an hour and coming back still works.
- Unknown and known addresses are indistinguishable from the outside.
- The limiter's memory is bounded under a flood of unique keys, demonstrated by a
  test.
- Every test runs on an injected clock; none sleeps.
- The nginx per-IP rule is written down where whoever runs the host will find it.
- `go build ./...`, `go vet ./...` and `go test ./...` pass; generators idempotent.

---

## Implementation Steps

- [ ] **Preparation**
  - [ ] Confirm the requests and offers branches are merged into `main`
  - [ ] Update `docs/specs/active/rate-limiting/index.md` — set to In Progress

- [ ] **The limiter**
  - [ ] Add `internal/ratelimit` — a token bucket with capacity and refill rate, an
        injected `now func() time.Time`, and `Allow(key)` / `Report(key)`
  - [ ] Bound the map: a maximum number of tracked keys, eviction of the least
        recently used, and a sweep of buckets that have refilled to full
  - [ ] Add `policy.go` — every limit as a named constant with a one-line comment
        saying what it is protecting and why that number
  - [ ] Verify: unit tests over the injected clock cover exhaustion, refill,
        eviction under a flood of unique keys, and concurrent access under `-race`

- [ ] **Contract**
  - [ ] Add `rate_limited` to the `ErrorCode` enum in `api/openapi.yaml`
  - [ ] Add the 429 response and the `Retry-After` header to every limited
        operation
  - [ ] Document the optional forwarded-client-IP header, noting it is trusted only
        because the service is private
  - [ ] Verify: both generators regenerate cleanly and idempotently

- [ ] **Middleware**
  - [ ] Add `internal/httpapi/ratelimit_mw.go` — key extraction per route and the
        429 through `internal/apierror`, never a bare gin abort
  - [ ] Read the email for keying without consuming the request body the handler
        then needs
  - [ ] Normalise the key: lowercase the email, so `A@` and `a@` share a bucket
  - [ ] Include the forwarded client IP in the key when the header is present
  - [ ] Wire it in `router.go` route by route — an allowlist, so a new endpoint is
        unlimited until someone decides otherwise, rather than limited by accident
  - [ ] Verify: by hand — fail a login repeatedly and read the 429, its
        `Retry-After` and its message; then log in correctly after the wait

- [ ] **Login reports failures**
  - [ ] A wrong password reports to the limiter; a correct one does not
  - [ ] An unknown address costs a token exactly as a known one does, so the two
        stay indistinguishable
  - [ ] Verify: a hundred correct logins in a row are never throttled

- [ ] **Operational documentation**
  - [ ] Add the nginx per-IP rule for `/api/auth/*` to the deploy notes, with the
        reason the API cannot do it itself
  - [ ] Note in `CLAUDE.md` that limits are per instance and reset on restart
  - [ ] Verify: the rule is written so it can be pasted into the proxy config and
        the reason survives without this spec

- [ ] **Tests**
  - [ ] `internal/ratelimit` unit tests: exhaustion, refill, bounded memory,
        eviction, `-race`
  - [ ] `internal/httpapi/ratelimit_test.go`: the 429 envelope and header on each
        limited route, the failures-only rule on login, the global backstop under
        key cycling, unknown and known addresses matching, and refresh staying
        unlimited
  - [ ] A test asserting the route allowlist matches the policy constants, so a
        limit added to one and not the other fails
  - [ ] Verify: `go test ./...` and `go test -race ./...` pass, with and without a
        database

- [ ] **Frontend check (this repository's responsibility to confirm, not to fix)**
  - [ ] Confirm the frontend's login, register and reset forms render the 429
        message — `toFormState` in `src/app/auth-actions.ts` shows the API's message
        for any failure, so no change is expected
  - [ ] Confirm the Playwright suite still passes against a limited API; if it
        trips a limit, the limit is wrong, not the test
  - [ ] Record the two follow-ups for that repository: adding `rate_limited` to the
        hand-written `ApiErrorCode` union in `src/lib/api-error.ts`, and forwarding
        the client IP

- [ ] **Final verification**
  - [ ] `go build ./...` and `go vet ./...` pass
  - [ ] All tests pass, with and without a database
  - [ ] `README.md` documents the limits and the 429
  - [ ] `docs/specs/INDEX.md` — move this feature from Active to Implemented
  - [ ] Move `docs/specs/active/rate-limiting/index.md` →
        `docs/specs/implemented/rate-limiting/index.md`

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
