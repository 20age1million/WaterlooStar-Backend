# Live Updates

**Status:** Ready
**Branch:** `feature/live-updates`
**Ticket / Work Item:** N/A
**Owner(s):** Developer (Carl Yan), with Claude
**Date:** 2026-11-08

---

## Purpose / Goal

Tell a signed-in student's open browser tabs, within a second or two, that something
changed in one of their conversations, so the inbox, the open thread and the unread
count update without reloading or polling.

---

## Problem Statement / Motivation

Messaging (backend Phases 15–18) is on `main` and deployed, and the frontend is now
building its messaging pages (frontend `feature/messaging`, Phases 16–17). Without a
push channel, the only way a page learns about a new message is to ask again. The
developer chose a channel over polling on 2026-11-08: a conversation should feel live,
and the frontend should be built on the channel from the start, not retrofitted.

The two repositories are built at the same time. The frontend's Phase 16 reads this
stream; it can be developed against this branch's contract, and merges after this
phase is on `main`.

---

## Decisions — 2026-11-08 (developer)

| # | Question | Decision |
|---|---|---|
| D-1 | Polling or a channel? | **A channel.** |
| D-2 | Which transport? | **Server-sent events.** The browser only ever talks to the Next.js server (`deploy/docker-compose.yml`: the API has no public URL), and an App Router route handler can stream a response but cannot proxy a WebSocket upgrade. Traffic is one-way anyway: sending stays on the existing `POST` endpoints. |
| D-3 | How does a change reach the process holding the stream? | **PostgreSQL `LISTEN`/`NOTIFY`.** Signals fire only for committed changes, every API process hears every signal, and a change made by any process (`cmd/admin`, a future job) reaches browsers. Chosen over an in-process hub, whose failure with a second API process would be silent. |
| D-4 | What does an event carry? | **Thin signals.** Which conversation changed and what kind of change, plus the student's fresh unread counts. Never a message's text, never an address, never another student's id. The browser re-reads through the existing endpoints, so every visibility rule (withheld address, suspension, takedown) stays where it is. |
| D-5 | How many streams? | **One per tab, covering all the student's conversations.** It drives the header count, the inbox and the open thread together. |

---

## Proposed Solution / Design

### The stream

`GET /me/stream` answers `200` with `Content-Type: text/event-stream` and stays open.
It requires a session (anonymous: `401`, as every `/me` path). Unverified students
may listen: they can already read their conversations.

Events, each one `event:` line and one JSON `data:` line:

| Event | When | Data |
|---|---|---|
| `ready` | Once, on connect, and again after the server's listener reconnects (`resync: true`) | `{ "unread": UnreadCounts, "resync": bool }` |
| `conversation` | A conversation the student is in changed | `{ "conversation_id": uuid, "change": "opened" \| "message" \| "read" \| "closed" \| "address_shared" \| "availability" }` |
| `unread` | After every `conversation` event | `UnreadCounts` |
| *(comment)* `: ping` | Every 25 seconds of silence | none |

`resync: true` means signals may have been missed while the server's own database
listener was reconnecting: the client re-reads what it shows. A client reconnecting
after its own disconnect does the same on the `ready` it receives.

The stream **ends at the session's access-token expiry** (15 minutes,
`auth.AccessTokenTTL`), with a final `event: expired`. The browser's `EventSource`
reconnects through the Next.js server, whose middleware refreshes the session first.
So a signed-out or suspended account stops receiving within 15 minutes without the
stream checking the database for it.

Response headers: `Cache-Control: no-store`, `X-Accel-Buffering: no` (so the host's
nginx streams instead of buffering), `Connection: keep-alive`.

### What sends a signal

Database triggers, added by one migration, call `pg_notify('conversation_events', …)`
with a payload naming **one recipient user**, the conversation and the change. A change
that concerns both participants sends two notifications. Triggers, rather than calls in
Go handlers, because they cover every writer — the API, `cmd/admin`, `cmd/seed` — and
cannot be forgotten by a new handler; they fire only on commit by `NOTIFY`'s own rules.
The project already keeps a trigger for `updated_at` (`000001_init`).

