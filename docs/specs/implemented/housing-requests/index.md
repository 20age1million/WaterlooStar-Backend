# Housing Requests — API Phases 5–7

**Status:** Complete
**Branch:** `feature/housing-requests`
**Ticket / Work Item:** N/A
**Owner(s):** WaterlooStar backend
**Date:** 2026-09-25

---

## Purpose / Goal

Let a student say what they need — term, budget, how far from campus — and let
an owner answer with a place they have. The other half of the marketplace: so
far the API only serves people who already have a room to offer.

---

## Problem Statement / Motivation

Housing Available works end to end. "Looking for Housing" does not exist at all:
the hub's second mode reads `src/fixtures/housing.ts` in the frontend, which is
invented data, and this repository has no concept of a request.

That leaves the market one-sided. A student leaving on co-op posts a room and
waits; a student arriving in January has no way to be found. The prototype's own
pitch — "read what students are looking for and offer them your room directly,
instead of waiting for someone to find your ad" — is the part that was never
built.

It also matters *now* for a reason unrelated to features: the site is live and
empty. A request costs a student two minutes and no photographs, so it is the
cheapest thing a new visitor can contribute.

---

## Proposed Solution / Design

Requests mirror listings, deliberately. Same shape of table, same discovery
machinery, same ownership rules — an owner reading requests should be able to
filter them the way a student filters listings.

### Key Components

- **`migrations/00000X_housing_requests`**: the `housing_requests` table, its
  status lifecycle and its search index.
- **`internal/db/queries/requests.sql`**: the query surface — browse with
  filters, one by id, the poster's own, create, update, set status.
- **`internal/httpapi/requests.go`, `requests_write.go`**: handlers, mirroring
  `listings.go` and `listings_write.go`.
- **`migrations/00000Y_request_offers`** and `internal/db/queries/offers.sql`:
  an owner's offer of one of their listings against a request.
- **`internal/db/dbtest`**: the missing piece — a harness that runs queries
  against a real PostgreSQL, so a malformed query fails a test rather than a
  deploy.

### Data / Control Flow

- A verified student posts a request. It is published immediately, like a
  listing, and appears in public results.
- Anyone — signed in or not — can browse requests and filter them by term,
  budget, distance, occupants and the rest. No address is involved: a request
  says what someone needs, not where they live.
- An owner offers one of **their own listings** against a request, with an
  optional note. The offer references the listing rather than repeating it, so
  the student sees real rent, dates and distance.
- If the owner has nothing suitable posted, they post a listing first and then
  offer it. That is two existing calls in sequence; the API needs nothing extra
  for the frontend to present it as one flow.
- The student sees the offers on their own request. They cannot act on them
  yet — acting means talking, and messaging does not exist.
- An offer whose listing is no longer published stops appearing. Taking a
  listing down withdraws its offers without a second mechanism.

---

## Layers / Areas Affected

| Layer / Area | Change |
|---|---|
| Database schema | `housing_requests`, `request_offers`, search index, lifecycle columns |
| Migrations | Two pairs |
| DB access layer | Request browse/detail/own/create/update/status; offer create/list/withdraw |
| Service layer | Validation, ownership, transition rules — mirroring listings |
| API handlers | Requests (7 operations), offers (3) |
| DTOs / contracts | `api/openapi.yaml`: request and offer schemas and paths |
| Auth | Verified students post requests; owners offer; 404 for someone else's |
| Tests | **New:** query tests against a real database. Handler tests as before |
| Deployment / Infra | CI gains a PostgreSQL service for the query tests |
| Operational | None — no new configuration, no new secrets |

---

## Phase Tracker

| Phase | Title | Status | Location |
|---|---|---|---|
| [5 — Query tests against a real database](./phase-5-query-test-layer.md) | The harness this feature's SQL needs before it is written | Complete | `docs/specs/implemented/housing-requests/` |
| [6 — Requests](./phase-6-requests.md) | Table, lifecycle, browse with filters, write path | Complete | `docs/specs/implemented/housing-requests/` |
| [7 — Offers](./phase-7-offers.md) | An owner offers a listing against a request | Complete | `docs/specs/implemented/housing-requests/` |

---

## Trade-offs / Alternatives Considered

- **Structured fields over free text** — chosen by the developer. Budget, dates,
  radius, occupants, pets and furnished are columns, so an owner can filter
  "who needs a place in my range for Winter". The prototype's `needs: string[]`
  reads well and answers no question.
- **Public requests** — chosen by the developer, matching listings. A request
  carries no address, so the exposure is lower than a listing's.
- **No program field** — chosen by the developer. Programs belong to user
  profiles, which do not exist; adding one here would start a profile feature
  specified nowhere.
- **An offer references a listing** — chosen by the developer, who added: the
  owner may also create one on the spot. Keeping the reference means the student
  sees a real place rather than an unverifiable pitch, and an offer cannot
  outlive the listing behind it.
- **No accept or decline** — chosen by the developer. Accepting promises a
  conversation the product cannot host yet.
- **The query test layer as this feature's first phase** — rather than a feature
  of its own. This feature adds roughly a dozen queries; Phase 4 shipped one
  that no test could catch and that broke against the real database. Building
  the harness after the queries would be writing tests for code already trusted.

---

## Assumptions

- Requests need no photographs. Nobody illustrates a need for a room.
- The discovery patterns from Phase 3 — `sqlc.narg` filters, a generated
  `tsvector`, keyset-free `LIMIT`/`OFFSET` paging — transfer to requests
  unchanged.
- Docker is available wherever the query tests run, including the Drone runner,
  which already runs Docker for every build.

---

## Constraints

- **This repository is specified on its own.** The frontend is a separate
  service and consumes this through `api/openapi.yaml`.
- The vocabulary rule stands: nothing named `renter` or `rentee`.
- Someone else's request answers 404, never 403 — as listings do.
- Only a verified student may post a request or make an offer.
- No new runtime configuration: the deployed stack must not need new secrets or
  environment variables.

---

## Success Criteria / Definition of Done

- A verified student can post, edit, pause and take down a request.
- Anyone can browse requests and filter them by term, budget, distance,
  occupants, pets and furnished, with a total that describes the whole matching
  set.
- An owner can offer one of their own listings against a request, and the
  student sees it on their request.
- An offer disappears when its listing stops being published.
- Someone else's request and someone else's offer both answer 404.
- Every query in `internal/db/queries/` is executed by a test against a real
  PostgreSQL, and the suite fails if one is malformed.
- CI runs those tests. `go test ./...` still passes with no database available.

---

## Open Questions

*None.* Every scope decision above was settled with the developer before
writing.
