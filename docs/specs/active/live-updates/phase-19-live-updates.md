# Phase 19 — Live updates

**Status:** Ready
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

- [ ] **Preparation**
  - [ ] Record #21's merge in `CLAUDE.md` and clear its in-flight entry (the next
        work starts by recording the last merge)
  - [ ] Update `docs/specs/active/live-updates/index.md` — set Phase 19 to In Progress

- [ ] **Schema**
  - [ ] Add `migrations/000015_live_updates.up.sql`: a function
        `notify_conversation_event(recipient uuid, conversation uuid, change text)`
        calling `pg_notify('conversation_events', json_build_object('u', recipient,
        'c', conversation, 'k', change)::text)`, and `AFTER` row triggers for each row of
        the index's table, comparing `OLD`/`NEW` so an unrelated update sends nothing
  - [ ] Add the matching `.down.sql` dropping the triggers and the function
  - [ ] Verify: `go run ./cmd/migrate up` then `down 1` then `up` on a scratch database

- [ ] **Contract**
  - [ ] Add `GET /me/stream` to `api/openapi.yaml` under `messages`: `200` as
        `text/event-stream` documenting each event and its schema, `401`, `429`
  - [ ] Add `LiveReady`, `LiveConversationEvent` and `LiveChange` (enum) to
        `components/schemas`, reusing `UnreadCounts`
  - [ ] Exclude `streamMyEvents` from code generation in `api/oapi-codegen.yaml`
        (`output-options.exclude-operation-ids`) and regenerate
  - [ ] Verify: `go generate ./...` succeeds; the schemas are generated, the operation is not

- [ ] **Fan-out (`internal/live`)**
  - [ ] `hub.go`: `Subscribe(user) (*Subscription, error)` refusing a sixth per user,
        `Publish(signal)`, `Resync()`, `Close()`; each subscription a buffered channel
        (16); a full channel ends that subscription
  - [ ] `listener.go`: dedicated `pgx.Conn` from `PG_DSN`, `LISTEN conversation_events`,
        `WaitForNotification` loop, payload decoding (a malformed payload is logged and
        skipped), reconnect with capped backoff, `Resync()` after reconnecting
  - [ ] Verify: unit tests for subscribe/limit/publish/drop/close

- [ ] **HTTP**
  - [ ] `internal/httpapi/stream.go`: the handler — principal required, connect rate
        limit, subscribe, headers, `ready`, then events and heartbeat until the
        request context ends, the subscription ends, or the access token's expiry
        (`expired`), flushing after each write
  - [ ] Register `GET /me/stream` in `router.go` after the middleware stack (so
        `Authenticate`, rate limiting and logging apply)
  - [ ] Add `StreamConnectPerUser` to `internal/ratelimit/policy.go` and its route rule
  - [ ] `cmd/api/main.go`: start the listener with the server's context; register hub
        closing with `srv.RegisterOnShutdown`
  - [ ] Verify: `curl -N -b <cookie> localhost:8080/me/stream` shows `ready`, a ping
        within 25 s, and a `conversation` + `unread` pair when a message is sent from
        another session

- [ ] **Tests**
  - [ ] `internal/db/queries_live_test.go` (real Postgres): each table change in the
        index yields exactly the listed `(recipient, conversation, change)` payloads,
        and an unrelated update (e.g. a title edit) yields none
  - [ ] `internal/live/*_test.go`: hub fan-out to several subscriptions of one user,
        isolation between users, the per-user limit, slow-subscriber drop, resync
  - [ ] `internal/httpapi/stream_test.go`: `401` anonymous; event framing; heartbeat
        (with an injectable clock or short interval); `expired` at token expiry; `429`
        on the sixth stream; no message text or address in any event
  - [ ] Verify: `go test -count=1 ./...` with `PG_TEST_DSN` set passes

- [ ] **Final verification**
  - [ ] `go build ./...`, `go vet ./...` pass
  - [ ] All relevant tests pass
  - [ ] In the local Docker stack, two browsers through the frontend (or two `curl -N`
        sessions) see a message's events within about two seconds
  - [ ] Update the handoff documentation so another agent can pick up from the files
        alone: `CLAUDE.md` (what is built, what is in flight, what comes next, decisions
        and gotchas from this phase), `README.md`, `docs/api.md`, `docs/setup.md`,
        `docs/specs/INDEX.md`, `docs/ROADMAP.md`
  - [ ] Update `index.md` — set Phase 19 to Complete
  - [ ] Move `docs/specs/active/live-updates/phase-19-live-updates.md` →
        `docs/specs/implemented/live-updates/phase-19-live-updates.md`
  - [ ] *(Final phase)* Move `docs/specs/active/live-updates/index.md` →
        `docs/specs/implemented/live-updates/index.md`
  - [ ] After deploy: confirm events arrive through production nginx without delay
        (the `X-Accel-Buffering` assumption)

---

## Completion Criteria

- [ ] All checklist items completed and verified
- [ ] No regressions in related areas
- [ ] Handoff documentation current: `CLAUDE.md`, `README.md`, `docs/specs/INDEX.md`
- [ ] `index.md` phase status set to Complete
- [ ] Phase spec doc moved to `docs/specs/implemented/live-updates/`
- [ ] *(Final phase)* `index.md` moved to `docs/specs/implemented/live-updates/`

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
