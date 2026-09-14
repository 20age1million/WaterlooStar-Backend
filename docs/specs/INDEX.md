# Feature Specifications — WaterlooStar API

Every feature is specified before it is built, and the specification is committed
as the first meaningful commit on its branch. `active/` holds work in progress;
`implemented/` holds what has shipped.

Each feature is a folder: `index.md` for the whole feature, `phase-N-*.md` for
each independently committable phase.

## Active

*None.*

## Implemented

| Feature | Phases | Location |
|---|---|---|
| [Platform Foundation](./implemented/platform-foundation/index.md) | 3 | `docs/specs/implemented/platform-foundation/` |

### Platform Foundation

Empty repository to a service that authenticates verified University of Waterloo
students and serves housing listings, under a spec-first OpenAPI contract.

| Phase | Title | Delivered |
|---|---|---|
| 0 | [Contract and scaffolding](./implemented/platform-foundation/phase-0-contract-and-scaffolding.md) | gin service, migration harness, sqlc + oapi-codegen chain, `/healthz` |
| 1 | [Accounts and verified students](./implemented/platform-foundation/phase-1-accounts-and-verification.md) | Registration gated on `uwaterloo.ca`, email verification, login, sessions, password reset |
| 2 | [Listings read path](./implemented/platform-foundation/phase-2-listings-read-path.md) | Structured listing schema, seed data, `GET /listings` and `GET /listings/{id}` |

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

The product roadmap continues with discovery — server-side search, filtering,
sorting and pagination — which is what makes the housing hub's filter rail real.
That is a new feature and starts with a new specification.
