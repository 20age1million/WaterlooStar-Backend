# Phase 9 — Admin identity and the ledger

**Status:** Ready
**Feature:** [Admin and Moderation](./index.md)
**Objective:** An admin can exist, prove it, and read every account — and the table that records what admins do is in place before anything can be done.

---

## Scope

### In scope

- `cmd/admin` with `promote`, `demote` and `list`: the only way the first admin
  can come to exist.
- The `admin_actions` ledger, append-only, and the helper that writes a change
  and its log row in one transaction.
- The `/admin` guard: admin-only, 404 to everyone else.
- Read-only endpoints: an overview with counts, the account list with search and
  filters, one account in detail, and the ledger itself.

### Out of scope

- Changing anything about an account — Phase 10.
- Anything to do with posts — Phase 11.
- Moderator-role behaviour. `moderator` stays an unused value in the existing
  `CHECK`; it is not a reduced admin in this feature.
- Rate limiting, which is Phase 8 and a separate feature. It is a prerequisite
  here, not a task.

---

## Dependencies / Prerequisites

- **Phase 8 — rate limiting — complete.** This phase hands out keys; that one fit
  the lock, and it is done: login allows five failed attempts per address per 15
  minutes, with a service-wide backstop behind it.
- Phase 7 — the three open pull requests (query test layer, requests, offers)
  must be merged into `main`, because the account view counts requests and the new
  queries need the test harness.
- `users.role` and its `CHECK` from migration 2, and `auth.Principal.IsAdmin()`
  from Phase 1 — both already present and unused.

---

## Implementation Steps

- [ ] **Preparation**
  - [ ] Confirm the requests and offers branches are merged into `main`
  - [ ] Confirm Phase 8 (rate limiting) is merged
  - [ ] Update `docs/specs/active/admin-moderation/index.md` — set Phase 9 to In Progress

- [ ] **Schema**
  - [ ] Add `migrations/000008_admin` — `users.suspended_at`, `suspended_by`
        (nullable self-reference, `ON DELETE SET NULL`), `suspend_reason`
  - [ ] Create `admin_actions`: id, `actor_id`, `action`, `subject_type`,
        `subject_id`, `reason`, `created_at`, and a `jsonb` `detail` for the
        before-and-after of the specific change
  - [ ] `CHECK` the known action names and subject types, so a typo in a handler
        fails at the database rather than entering the ledger
  - [ ] Index `admin_actions` by `created_at DESC`, and by `(subject_type, subject_id)`
        so "what happened to this account" is one query
  - [ ] Keep `actor_id` on `ON DELETE SET NULL` rather than cascade — the ledger
        outlives the account that acted
  - [ ] Verify: migrate up and down cleanly; confirm the down migration drops the
        table and both column sets and nothing else

- [ ] **Bootstrap CLI**
  - [ ] Add `cmd/admin` with `promote <email>`, `demote <email>`, `list`
  - [ ] Each writes an `admin_actions` row with a null actor, so a grant made from
        the host is as visible as one made in the portal
  - [ ] Refuse to demote the last remaining admin
  - [ ] Print what changed, including the previous role — a promote that was
        already an admin says so rather than reporting success
  - [ ] Verify: promote a seeded account, log in as it, decode the JWT and confirm
        `role: "admin"`; run `list` and see it

- [ ] **Queries**
  - [ ] Add `internal/db/queries/admin.sql` — `ListUsersForAdmin` (search over
        email and username, filter by role, verified and suspended, paged),
        `CountUsersForAdmin`, `GetUserForAdmin` (with listing, request and offer
        counts), `CountAdmins`, `InsertAdminAction`, `ListAdminActions`,
        `ListAdminActionsForSubject`, `AdminOverviewCounts`
  - [ ] Use the `sqlc.narg(...) IS NULL OR ...` optional-filter pattern from
        Phase 3 so one prepared statement serves every combination
  - [ ] **No update and no delete against `admin_actions`** — the absence is the
        guarantee
  - [ ] Verify: query tests; `TestEveryQueryIsExercised` names nothing new

- [ ] **Contract**
  - [ ] Add the admin schemas and paths to `api/openapi.yaml` under an `admin` tag
  - [ ] The admin account shape carries email, role, verified, suspension and
        counts — it is not the public `User` schema, which has no business
        growing an operator's fields
  - [ ] Document the 404-not-403 behaviour on the admin paths, so the contract
        states it rather than the implementation implying it
  - [ ] Verify: both generators run cleanly and idempotently

- [ ] **Guard and handlers**
  - [ ] Add `internal/httpapi/admin_guard.go`: resolve the principal, require
        `IsAdmin()`, and return the 404 refusal otherwise — one helper every admin
        handler calls, in the shape of `ownedListing`'s `*refusal`
  - [ ] Add the transactional helper that performs a change and inserts its ledger
        row together, so no handler can log and not act, or act and not log
  - [ ] Add `internal/httpapi/admin.go` (overview, ledger) and `admin_users.go`
        (list, detail)
  - [ ] Verify: by hand — as an admin, list accounts and read one; as an ordinary
        user and as an anonymous caller, every admin path answers 404 with the
        standard error envelope

- [ ] **Tests**
  - [ ] Query tests for each new query, including the filter combinations and the
        counts
  - [ ] `internal/httpapi/admin_test.go`: the guard against anonymous, ordinary
        user, and admin; search and filter behaviour; the ledger read
  - [ ] Extend the in-memory fake querier
  - [ ] A test asserting the generated `Querier` has no method that updates or
        deletes an `admin_actions` row
  - [ ] Verify: `go test ./...` passes, and passes with `PG_TEST_DSN` unset

- [ ] **Documentation**
  - [ ] `README.md`: the admin endpoints, and how to make the first admin
  - [ ] `CLAUDE.md`: the phase table, and the gotcha that `/admin` answers 404
  - [ ] Verify: a reader with a fresh clone can create an admin from the README alone

- [ ] **Final verification**
  - [ ] `go build ./...` and `go vet ./...` pass
  - [ ] All tests pass, with and without a database
  - [ ] Update `index.md` — set Phase 9 to Complete
  - [ ] Move `docs/specs/active/admin-moderation/phase-9-admin-identity.md` →
        `docs/specs/implemented/admin-moderation/phase-9-admin-identity.md`

---

## Completion Criteria

- [ ] All checklist items completed and verified
- [ ] `cmd/admin promote` grants the role, and the resulting login carries it
- [ ] The last admin cannot be demoted
- [ ] An admin can search, filter and page every account and read one in detail
- [ ] Every `/admin` path answers 404 to an anonymous caller and to a non-admin
- [ ] `admin_actions` records the CLI grants, and no code path can modify a row
- [ ] No regressions: the public surface is byte-for-byte unchanged
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
