# Phase 10 — Account management

**Status:** Ready
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

- [ ] **Preparation**
  - [ ] Confirm Phase 9 is complete
  - [ ] Update `index.md` — set Phase 10 to In Progress

- [ ] **Queries**
  - [ ] Add to `internal/db/queries/admin.sql` — `SuspendUser`, `ReinstateUser`,
        `SetUserRole`, `VerifyUserAsAdmin`
  - [ ] `SuspendUser` sets the timestamp, actor and reason together and returns
        the row, so the handler logs what actually changed rather than what it
        intended
  - [ ] Reinstating clears all three columns — a reinstated account is
        indistinguishable from one never suspended, and the ledger is where the
        history lives
  - [ ] Suspending an already-suspended account must not overwrite the original
        timestamp and reason
  - [ ] Verify: query tests including the re-suspend case

- [ ] **Making suspension bite**
  - [ ] Login refuses a suspended account, with a message that says suspended
        rather than implying a wrong password
  - [ ] Refresh refuses a suspended account, so an existing session dies within
        the access token's fifteen minutes
  - [ ] Suspension revokes the account's refresh tokens in the same transaction
  - [ ] Password reset cannot be used to get back in: the reset completes or is
        refused, but the resulting login is still refused while suspended
  - [ ] A role change revokes refresh tokens too — a demoted admin should not keep
        a live admin session by refreshing
  - [ ] Verify: by hand — sign in, suspend from a second admin session, confirm
        the first session cannot refresh and cannot sign back in; reinstate and
        confirm it works again

- [ ] **Hiding a suspended user's content**
  - [ ] Add the suspension filter to the public read queries in
        `internal/db/queries/listings.sql`, `requests.sql` and `offers.sql` — the
        owner's account must not be suspended
  - [ ] Put the filter in the SQL, not the handlers, for the reason recorded in
        `CLAUDE.md`: a handler can forget
  - [ ] Confirm the owner still sees their own posts in `/me/listings` and
        `/me/requests` — suspension hides content from the public, it does not
        hide it from the person who wrote it
  - [ ] Check the query plans: this adds a join or an `EXISTS` to the hottest
        reads on the site
  - [ ] Verify: query tests proving a suspended owner's listing, request and offer
        leave public results and return in full on reinstatement

- [ ] **Contract and handlers**
  - [ ] Add the account-management paths to `api/openapi.yaml`
  - [ ] A reason is required on every one of them, with a minimum length — a
        ledger of empty reasons is a ledger of nothing
  - [ ] Add the handlers to `internal/httpapi/admin_users.go`
  - [ ] Refuse: suspending yourself, demoting yourself, demoting the last admin,
        and acting on an account that does not exist (404, as everywhere else)
  - [ ] Every successful change writes its `admin_actions` row in the same
        transaction, with the before and after in `detail`
  - [ ] Verify: generators run cleanly; each refusal returns the documented
        envelope

- [ ] **Tests**
  - [ ] Query tests for each new query
  - [ ] `internal/httpapi/admin_users_test.go`: the suspend round trip, the
        reinstate round trip, manual verification, role change, every self-action
        guard, the last-admin guard, the reason requirement, and a ledger row per
        action
  - [ ] Regression tests in the listings, requests and offers handler tests
        proving a suspended owner's content is absent from public reads
  - [ ] Verify: `go test ./...` passes with and without a database

- [ ] **Final verification**
  - [ ] `go build ./...` and `go vet ./...` pass
  - [ ] All tests pass
  - [ ] Update `index.md` — set Phase 10 to Complete
  - [ ] Move `docs/specs/active/admin-moderation/phase-10-account-management.md` →
        `docs/specs/implemented/admin-moderation/phase-10-account-management.md`

---

## Completion Criteria

- [ ] All checklist items completed and verified
- [ ] A suspended account cannot log in, cannot refresh, and has no live session
- [ ] A suspended owner's listings, requests and offers are absent from every
      public read, and their own statuses are untouched
- [ ] Reinstating restores exactly what suspension hid
- [ ] An admin can verify an account by hand, and that account can then post
- [ ] An admin cannot suspend or demote themselves, and the last admin cannot be
      demoted
- [ ] Every action required a reason and produced exactly one ledger row
- [ ] `index.md` phase status set to Complete, phase doc moved to `implemented/`

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
