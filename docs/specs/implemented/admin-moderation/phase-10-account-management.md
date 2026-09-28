# Phase 10 — Account management

**Status:** Complete
**Feature:** [Admin and Moderation](./index.md)
**Objective:** An admin can suspend an account, reinstate it, verify one by hand and change a role — each with a reason, each recorded, each reversible.

---

## Scope

### In scope

- Suspend and reinstate, with the consequences that make suspension mean
  something: no login, no refresh, and their content out of public view.
- Manual verification — the immediate answer to email links that only reach the
  log.
- Role change between `user` and `admin`, with the guards that stop an operator
  locking themselves out.
- The per-account history: what has been done to this account, and by whom.

### Out of scope

- Deleting an account. Ruled out for the feature: every user foreign key
  cascades.
- Editing an account's profile — username, email, avatar, level, star points. An
  admin fixing someone's display name is a different feature and a different
  argument.
- Resetting someone's password, or signing in as them. Impersonation is not on
  the table.
- Notifying the user they were suspended. No email provider.
- The `moderator` role.

---

## Dependencies / Prerequisites

- Phase 9 — the guard, the ledger and the transactional helper.
- The refresh-token revocation path from Phase 1, which suspension reuses rather
  than reinventing.

---

## Implementation Steps

- [x] **Preparation**
  - [x] Confirm Phase 9 is complete
  - [x] Update `index.md` — set Phase 10 to In Progress (went straight to Complete
        in the phase commit)

- [x] **Queries**
  - [x] Add to `internal/db/queries/admin.sql` — `SuspendUser`, `ReinstateUser`,
        `SetUserRole`, `VerifyUserAsAdmin` (`SetUserRole` already came with Phase 9)
  - [x] `SuspendUser` sets the timestamp, actor and reason together and returns
        the row, so the handler logs what actually changed rather than what it
        intended
  - [x] Reinstating clears all three columns — a reinstated account is
        indistinguishable from one never suspended, and the ledger is where the
        history lives
  - [x] Suspending an already-suspended account must not overwrite the original
        timestamp and reason
  - [x] Verify: query tests including the re-suspend case

- [x] **Making suspension bite**
  - [x] Login refuses a suspended account, with a message that says suspended
        rather than implying a wrong password
  - [x] Refresh refuses a suspended account, so an existing session dies within
        the access token's fifteen minutes
  - [x] Suspension revokes the account's refresh tokens in the same transaction
  - [x] Password reset cannot be used to get back in: the reset completes or is
        refused, but the resulting login is still refused while suspended
  - [x] A role change revokes refresh tokens too — a demoted admin should not keep
        a live admin session by refreshing
  - [x] Verify: by hand — sign in, suspend from a second admin session, confirm
        the first session cannot refresh and cannot sign back in; reinstate and
        confirm it works again

- [x] **Hiding a suspended user's content**
  - [x] Add the suspension filter to the public read queries in
        `internal/db/queries/listings.sql`, `requests.sql` and `offers.sql` — the
        owner's account must not be suspended
  - [x] Put the filter in the SQL, not the handlers, for the reason recorded in
        `CLAUDE.md`: a handler can forget
  - [x] Confirm the owner still sees their own posts in `/me/listings` and
        `/me/requests` — suspension hides content from the public, it does not
        hide it from the person who wrote it
  - [x] Check the query plans: this adds a join or an `EXISTS` to the hottest
        reads on the site
  - [x] Verify: query tests proving a suspended owner's listing, request and offer
        leave public results and return in full on reinstatement

- [x] **Contract and handlers**
  - [x] Add the account-management paths to `api/openapi.yaml`
  - [x] A reason is required on every one of them, with a minimum length — a
        ledger of empty reasons is a ledger of nothing
  - [x] Add the handlers to `internal/httpapi/admin_users.go`
  - [x] Refuse: suspending yourself, demoting yourself, demoting the last admin,
        and acting on an account that does not exist (404, as everywhere else)
  - [x] Every successful change writes its `admin_actions` row in the same
        transaction, with the before and after in `detail`
  - [x] Verify: generators run cleanly; each refusal returns the documented
        envelope

- [x] **Tests**
  - [x] Query tests for each new query
  - [x] `internal/httpapi/admin_users_test.go`: the suspend round trip, the
        reinstate round trip, manual verification, role change, every self-action
        guard, the last-admin guard, the reason requirement, and a ledger row per
        action
  - [x] Regression tests in the listings, requests and offers handler tests
        proving a suspended owner's content is absent from public reads
  - [x] Verify: `go test ./...` passes with and without a database

