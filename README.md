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
cp -n .env.example .env
RESEND_API_KEY= EMAIL_FROM= docker compose --env-file .env.example up -d --build
# API on localhost:8080; PostgreSQL and pending migrations included. No reseed.
curl localhost:8080/healthz          # {"database":"ok","status":"ok"}
docker compose logs api             # verification and reset links, and notification emails, without RESEND_API_KEY
```

The forum now supports public post browsing and details, with authenticated
title/body creation and author-only editing/deletion. Migration `000016_forum_posts`
is additive; deleted forum posts are hidden using `deleted_at`. Comments,
reactions, categories and moderation tools are deferred. See [the forum guide](docs/forum.md).

## Further reading

- [Setup](docs/setup.md) — Docker, running from source and email configuration.
- [API surface](docs/api.md) — student endpoints, messages, notification emails, rate limits and listing terms.
- [Admin](docs/admin.md) — admin endpoints, role creation, moderation and `cmd/admin` usage.
- [Development](docs/development.md) — code layout, workflow, generators, checks and technical decisions.
- [Project roadmap](docs/ROADMAP.md) — the goal, feature order and decisions for both repositories.
- [Specifications index](docs/specs/INDEX.md) — implemented features and active specifications.
