# Feature Specifications — WaterlooStar API

Every feature is specified before it is built, and the specification is committed
as the first meaningful commit on its branch. `active/` holds work in progress;
`implemented/` holds what has shipped.

Each feature is a folder: `index.md` for the whole feature, `phase-N-*.md` for
each independently committable phase.

**This repository is specified on its own.** The frontend is a separate service with
its own phases and its own specs; the two are coordinated through
`api/openapi.yaml` alone. Platform Foundation predates this and was mirrored
across both; everything from Phase 4 onward is not.

## Active

| Feature | Phases | Status | Location |
|---|---|---|---|
| [Messaging with Contact Privacy](./active/messaging/index.md) | 4 (15–18) | In progress: 15 and 16 complete | `docs/specs/active/messaging/` |

### Messaging with Contact Privacy

A verified student messages a listing's owner inside WaterlooStar, and an owner
answers a housing request with a conversation. Nobody's email or phone is ever
shown; the exact address reaches a student only when the owner shares it in
their conversation. Phase 15 is built on `feature/messaging` (pull request
#17) and Phase 16 on `feature/messaging-privacy`, stacked on it; neither is
merged yet. Their phase docs are under `implemented/messaging/`, while
`index.md` and the later phases stay in `active/` until the feature is done.

| Phase | Title | Status |
|---|---|---|
| 15 | [Conversations and messages](./implemented/messaging/phase-15-conversations.md) | Complete: start, inbox, unread counts, close; migration 11; eight endpoints |
| 16 | [Contact privacy and address sharing](./implemented/messaging/phase-16-contact-privacy.md) | Complete: the exact address off every public read and out of search; `cross_streets`; the owner's `POST /conversations/{id}/share-address`; migration 12; one endpoint |
| 17 | [Offers open conversations](./active/messaging/phase-17-offer-conversations.md) | Ready |
| 18 | [Message notifications](./active/messaging/phase-18-message-notifications.md) | Ready |

## Implemented

| Feature | Phases | Location |
|---|---|---|
| [Platform Foundation](./implemented/platform-foundation/index.md) | 3 (0–2) | `docs/specs/implemented/platform-foundation/` |
| [Discovery](./implemented/discovery/index.md) | 1 (Phase 3) | `docs/specs/implemented/discovery/` |
| [Listing Write Path](./implemented/listing-write-path/index.md) | 1 (Phase 4) | `docs/specs/implemented/listing-write-path/` |
| [Housing Requests](./implemented/housing-requests/index.md) | 3 (5–7) | `docs/specs/implemented/housing-requests/` |
| [Rate Limiting](./implemented/rate-limiting/index.md) | 1 (Phase 8) | `docs/specs/implemented/rate-limiting/` |
| [Admin and Moderation](./implemented/admin-moderation/index.md) | 3 (9–11) | `docs/specs/implemented/admin-moderation/` |
| [Listing Terms](./implemented/listing-terms/index.md) | 1 (Phase 12) | `docs/specs/implemented/listing-terms/` |
| [Email Delivery](./implemented/email-delivery/index.md) | 2 (13–14) | `docs/specs/implemented/email-delivery/` |

### Platform Foundation

Empty repository to a service that authenticates verified University of Waterloo
students and serves housing listings, under a spec-first OpenAPI contract.

| Phase | Title | Delivered |
|---|---|---|
| 0 | [Contract and scaffolding](./implemented/platform-foundation/phase-0-contract-and-scaffolding.md) | gin service, migration harness, sqlc + oapi-codegen chain, `/healthz` |
| 1 | [Accounts and verified students](./implemented/platform-foundation/phase-1-accounts-and-verification.md) | Registration gated on `uwaterloo.ca`, email verification, login, sessions, password reset |
| 2 | [Listings read path](./implemented/platform-foundation/phase-2-listings-read-path.md) | Structured listing schema, seed data, `GET /listings` and `GET /listings/{id}` |

### Discovery

`GET /listings` grew every filter the hub's rail offers — full-text search over a
weighted `tsvector`, term window, price, distance, bedrooms, amenities, utilities,
verified-owner and four sorts — all optional and composing through one prepared
statement.

### Listing Write Path

Verified students post, edit, pause, republish and take down their own listings.
`DELETE` archives rather than removes, because questions and saves hang off the
row. Someone else's listing answers 404, never 403.

### Housing Requests

The other side of the market, in three phases.

| Phase | Title | Delivered |
|---|---|---|
| 5 | [Query tests against a real database](./implemented/housing-requests/phase-5-query-test-layer.md) | `internal/db/dbtest`, every query executed against PostgreSQL, and a guard that fails when one has no test |
| 6 | [Requests](./implemented/housing-requests/phase-6-requests.md) | `housing_requests`, seven operations, discovery filters that read from the owner's side |
| 7 | [Offers](./implemented/housing-requests/phase-7-offers.md) | An owner answers with one of their own listings; offers die with the listing behind them |

### Rate Limiting

Login, registration, password reset, token redemption and the three write
endpoints answer 429 with a `Retry-After` when an allowance is spent. Keyed on
identity rather than IP address, because the API has no public URL and every
request reaches it from the frontend. Only a *failed* login is charged, and
nothing is ever locked — allowances refill on a clock.

### Admin and Moderation

The site gains an operator, in three phases. Rate limiting was its prerequisite
and shipped first: the admin surface should not exist in front of an unlimited
login form.

| Phase | Title | Delivered |
|---|---|---|
| 9 | [Admin identity and the ledger](./implemented/admin-moderation/phase-9-admin-identity.md) | `cmd/admin`, the append-only `admin_actions` ledger, the `/admin` guard, read-only account views |
| 10 | [Account management](./implemented/admin-moderation/phase-10-account-management.md) | Suspend, reinstate, verify by hand, change role; suspension hides content and ends sessions |
| 11 | [Post moderation](./implemented/admin-moderation/phase-11-post-moderation.md) | Every post in any state; takedown and restore on their own axis, separate from the owner's status |

Every action needs a reason, writes one ledger row in the same transaction, and
has an inverse. `/admin` answers exactly as an unknown path does to anyone who
is not an admin.

### Listing Terms

An owner can allow shorter stays, with a minimum, and give an estimate of the
bills the rent leaves out. Listings report `all_in_cents`, null when no estimate
was given. `GET /listings` gains `all_in_max_cents`, `sort=allInAsc`, and a `match`
ranking that puts exact-window listings first. `GET /listings/availability`
counts what is open in each month of a year. Search stays full-cover.

### Email Delivery

Verification and reset emails reach the inbox, through Resend's API called with
`net/http` (13); without `RESEND_API_KEY` the log stays the sender. A student
whose link is lost or expired asks for a new one with `POST /auth/verify/resend`,
which answers 202 for every address (14). Clerk was the first provider and its
endpoint proved closed to the account.

## Decisions carried forward

Recorded in the Platform Foundation `index.md` and still in force:

- UUID primary keys throughout.
- Spec-first OpenAPI: `api/openapi.yaml` is edited before the handler.
- Sessions are a JWT in an httpOnly cookie, with CSRF on unsafe methods.
- pgx + sqlc, no ORM; golang-migrate owns the schema.
- *renter* and *rentee* are internal words only.

## Open, deliberately deferred

- **Image-hosting and map/geocoding providers.** The developer chooses them when
  those features start; [the roadmap](../ROADMAP.md#providers-still-to-be-chosen)
  records the decision. Placeholders remain behind interfaces. Email is Resend.

## Decisions settled — 2026-10-14

**Address privacy:** show only an approximate location; the exact address is
revealed only when the listing's owner shares it in a conversation (refined
2026-10-16), and nothing but the address is ever revealed. Implemented in
messaging Phase 16 (on `feature/messaging-privacy`, not yet merged): public
reads carry `neighbourhood` and `cross_streets`, never `address_line` or
coordinates, and search no longer indexes the address. See
[the roadmap](../ROADMAP.md#address-privacy).

## What comes next

Email Delivery is implemented (Phases 13–14). On **2026-10-14** the developer
reordered backend work: messaging with contact privacy first, in parallel with
the frontend Admin Portal and overhaul; then saves, views and the question
thread. This replaces the **2026-09-27** order. Answering an offer ships with
messaging, and since **2026-10-16** promotion waits on it. Its spec is in
Active. Read [the project roadmap](../ROADMAP.md) for the full sequence and
launch goal.
