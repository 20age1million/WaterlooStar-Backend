# Phase 9 — Admin identity and the ledger

**Status:** Complete
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

- [x] **Preparation**
  - [x] Confirm the requests and offers branches are merged into `main`
  - [x] Confirm Phase 8 (rate limiting) is merged — **not merged into `main`;
        contained in this branch instead.** See Implementation Notes.
  - [x] Update `docs/specs/active/admin-moderation/index.md` — set Phase 9 to In Progress
        (went straight to Complete in the phase commit)

- [x] **Schema**
  - [x] Add `migrations/000008_admin` — `users.suspended_at`, `suspended_by`
        (nullable self-reference, `ON DELETE SET NULL`), `suspend_reason`
  - [x] Create `admin_actions`: id, `actor_id`, `action`, `subject_type`,
        `subject_id`, `reason`, `created_at`, and a `jsonb` `detail` for the
        before-and-after of the specific change
  - [x] `CHECK` the known action names and subject types, so a typo in a handler
        fails at the database rather than entering the ledger
  - [x] Index `admin_actions` by `created_at DESC`, and by `(subject_type, subject_id)`
        so "what happened to this account" is one query
  - [x] Keep `actor_id` on `ON DELETE SET NULL` rather than cascade — the ledger
        outlives the account that acted
  - [x] Verify: migrate up and down cleanly; confirm the down migration drops the
        table and both column sets and nothing else

- [x] **Bootstrap CLI**
  - [x] Add `cmd/admin` with `promote <email>`, `demote <email>`, `list`
  - [x] Each writes an `admin_actions` row with a null actor, so a grant made from
        the host is as visible as one made in the portal
  - [x] Refuse to demote the last remaining admin
  - [x] Print what changed, including the previous role — a promote that was
        already an admin says so rather than reporting success
  - [x] Verify: promote a seeded account, log in as it, decode the JWT and confirm
        `role: "admin"`; run `list` and see it

- [x] **Queries**
  - [x] Add `internal/db/queries/admin.sql` — `ListUsersForAdmin` (search over
        email and username, filter by role, verified and suspended, paged),
        `CountUsersForAdmin`, `GetUserForAdmin` (with listing, request and offer
        counts), `CountAdmins`, `InsertAdminAction`, `ListAdminActions`,
        `ListAdminActionsForSubject`, `AdminOverviewCounts`
  - [x] Use the `sqlc.narg(...) IS NULL OR ...` optional-filter pattern from
        Phase 3 so one prepared statement serves every combination
  - [x] **No update and no delete against `admin_actions`** — the absence is the
        guarantee
  - [x] Verify: query tests; `TestEveryQueryIsExercised` names nothing new

- [x] **Contract**
  - [x] Add the admin schemas and paths to `api/openapi.yaml` under an `admin` tag
  - [x] The admin account shape carries email, role, verified, suspension and
        counts — it is not the public `User` schema, which has no business
        growing an operator's fields
  - [x] Document the 404-not-403 behaviour on the admin paths, so the contract
        states it rather than the implementation implying it
  - [x] Verify: both generators run cleanly and idempotently

- [x] **Guard and handlers**
  - [x] Add `internal/httpapi/admin_guard.go`: resolve the principal, require
        `IsAdmin()`, and return the 404 refusal otherwise — one helper every admin
        handler calls, in the shape of `ownedListing`'s `*refusal`
  - [x] Add the transactional helper that performs a change and inserts its ledger
        row together, so no handler can log and not act, or act and not log
  - [x] Add `internal/httpapi/admin.go` (overview, ledger) and `admin_users.go`
        (list, detail)
  - [x] Verify: by hand — as an admin, list accounts and read one; as an ordinary
        user and as an anonymous caller, every admin path answers 404 with the
        standard error envelope

- [x] **Tests**
  - [x] Query tests for each new query, including the filter combinations and the
        counts
  - [x] `internal/httpapi/admin_test.go`: the guard against anonymous, ordinary
        user, and admin; search and filter behaviour; the ledger read
  - [x] Extend the in-memory fake querier
  - [x] A test asserting the generated `Querier` has no method that updates or
        deletes an `admin_actions` row
  - [x] Verify: `go test ./...` passes, and passes with `PG_TEST_DSN` unset

- [x] **Documentation**
  - [x] `README.md`: the admin endpoints, and how to make the first admin
  - [x] `CLAUDE.md`: the phase table, and the gotcha that `/admin` answers 404
  - [x] Verify: a reader with a fresh clone can create an admin from the README alone

- [x] **Final verification**
  - [x] `go build ./...` and `go vet ./...` pass
  - [x] All tests pass, with and without a database
  - [x] Update `index.md` — set Phase 9 to Complete
  - [x] Move `docs/specs/active/admin-moderation/phase-9-admin-identity.md` →
        `docs/specs/implemented/admin-moderation/phase-9-admin-identity.md`

---

## Completion Criteria

- [x] All checklist items completed and verified
- [x] `cmd/admin promote` grants the role, and the resulting login carries it
- [x] The last admin cannot be demoted
- [x] An admin can search, filter and page every account and read one in detail
- [x] Every `/admin` path answers 404 to an anonymous caller and to a non-admin
- [x] `admin_actions` records the CLI grants, and no code path can modify a row
- [x] No regressions: the public surface is byte-for-byte unchanged
- [x] `index.md` phase status set to Complete, phase doc moved to `implemented/`

