# Phase 1 — Initial Forum Api

**Status:** Ready
**Feature:** [Initial Forum](./index.md)

## Scope

Implement only this repository's initial forum scope described in the accepted
index. Public read; authenticated owner CRUD; real data and complete UI/API states.

## Checklist

- [ ] Confirm branch, prerequisite inspection, preserved-file snapshots and contract.
- [ ] Edit the OpenAPI contract before implementation and regenerate server types.
- [ ] Add migration 16 and sqlc queries without changing any migration 1–15.
- [ ] Implement public read and authenticated create/update/delete handlers.
- [ ] Validate input, derive authorship from the session, and enforce ownership in SQL.
- [ ] Cover anonymous refusal, forged authorship, non-owner refusal, soft deletion, missing/deleted posts, active-account and input rules.
- [ ] Run every new query against real PostgreSQL and test migration preservation from version 15.
- [ ] Run generators, Go tests with PG_TEST_DSN, go vet and build.
- [ ] Verify preservation of all original uncommitted files and preview files.
- [ ] Update handoff documentation, README, relevant guides and docs/specs/INDEX.md.
- [ ] Record verification outcomes and limitations in Implementation Notes.
- [ ] Mark the phase/index complete and move both documents to docs/specs/implemented/forum/.
- [ ] Commit only this phase's changes locally; exclude pre-existing work.

## Completion criteria

Every checklist item is complete or has a concrete recorded verification limit.
The forum initial scope is usable locally, with no push, merge or deployment.

## Implementation Notes

To be completed after verification. Future forum scope requires a new request.