| Table change | Change kind | Recipients |
|---|---|---|
| `conversations` insert | `opened` | both participants |
| `messages` insert | `message` | both (the sender's other tabs show it too) |
| `conversations.owner_read_at` / `enquirer_read_at` changes | `read` | that side only; read state is never shown to the other side |
| `conversations.closed_at` becomes set | `closed` | both |
| `conversations.address_shared_at` becomes set | `address_shared` | both |
| `listings.removed_at`, `listings.status` or `listings.address_line` changes | `availability` | both participants of every conversation about that listing |
| `users.suspended_at` changes | `availability` | the counterpart in every conversation of that user |

### Fan-out inside a process

A new package, `internal/live`:

- **`Listener`** holds one dedicated connection (outside the pool) running
  `LISTEN conversation_events`, decodes each payload and hands it to the hub. On a
  lost connection it reconnects with backoff, then tells every subscriber to resync.
- **`Hub`** maps a user id to that user's open subscriptions in this process. Each
  subscription has a small buffered channel. A subscriber too slow to keep up is
  dropped (its stream ends), never allowed to block the others; its browser
  reconnects and re-reads.
- **The stream handler** subscribes, writes `ready`, then for each signal writes
  `conversation` and a freshly counted `unread` (`CountUnreadForUser`, the query
  behind `GET /me/conversations/unread`), flushing after each event.

### Limits

- **Five concurrent streams per account per process.** A sixth answers `429`. Five
  covers a student with several tabs and a phone.
- **A connect rate limit**, `stream_connect_per_user`: 120 an hour. A tab reconnects
  about four times an hour on token expiry alone; the rest absorbs flaky networks.
- Shutdown closes every stream first (`http.Server.RegisterOnShutdown`), so
  `Shutdown` does not wait out its deadline on open streams.

---

## Layers / Areas Affected

| Layer / Area | Change |
|---|---|
| Contract (`api/openapi.yaml`) | `GET /me/stream` (`streamMyEvents`), `text/event-stream`; schemas `LiveReady`, `LiveConversationEvent`, `LiveChange` |
| Code generation | The operation is excluded from the strict server and registered on gin directly: the generated strict handler writes a body in one copy and cannot flush per event |
| Schema (`migrations/`) | Migration 15, `live_updates`: one notify function and the triggers above. No table or column changes |
| New package `internal/live` | `Listener`, `Hub`, subscription |
| HTTP (`internal/httpapi`) | The stream handler; route registration in `router.go` |
| Rate limiting (`internal/ratelimit`) | `stream_connect_per_user` |
| Startup (`cmd/api/main.go`) | Start the listener; close streams on shutdown |
| Tests | Trigger payloads against real Postgres (`PG_TEST_DSN`), hub fan-out and slow-subscriber drop, the handler's event format, heartbeat and expiry |
| Configuration | None. Heartbeat and limits are constants |
| Deployment / Infra | None required: `X-Accel-Buffering: no` keeps nginx from buffering. `docs/setup.md` notes it in case the proxy is changed |
| Operational | One extra Postgres connection per API process |

---

## Phase Tracker

| Phase | Title | Status | Location |
|---|---|---|---|
| [19 — Live updates](./phase-19-live-updates.md) | The stream, the triggers, the listener and hub | Ready | `docs/specs/active/live-updates/` |

---

## Trade-offs / Alternatives Considered

- **Polling** — rejected by the developer (D-1). It would have needed nothing new.
- **WebSockets** — rejected (D-2): the Next.js server cannot proxy the upgrade, and
  nothing needs to flow browser → server on the channel.
- **In-process hub only** — rejected (D-3): breaks silently with a second process, and
  `cmd/admin`'s changes would never reach browsers.
- **`NOTIFY` from Go handlers instead of triggers** — rejected: every writer would
  have to remember it, and moderation's writes in `internal/db` would need it too.
- **Full message payloads** — rejected (D-4): the stream would have to repeat the read
  endpoints' visibility rules.

---

## Assumptions

- The host nginx (1Panel) honours `X-Accel-Buffering: no` on a proxied response. This
  is nginx's default behaviour (`proxy_ignore_headers` is not set by 1Panel's
  template); Phase 19 verifies it in production after deploy.
- nginx's `proxy_read_timeout` is at its 60-second default or longer; the 25-second
  heartbeat stays inside it.
- `NOTIFY` volume stays small: one or two notifications per message.

---

## Constraints

- No message text, address or other student's id in any event (D-4).
- No new infrastructure (D-3).
- The contract stays the single coordination point with the frontend.

---

## Success Criteria / Definition of Done

- A message sent in one browser shows up in the other participant's open thread and
  unread count within about two seconds, through the Next.js server, locally and in
  production.
- Every change kind in the table above reaches exactly the listed recipients (tests).
- A stream ends at token expiry with `expired`; a slow subscriber is dropped without
  delaying others; shutdown does not hang on open streams.
- All phase checklists completed and verified; no regressions.

---

## References (internal only)

- `docs/specs/implemented/messaging/index.md` — conversations, the address rule, notifications
- `docs/specs/implemented/rate-limiting/index.md` — rule definitions and keys
- `internal/httpapi/conversations.go`, `messages.go` — the writes the triggers observe
- `deploy/docker-compose.yml`, `deploy/RATE-LIMITING.md` — the proxy path
- Frontend: `WaterlooStar-Frontend/docs/specs/active/messaging/` — the consumer
