# Phase 19 — Live updates

**Status:** Complete (2026-11-08, on `feature/live-updates`)
**Feature:** [Live Updates](./index.md)
**Objective:** `GET /me/stream` tells a signed-in student's open tabs, as server-sent events, which of their conversations changed and their fresh unread counts, fed by PostgreSQL `LISTEN`/`NOTIFY`.

---

## Scope

### In scope

- Migration 15, `live_updates`: the notify function and triggers in the index's table.
- `internal/live`: `Listener` (dedicated connection, reconnect with backoff, resync),
  `Hub` (per-user subscriptions, buffered, slow subscribers dropped).
- `GET /me/stream` (`streamMyEvents`) in the contract and the handler: `ready`,
  `conversation`, `unread`, `expired`, a 25-second heartbeat.
- Limits: five concurrent streams per account per process (`429` beyond), the
  `stream_connect_per_user` rate limit (120 an hour).
- Startup and shutdown wiring in `cmd/api/main.go`.
- Documentation: `docs/api.md` (the stream and its events), `docs/setup.md` (the proxy
  note), `CLAUDE.md`, `docs/specs/INDEX.md`, `docs/ROADMAP.md`.

### Out of scope

- Typing indicators, presence and read receipts shown to the other side.
- Events for anything but conversations (listings, requests, offers on their own).
- Browser → server traffic on the stream.
- Any frontend work: frontend `feature/messaging`, Phase 16.

---

## Dependencies / Prerequisites

- Messaging Phases 15–18 on `main` (merged in #17 and #21).
- `PG_TEST_DSN` pointing at a throwaway `postgres:18` for the trigger tests.

---

## Implementation Steps

- [x] **Preparation**
  - [x] Record #21's merge in `CLAUDE.md` and clear its in-flight entry (the next
        work starts by recording the last merge)
  - [x] Update `docs/specs/active/live-updates/index.md` — set Phase 19 to In Progress

- [x] **Schema**
  - [x] Add `migrations/000015_live_updates.up.sql`: a function
        `notify_conversation_event(recipient uuid, conversation uuid, change text)`
        calling `pg_notify('conversation_events', json_build_object('u', recipient,
        'c', conversation, 'k', change)::text)`, and `AFTER` row triggers for each row of
        the index's table, comparing `OLD`/`NEW` so an unrelated update sends nothing
  - [x] Add the matching `.down.sql` dropping the triggers and the function
  - [x] Verify: `go run ./cmd/migrate up` then `down 1` then `up` on a scratch database

- [x] **Contract**
  - [x] Add `GET /me/stream` to `api/openapi.yaml` under `messages`: `200` as
        `text/event-stream` documenting each event and its schema, `401`, `429`
  - [x] Add `LiveReady`, `LiveConversationEvent` and `LiveChange` (enum) to
        `components/schemas`, reusing `UnreadCounts`
  - [x] Exclude `streamMyEvents` from code generation in `api/oapi-codegen.yaml`
        (`output-options.exclude-operation-ids`) and regenerate
  - [x] Verify: `go generate ./...` succeeds; the schemas are generated, the operation is not

- [x] **Fan-out (`internal/live`)**
  - [x] `hub.go`: `Subscribe(user) (*Subscription, error)` refusing a sixth per user,
        `Publish(signal)`, `Resync()`, `Close()`; each subscription a buffered channel
        (16); a full channel ends that subscription
  - [x] `listener.go`: dedicated `pgx.Conn` from `PG_DSN`, `LISTEN conversation_events`,
        `WaitForNotification` loop, payload decoding (a malformed payload is logged and
        skipped), reconnect with capped backoff, `Resync()` after reconnecting
  - [x] Verify: unit tests for subscribe/limit/publish/drop/close

- [x] **HTTP**
  - [x] `internal/httpapi/stream.go`: the handler — principal required, connect rate
        limit, subscribe, headers, `ready`, then events and heartbeat until the
        request context ends, the subscription ends, or the access token's expiry
        (`expired`), flushing after each write
  - [x] Register `GET /me/stream` in `router.go` after the middleware stack (so
        `Authenticate`, rate limiting and logging apply)
  - [x] Add `StreamConnectPerUser` to `internal/ratelimit/policy.go` and its route rule
  - [x] `cmd/api/main.go`: start the listener with the server's context; register hub
        closing with `srv.RegisterOnShutdown`
  - [x] Verify: `curl -N -b <cookie> localhost:8080/me/stream` shows `ready`, a ping
        within 25 s, and a `conversation` + `unread` pair when a message is sent from
        another session

- [x] **Tests**
  - [x] `internal/db/queries_live_test.go` (real Postgres): each table change in the
        index yields exactly the listed `(recipient, conversation, change)` payloads,
        and an unrelated update (e.g. a title edit) yields none
  - [x] `internal/live/*_test.go`: hub fan-out to several subscriptions of one user,
        isolation between users, the per-user limit, slow-subscriber drop, resync
  - [x] `internal/httpapi/stream_test.go`: `401` anonymous; event framing; heartbeat
        (with an injectable clock or short interval); `expired` at token expiry; `429`
        on the sixth stream; no message text or address in any event
  - [x] Verify: `go test -count=1 ./...` with `PG_TEST_DSN` set passes

