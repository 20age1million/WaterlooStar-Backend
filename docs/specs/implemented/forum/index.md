# Initial Forum — WaterlooStar API

**Status:** Complete
**Branch:** `feature/forum`
**Date:** 2026-11-17

## Purpose

Deliver this repository's initial forum functionality within the developer's
approved title/body CRUD scope. The other service has its own specification;
coordination is through `api/openapi.yaml`.

## Accepted scope and constraints

The developer selected design A and explicitly requested this initial implementation
on 2026-11-17. Public browsing and details; authenticated title/body creation;
author-only editing and deletion; loading, empty, validation and error states.
Comments, reactions, categories, moderation tools and deployment are deferred.
Keep all three preview routes unchanged until the developer approves removal.
Use local development only. Preserve uncommitted work and existing database rows.
Do not push, merge, deploy, reseed the development database, rewrite an existing
migration, or change authentication semantics.

## Design

- Contract first: public `GET /forum/posts` and `GET /forum/posts/{id}`;
  authenticated `POST /forum/posts`, `PATCH /forum/posts/{id}`, and
  `DELETE /forum/posts/{id}`. Reuse session authentication, CSRF and error envelopes.
- Reuse page/per-page metadata, stable newest-first ordering, and optional title/body
  search `q`. Return public author fields only, through the existing public owner shape.
- Request bodies contain title/body only; authorship comes from the authenticated
  principal. Trim and validate Unicode lengths: title 1–200, body 1–8,000 characters.
  PATCH accepts supplied fields and rejects null/empty/invalid changes.
- Append migration `000016_forum_posts.up.sql` and its down pair. Add a new table,
  indexes and update trigger; change no existing migrations or data. Soft-delete
  posts so deletion removes public visibility without destroying stored content.
- Enforce author ID, nondeleted post and active account in update/delete SQL, not
  merely in the frontend. Return 401 anonymously; 404 for another author's, deleted
  or missing post. Public queries exclude suspended authors in SQL.
- Exercise every generated query against isolated real PostgreSQL databases; stage
  a version-15 database to prove migration 16 preserves existing account/housing data.

## Areas

| Area | Change |
|---|---|
| Contract and generated handlers | OpenAPI forum schemas/operations and oapi-codegen output |
| Database | One additive migration pair, sqlc queries and generated types |
| HTTP/auth | New forum handlers using existing session/CSRF machinery |
| Tests | Unit, real SQL, ownership and migration-preservation tests |
| Documentation | API/setup, handoff, spec index and local run instructions |

## Phase tracker

| Phase | Status | Specification |
|---|---|---|
| 1 — initial forum api | Complete | [Phase 1](./phase-1-initial-forum-api.md) |

## Completion criteria

- Public readers and authenticated owners can use the requested flows.
- Anonymous and non-owner writes are rejected by the backend.
- Existing content and uncommitted work remain intact.
- Relevant checks and local CRUD/ownership tests are recorded.
- Local rebuild/migration/run commands are documented.
- No work outside the approved initial scope is started.

## References

- `CLAUDE.md`: project workflow, design/security conventions and gotchas.
- `docs/development.md` and `docs/setup.md` (API), or `docs/architecture.md`
  and `docs/testing.md` (Web): established implementation/testing patterns.
- `api/openapi.yaml` (API), or `../WaterlooStar-Backend/api/openapi.yaml` (Web):
  the integration contract.
