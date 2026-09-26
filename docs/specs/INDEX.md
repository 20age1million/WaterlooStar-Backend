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

*None.*

## Implemented

| Feature | Phases | Location |
|---|---|---|
| [Platform Foundation](./implemented/platform-foundation/index.md) | 3 (0–2) | `docs/specs/implemented/platform-foundation/` |
| [Discovery](./implemented/discovery/index.md) | 1 (Phase 3) | `docs/specs/implemented/discovery/` |
| [Listing Write Path](./implemented/listing-write-path/index.md) | 1 (Phase 4) | `docs/specs/implemented/listing-write-path/` |
| [Housing Requests](./implemented/housing-requests/index.md) | 3 (5–7) | `docs/specs/implemented/housing-requests/` |

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

## Decisions carried forward

Recorded in the Platform Foundation `index.md` and still in force:

- UUID primary keys throughout.
- Spec-first OpenAPI: `api/openapi.yaml` is edited before the handler.
- Sessions are a JWT in an httpOnly cookie, with CSRF on unsafe methods.
- pgx + sqlc, no ORM; golang-migrate owns the schema.
- *renter* and *rentee* are internal words only.

## Open, deliberately deferred

- **Whether a listing's street address stays public.** Public by default today;
  the design implies an exact address revealed only after contact. The schema
  keeps `address_line` separate from `neighbourhood` so a private field can be
  added without moving data.
- **Email, image-hosting and map/geocoding providers.** Placeholders behind
  interfaces; see the API README.

## What comes next

Nothing is specified; `active/` is empty. Two things are overdue before more
features, both recorded as gaps rather than specs:

1. **Email delivery.** Verification and reset links only reach the log, so on the
   live site nobody can finish signing up. The provider is chosen (Clerk's
   transactional endpoint); `internal/email` already has the seam.
2. **Rate limiting.** There is none — login accepts unlimited guesses against
   guessable `uwaterloo.ca` addresses, and password reset has no cap.

After those: saves, views and the question thread; then messaging with contact
privacy, which is what "accept an offer" is waiting on. Each starts with a new
specification.