- [x] **Final verification**
  - [x] `go build ./...`, `go vet ./...` pass
  - [x] All relevant tests pass
  - [x] In the local Docker stack, two browsers through the frontend (or two `curl -N`
        sessions) see a message's events within about two seconds
  - [x] Update the handoff documentation so another agent can pick up from the files
        alone: `CLAUDE.md` (what is built, what is in flight, what comes next, decisions
        and gotchas from this phase), `README.md`, `docs/api.md`, `docs/setup.md`,
        `docs/specs/INDEX.md`, `docs/ROADMAP.md`
  - [x] Update `index.md` — set Phase 19 to Complete
  - [x] Move `docs/specs/active/live-updates/phase-19-live-updates.md` →
        `docs/specs/implemented/live-updates/phase-19-live-updates.md`
  - [x] *(Final phase)* Move `docs/specs/active/live-updates/index.md` →
        `docs/specs/implemented/live-updates/index.md`
  - [ ] After deploy: confirm events arrive through production nginx without delay
        (the `X-Accel-Buffering` assumption) — **deferred:** possible only once
        merged and deployed; recorded in `CLAUDE.md`

---

## Completion Criteria

- [x] All checklist items completed and verified
- [x] No regressions in related areas
- [x] Handoff documentation current: `CLAUDE.md`, `README.md`, `docs/specs/INDEX.md`
- [x] `index.md` phase status set to Complete
- [x] Phase spec doc moved to `docs/specs/implemented/live-updates/`
- [x] *(Final phase)* `index.md` moved to `docs/specs/implemented/live-updates/`

---

## Implementation Notes

**Key files changed:**
- `migrations/000015_live_updates.{up,down}.sql`: `notify_conversation_event` and
  five triggers (conversations insert and update, messages insert, listings
  update, users update), each comparing `OLD`/`NEW` so unrelated updates send
  nothing.
- `api/openapi.yaml`: `GET /me/stream` (`streamMyEvents`, tag `messages`),
  `LiveReady`, `LiveChange`, `LiveConversationEvent`.
- `api/oapi-codegen.yaml`: `exclude-operation-ids: [streamMyEvents]` and
  `skip-prune: true`.
- `internal/live/hub.go`, `listener.go`: the hub (per-user subscriptions,
  buffer 16, slow subscriber dropped, resync, close) and the listener (dedicated
  `pgx.Conn`, `LISTEN conversation_events`, reconnect with backoff 0.5–30 s,
  resync after reconnecting).
- `internal/httpapi/stream.go`: the handler; `router.go`: the hub and heartbeat on
  `Server`, `LiveHub()`, `SetStreamHeartbeat()`, the hand-registered route.
- `internal/auth/token.go`: `Principal.ExpiresAt`, set from the token's `exp`.
- `internal/ratelimit/policy.go`, `internal/httpapi/ratelimit_mw.go`:
  `stream_connect_per_user`, 120 an hour.
- `cmd/api/main.go`: starts the listener; `RegisterOnShutdown(hub.Close)`.
- Tests: `internal/db/queries_live_test.go`, `internal/live/hub_test.go`,
  `internal/live/listener_test.go`, `internal/httpapi/stream_test.go`;
  `internal/auth/auth_test.go` adjusted for `ExpiresAt`.
- Docs: `docs/api.md` ("Live updates", the endpoint and rate-limit rows),
  `docs/setup.md` (keyless local API for messaging; proxy note), `CLAUDE.md`,
  `docs/ROADMAP.md`, `docs/specs/INDEX.md`. Also recorded #21's merge in
  `CLAUDE.md` (the preparation step).

**Divergences from plan:**
- **`skip-prune: true` added to the codegen config.** Excluding the operation left
  its schemas unreferenced, and oapi-codegen pruned them silently. It also
  generates one unused alias, `Forbidden`, harmless.
- **The token expiry rides on `auth.Principal`** (`ExpiresAt`) rather than being
  re-parsed from the cookie in the handler. The spec did not say how the handler
  learns the expiry.
- **Opening a conversation sends `opened` and then `message`** (its first message
  is a separate insert). Clients re-read on each; harmless.
- **A sixth stream answers 429 with `Retry-After: 60`**, via the standard 429
  helper; a closed hub (shutdown) answers 503.

**Verification run:**
- `go build ./...`, `go vet` on every changed package: pass.
- `PG_TEST_DSN=… go test -count=1 ./...` against a throwaway `postgres:18`: every
  package passes, including the trigger tests (each change kind, the exact
  recipients, unrelated updates silent, a rolled-back insert silent) and the
  listener test against a real database. The first run failed only because the
  test's takedown and suspension SQL omitted the reason the schema requires;
  fixed in the test.
- `go run ./cmd/migrate up`, `down 1` (0 live triggers left), `up` (5 again) on a
  scratch database.
- The local Docker stack rebuilt from this branch, **keyless**
  (`RESEND_API_KEY= EMAIL_FROM= docker compose up -d --build`; it had held a real
  key). Seeded `meil` held `curl -N /me/stream` open while seeded `danielo` opened
  a conversation and sent a message: `ready`, then `conversation` `opened`,
  `unread`, `conversation` `message`, `unread`, `conversation` `message`, `unread`
  arrived within the same second. No event contained message text.
- Not run: CI's `go test -race` (no C toolchain on this machine; Drone runs it).
- Not yet possible: the production nginx check (after deploy).