- [x] **Final verification**
  - [x] `go build ./...` and `go vet ./...` pass
  - [x] All tests pass
  - [x] Update `index.md` — set Phase 10 to Complete
  - [x] Move `docs/specs/active/admin-moderation/phase-10-account-management.md` →
        `docs/specs/implemented/admin-moderation/phase-10-account-management.md`

---

## Completion Criteria

- [x] All checklist items completed and verified
- [x] A suspended account cannot log in, cannot refresh, and has no live session
- [x] A suspended owner's listings, requests and offers are absent from every
      public read, and their own statuses are untouched
- [x] Reinstating restores exactly what suspension hid
- [x] An admin can verify an account by hand, and that account can then post
- [x] An admin cannot suspend or demote themselves, and the last admin cannot be
      demoted
- [x] Every action required a reason and produced exactly one ledger row
- [x] `index.md` phase status set to Complete, phase doc moved to `implemented/`

---

## Implementation Notes

**Key files changed:**
- `internal/db/queries/admin.sql`: `SuspendUser`, `ReinstateUser`,
  `VerifyUserAsAdmin`. Each matches only when the change would change
  something, so a repeat returns no row and nothing is logged.
- `internal/db/queries/listings.sql`, `requests.sql`, `offers.sql`: the
  suspension filter on `ListListings`, `CountListings`, `GetPublishedListing`,
  `ListRequests`, `CountRequests`, `GetPublishedRequest`,
  `ListOffersForRequest`, `CountOffersForRequest`, and the offer counts embedded
  in the three request reads.
- `internal/db/accounts.go`: `Suspend`, `Reinstate`, `VerifyByHand` through
  `Audited`, with their refusals as errors. `roles.go`: `ChangeRole` refuses a
  self-change.
- `internal/httpapi/admin_accounts.go`: the four handlers and the shared
  reason check and error mapping. `admin_guard.go`: `unrouted` covers the new
  operations.
- `internal/httpapi/auth.go`: login answers 403 for a suspended account once the
  password matches; refresh refuses one.
- `api/openapi.yaml`: `POST /admin/users/{id}/suspend`, `/reinstate`, `/verify`,
  `/role`; `AdminReason`, `AdminRoleChange`; a 403 on `/auth/login`.
- Tests: `internal/db/queries_suspension_test.go`,
  `internal/httpapi/admin_users_test.go`; the fake mirrors the filter and the
  three queries.
- `README.md`, `CLAUDE.md`.

**Divergences from plan:**
- **The handlers are in `admin_accounts.go`**, not `admin_users.go`. The
  write path is separate from the read path, as `listings_write.go` is from
  `listings.go`.
- **The regression tests for hidden content live in `admin_users_test.go`**
  (handler level, over the fake) and `queries_suspension_test.go` (the SQL),
  rather than in the listings, requests and offers handler files. One test
  follows a suspended owner's listing, request and offer through every public
  read and back, which reads better than three partial copies.
- **Every refusal is a 409 `conflict`**: acting on yourself, repeating a
  change, and demoting the last admin. The contract documents 409 with
  `message` naming which one.
- **The minimum reason is ten characters once trimmed**, and at most 1,000, the
  ledger's limit.
- **The last-admin guard is unreachable over HTTP.** The caller is always an
  admin and cannot demote themselves, so demoting anyone else always leaves one.
  `ChangeRole` still enforces it; the Phase 9 query test and `cmd/admin` test
  cover it.
- **Password reset needed no change.** A reset completes as before; login is
  still refused while suspended, because the check is in login.

**Verification run:**
- `go build ./...`, `go vet ./...`: clean. Both generators idempotent.
- `go test ./...` with and without `PG_TEST_DSN`: pass.
- Mutation check: removing the filter from `GetPublishedListing` fails
  `TestASuspendedAccountsPostsLeavePublicReads`; restored.
- Query plans on the development database: the listing reads already joined
  `users` before this phase, so the filter tests a row the query fetches
  anyway. At this size PostgreSQL scans `users`; at scale the join goes through
  the primary key. No new index.
- By hand against a running API, with `meil` as admin and `priyas` signed in:
  suspend → public listings 6 → 5, Priya's refresh 401, her login 403 with the
  suspended message, a wrong password the ordinary 401; reinstate → listings
  back to 6, login 200. The account's ledger shows both actions with `meil` as
  actor. (A first run showed 403 after reinstating too. That was the test
  script's cookie jar tripping the CSRF check, not the API; rerun with fresh
  jars as above.)
