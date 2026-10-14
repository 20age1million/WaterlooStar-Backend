# Setup

Requires Docker Desktop; running from source also requires Go 1.26+. No other
tooling to install — sqlc and oapi-codegen are pinned as `go.mod` tool dependencies.

## The whole service in Docker

To work on the frontend, or to run the backend without Go installed, one command
builds the API and starts it with PostgreSQL, applying migrations first:

```bash
docker compose up -d --build         # API on localhost:8080
docker compose run --rm api seed     # the development fixtures: 6 listings, 5 requests
```

That is wired for `WaterlooStar-Frontend` on its defaults: `npm run dev` there
serves `localhost:3000`, calls the API at `localhost:8080`, and the API allows
it and builds email links to it. Verification and reset links appear in
`docker compose logs api` unless `RESEND_API_KEY` is set in `.env`. Rebuild with
`--build` after pulling or editing the backend.

The container takes `RESEND_API_KEY` and `EMAIL_FROM` from `.env` when it is
created. After changing either, `docker compose up -d api` recreates it; its log
then says which sender it uses (`docker compose logs api | grep email`).

Seeding deletes every listing before writing the fixtures, so it is a step you
take, not one `up` takes for you.

## The API from source

```bash
cp .env.example .env     # defaults match docker-compose.yml
make up                  # start PostgreSQL 18 alone
make migrate-up          # apply migrations
make seed                # load the development fixtures: 6 listings, 5 requests
make run                 # start the API on :8080
```

`make` is a convenience, not a requirement — it is not installed by default on
Windows. Every target is a one-line Go or Docker command, and the equivalents are:

```bash
docker compose up -d postgres
go run ./cmd/migrate up
go run ./cmd/seed
go run ./cmd/api
```

Start `postgres` by name: a bare `docker compose up` also starts the API
container, which holds port 8080. `docker compose stop api` frees it.

Then:

```bash
curl localhost:8080/healthz
# {"database":"ok","status":"ok"}
```

`make help` lists every target.

## Email

Verification and password-reset emails go through Resend when `RESEND_API_KEY`
is set, from `EMAIL_FROM` (by default `WaterlooStar <no-reply@waterloostar.com>`,
a domain that must be verified for sending in Resend). `internal/email/resend.go`
calls Resend's public API with `net/http`; there is no SDK.

**Without a key, links are printed to the API log.** That is the usual way to
complete a signup in development. With a key, real emails go out, so register
only addresses you can read. Startup logs which sender is in use, and production
without a key starts anyway with a warning, since nothing will be delivered.

A student whose link is lost or expired asks for a new one with
`POST /auth/verify/resend`. It spends the old link, sends only to an unverified,
unsuspended account, and answers 202 for every address.

Clerk's transactional endpoint was the first choice and proved closed to the
account (404 with both keys); [the Email Delivery spec](specs/implemented/email-delivery/index.md)
has the record.