---

## Implementation Notes

**Key files changed:**
- `migrations/000008_admin.{up,down}.sql`: suspension columns on `users` (with a
  CHECK that the time and reason are set together), and `admin_actions` with its
  CHECKs and both indexes.
- `internal/db/queries/admin.sql`: `ListUsersForAdmin`, `CountUsersForAdmin`,
  `GetUserForAdmin`, `CountAdmins`, `SetUserRole`, `InsertAdminAction`,
  `ListAdminActions`, `CountAdminActions`, `ListAdminActionsForSubject`,
  `AdminOverviewCounts`.
- `internal/db/audit.go`: `TxRunner` (`PoolTx`, and `Direct` for the in-memory
  fake), the ledger vocabulary as constants, and `Audited`, the helper that runs a
  change and its ledger row in one transaction.
- `internal/db/roles.go`: `ChangeRole`, with the last-admin guard, the
  no-op refusal, and session revocation. Shared by `cmd/admin` and Phase 10.
- `cmd/admin/main.go`: `promote`, `demote`, `list`.
- `api/openapi.yaml`: the `admin` tag; `GET /admin/overview`, `/admin/users`,
  `/admin/users/{id}`, `/admin/actions`; `AdminUser`, `AdminUserPage`,
  `AdminUserDetail`, `AdminAction`, `AdminActor`, `AdminActionPage`,
  `AdminOverview`, `AdminUserCounts`, `AdminPostCounts`.
- `internal/httpapi/admin_guard.go`, `admin.go`, `admin_users.go`: the guard and
  the four handlers. `router.go`: the `Server` carries a `TxRunner` for Phase 10.
- `sqlc.yaml`: fixed the nullable-uuid override (see divergences).
- Tests: `internal/db/queries_admin_test.go`, `cmd/admin/main_test.go`,
  `internal/httpapi/admin_test.go`, `fake_querier_admin_test.go`.
- `README.md`, `CLAUDE.md`.

**Divergences from plan:**
- **Phase 8 is not merged into `main`.** The preparation step asked for it. It is
  merged into this branch (`b86b43c`), so this branch cannot reach `main` without
  it and the ordering the constraint protects still holds. Merge
  `feature/rate-limiting` first, then this.
- **The 404 is written by the guard, not returned as the generated 404 type.** The
  first version returned `gen.*404JSONResponse` with the NoRoute wording. Checked by
  hand, it differed from a real unknown path in `Content-Type` (no charset) and a
  trailing newline, so a prober could still tell the paths apart. `requireAdmin`
  now writes through `apierror.NotFound`, as `NoRoute` does, and the handler
  returns `unrouted{}`. The test compares headers and raw bytes.
- **The ledger's action list covers phases 9–11 now:** `set_role`, `suspend`,
  `reinstate`, `verify`, `remove_post`, `restore_post`. It saves a migration that
  would only widen a CHECK. Promote and demote are both `set_role`, with
  `{"from", "to"}` in `detail`.
- **`cmd/admin` takes `-reason`, defaulting to "Changed from the host with
  cmd/admin".** The ledger requires a reason; the spec's `promote <email>` form
  still works.
- **Every role change revokes the account's refresh tokens**, promotion included,
  so the same rule applies both ways. Phase 10 already required it for the portal.
- **`CountAdminActions` was added** so the ledger page can report a total, like
  every other paged endpoint.
- **`sqlc.yaml` had a latent bug.** The nullable-uuid override said
  `type: "uuid.UUID"` together with `import`, which generates `*uuid.uuid.UUID`.
  Nothing hit it until `suspended_by` and `actor_id`, the first nullable uuid
  columns. Now `type: "UUID"`.

**Verification run:**
- `go build ./...`, `go vet ./...`: clean.
- `go test ./...` with `PG_TEST_DSN` unset: pass (query tests skip).
- `go test ./...` with `PG_TEST_DSN` set: pass, including
  `TestEveryQueryIsExercised`, the transaction rollback tests and the ledger
  CHECK tests.
- Both generators run twice: no diff on the second run.
- Migrations on the development database: `up` to 8, `down-one` to 7 (the table,
  the three columns and the CHECK gone; `users` back to its 11 columns), `up` to 8.
- By hand against a running API: `cmd/admin promote meil@uwaterloo.ca`, then
  login. The access JWT decodes to `"role":"admin"`. `/admin/users?q=priya`,
  `/admin/overview` and `/admin/actions` answer; the ledger shows the promotion
  with `actor: null`. Anonymous and ordinary-user calls get 404 with the same
  headers, byte length and wording as `/admin/nothing-here`.
- Mutation checks: dropping `IsAdmin()` from the guard fails
  `TestAdminSurfaceIsInvisibleToOrdinaryUsers`; changing the refusal's
  Content-Type fails the header comparison.
- While verifying, `go run ./cmd/migrate down 1` rolled back **every** migration,
  because `down` ignores a count (`down-one` is one step). That wiped the local
  development database, and it was re-seeded. `CLAUDE.md` now records it.
