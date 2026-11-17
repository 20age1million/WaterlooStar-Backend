# Initial forum

Design A was selected on 2026-11-17. This API phase supports public forum reads
and authenticated title/body CRUD only. The frontend consumes the OpenAPI
contract; comments, reactions, categories and moderation tools remain deferred.

## Contract

| Method | Path | Behavior |
|---|---|---|
| GET | `/forum/posts?q=&page=1&per_page=20` | Public newest-first list, title/body full-text search and page metadata |
| GET | `/forum/posts/{id}` | Public detail; missing/deleted/suspended-author posts are 404 |
| POST | `/forum/posts` | Authenticated active account; title/body only; author comes from the session |
| PATCH | `/forum/posts/{id}` | Author only; edit supplied title/body fields |
| DELETE | `/forum/posts/{id}` | Author only; soft-delete and return 204 |

Title and body are trimmed and checked as Unicode characters: title 1–200,
body 1–8,000. Invalid, null or empty supplied values return the existing field
error envelope. Anonymous writes return 401; non-owner writes return 404.
All writes carrying a session require the existing matching CSRF header.
Creation shares the existing per-user posting allowance and returns429 with
Retry-After when spent; owner edits/deletes and reads are not charged.
Unverified but authenticated active accounts may use the initial forum.
Responses expose the existing public author shape, never email or password data.

## Migration and data

`000016_forum_posts.up.sql` creates a separate forum table, search/index support
and the existing timestamp-trigger pattern. Its down pair removes only the new
forum schema. No migration 1–15 is rewritten. No existing account, housing,
conversation or message rows are backfilled, reset or reseeded. Delete sets
`deleted_at`; the content is no longer publicly readable or editable.

## Run locally

From `WaterlooStar-Backend`:

```bash
RESEND_API_KEY= EMAIL_FROM= docker compose --env-file .env.example up -d --build
curl http://localhost:8080/healthz
```

Compose builds the current branch, starts PostgreSQL, applies pending migrations
and waits for them before starting the API. The existing named data volume is
reused. The blank email overrides keep local tests from sending real emails.
For an explicit, repeatable migration check after startup:

```bash
RESEND_API_KEY= EMAIL_FROM= docker compose --env-file .env.example run --rm api migrate up
```

Do not run `seed` to enable the forum: it is not needed and rewrites housing
fixtures. Do not run `down -v`, `migrate down` or drop the development database.

For source development instead of the API container, start only PostgreSQL,
apply migrations, then run Go with a keyless environment. Stop the API container
first if using its port 8080; or use a separate port and matching frontend API_URL.

## Regenerate and check

```bash
go tool oapi-codegen -config api/oapi-codegen.yaml api/openapi.yaml
go tool sqlc generate
go vet ./...
PG_TEST_DSN='postgres://waterloostar:waterloostar@localhost:5433/waterloostar?sslmode=disable' go test ./...
go build ./cmd/api ./cmd/migrate
```

Real query tests create their own databases on the PostgreSQL server and clean
up those databases only. The staged migration test starts at version 15 with
existing data and applies version 16. Handler tests cover authorship, ownership,
authentication, CSRF, validation and deletion.

See the completed phase record for exact verification results and local-only
integration testing. No merge, push or deployment is part of this phase.
