# Phase 1 — Initial Forum Api

**Status:** Complete
**Feature:** [Initial Forum](./index.md)

## Scope

Implement only this repository's initial forum scope described in the accepted
index. Public read; authenticated owner CRUD; real data and complete UI/API states.

## Checklist

- [x] Confirm branch, prerequisite inspection, preserved-file snapshots and contract.
- [x] Edit the OpenAPI contract before implementation and regenerate server types.
- [x] Add migration 16 and sqlc queries without changing any migration 1–15.
- [x] Implement public read and authenticated create/update/delete handlers.
- [x] Validate input, derive authorship from the session, and enforce ownership in SQL.
- [x] Cover anonymous refusal, forged authorship, non-owner refusal, soft deletion, missing/deleted posts, active-account and input rules.
- [x] Run every new query against real PostgreSQL and test migration preservation from version 15.
- [x] Run generators, Go tests with PG_TEST_DSN, go vet and build.
- [x] Verify preservation of all original uncommitted files and preview files.
- [x] Update handoff documentation, README, relevant guides and docs/specs/INDEX.md.
- [x] Record verification outcomes and limitations in Implementation Notes.
- [x] Mark the phase/index complete and move both documents to docs/specs/implemented/forum/.
- [x] Commit only this phase's changes locally; exclude pre-existing work.

## Completion criteria

Every checklist item is complete or has a concrete recorded verification limit.
The forum initial scope is usable locally, with no push, merge or deployment.

## Implementation Notes

Implemented the five contract-first forum operations, migration16, six sqlc
queries, generated database/server code, active-user/session-derived authorship,
Unicode field validation, privacy filters and SQL-bound ownership. Creation
reuses the existing WritePerUser allowance; edits/deletes/reads stay unaffected.

Passed generators, full `go test ./...` (including an uncached run), `go vet
./...`, `go build ./...`, focused forum/rate-limit tests and the complete
`PG_TEST_DSN=... go test ./internal/db/... -count=1` suite. The full Go runs did
not set PG_TEST_DSN; real SQL tests were separately run with it and all passed.
The staged15→16 test compares existing users, listings, requests, conversations
and messages exactly. Every new generated query is exercised.

A real keyless API against a separate QA database passed public reads, login,
create/edit/delete, author-spoof prevention, non-owner404, anonymous401, CSRF403,
blank/null/oversized-field400, Unicode boundary acceptance and deletion visibility.
Stored deleted content was verified retained. The local Docker image was rebuilt,
migration16 applied to the original development volume, health/forum endpoints
checked, and original entity counts compared unchanged. A pre-migration backup
was saved outside the repositories. Original migration1–15 contents and untracked
.DS_Store were preserved exactly.

All changes are committed locally with pre-existing work excluded. No push,
merge, deploy or later forum phase was started. Temporary QA services/database
were cleaned up; the development Docker API/database remain running.
