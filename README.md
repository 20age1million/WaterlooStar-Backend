# WaterlooStar API

Backend for WaterlooStar, a housing marketplace and community forum for University
of Waterloo students. Go + gin, PostgreSQL via pgx, typed queries from sqlc and
schema managed with golang-migrate. The spec-first HTTP contract in
`api/openapi.yaml` generates the gin server interfaces and the TypeScript types
for the separate [WaterlooStar frontend](https://github.com/20age1million/WaterlooStar-Frontend).

## Quick start

Requires Docker Desktop; running from source also requires Go 1.26+. sqlc and
oapi-codegen are pinned as `go.mod` tool dependencies.

```bash
cp .env.example .env
docker compose up -d --build         # API on localhost:8080; PostgreSQL and migrations included
docker compose run --rm api seed     # optional: 6 listings, 5 requests; deletes every listing first
curl localhost:8080/healthz          # {"database":"ok","status":"ok"}
docker compose logs api             # verification and reset links without RESEND_API_KEY
```

## Further reading

- [Setup](docs/setup.md) — Docker, running from source and email configuration.
- [API surface](docs/api.md) — student endpoints, rate limits and listing terms.
- [Admin](docs/admin.md) — admin endpoints, role creation, moderation and `cmd/admin` usage.
- [Development](docs/development.md) — code layout, workflow, generators, checks and technical decisions.
- [Project roadmap](docs/ROADMAP.md) — the goal, feature order and decisions for both repositories.
- [Specifications index](docs/specs/INDEX.md) — implemented features and active specifications.
