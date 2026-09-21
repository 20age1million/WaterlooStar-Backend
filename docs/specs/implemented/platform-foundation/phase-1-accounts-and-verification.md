# Phase 1 — Accounts and Verified Students

**Status:** Complete
**Feature:** [platform-foundation](./index.md)
**Objective:** Let a University of Waterloo student register, prove their address, log in, and hold a session — so every later feature can know who is asking and whether they are verified.

---

## Scope

### In scope

- `users` table with UUID keys, plus verification-token and refresh-token tables
- Registration restricted to `uwaterloo.ca` addresses
- Email verification producing the "verified student" state the product gates on
- Login, refresh, logout, and `GET /me`
- Password reset request and confirm
- JWT issued in an httpOnly SameSite cookie, with CSRF protection on mutations
- `RequireAuth` and `RequireVerified` middleware
- An email `Sender` interface with a log-only implementation

### Out of scope

- Choosing or wiring a real email provider — deferred, placeholder only
- Avatars and profile fields beyond what registration collects
- Roles beyond storing the column and putting it in the token claims
- OAuth, SSO, or the university's real identity provider
- Rate limiting — arrives with messaging in a later feature

---

## Dependencies / Prerequisites

- Phase 0 complete: service runs, migrations apply, both generators work

---

## Implementation Steps

- [x] **Preparation**
  - [x] Confirm Phase 0 is marked Complete in `index.md`
  - [x] Update `index.md` — set Phase 1 to In Progress

- [x] **Schema**
  - [x] Add `migrations/000002_users.up.sql` / `.down.sql` creating:
        `users` (`id` uuid default `gen_random_uuid()`, `email` citext-equivalent via a unique
        index on `lower(email)`, `username` unique, `password_hash`, `role` default `user`,
        `verified` default false, `avatar_url` null, `level` default 1, `star_points` default 0,
        `created_at`, `updated_at`)
  - [x] Add `email_verification_tokens` (`token_hash` primary key, `user_id`, `expires_at`,
        `consumed_at` null) and `password_reset_tokens` with the same shape
  - [x] Add `refresh_tokens` (`token_hash` primary key, `user_id`, `expires_at`, `revoked_at` null,
        `user_agent`, `created_at`)
  - [x] Store only hashes of every token, never the raw value
  - [x] Verify: `make migrate-up` then `make migrate-down` both succeed

- [x] **Queries**
  - [x] Add `internal/db/queries/users.sql` — create user, get by id, get by lower(email),
        get by username, mark verified, update password
  - [x] Add `internal/db/queries/tokens.sql` — insert, fetch unconsumed by hash, consume,
        revoke refresh token, revoke all for user
  - [x] Verify: `sqlc generate` produces compiling Go

- [x] **Auth primitives**
  - [x] Add `internal/auth/password.go` — bcrypt hash and compare at an explicit cost
  - [x] Add `internal/auth/token.go` — mint and verify access JWTs carrying `sub`, `role`,
        `verified`, `exp`; short access lifetime with a longer opaque refresh token
  - [x] Add `internal/auth/cookie.go` — shape the session cookie httpOnly, SameSite=Lax,
        Secure off only when the configured environment is development
  - [x] Add `internal/auth/random.go` — cryptographically random token generation and hashing
  - [x] Verify: unit tests cover hash round-trip, token expiry rejection, and tampered-signature rejection

- [x] **Email placeholder**
  - [x] Add `internal/email/sender.go` defining a `Sender` interface
  - [x] Add a `LogSender` implementation writing the message and verification link via `slog`
  - [x] Record in `README.md` that no provider is selected and where to plug one in
  - [x] Verify: registration logs a usable verification link in development

- [x] **Contract**
  - [x] Extend `api/openapi.yaml` with `POST /auth/register`, `POST /auth/verify`,
        `POST /auth/login`, `POST /auth/refresh`, `POST /auth/logout`,
        `POST /auth/password-reset`, `POST /auth/password-reset/confirm`, `GET /me`
  - [x] Add the `User` schema, exposing no password hash and no email of other users
  - [x] Document the cookie-based security scheme and the CSRF header
  - [x] Verify: `oapi-codegen` regenerates and the interface compiles

- [x] **Handlers and middleware**
  - [x] Implement registration — validate the address ends in `uwaterloo.ca`, reject duplicates
        without revealing which field collided, hash the password, issue a verification token
  - [x] Implement verification, login, refresh with rotation, logout revoking the refresh token,
        and password reset
  - [x] Implement `GET /me` returning the authenticated user
  - [x] Add `internal/middleware/auth.go` — `RequireAuth` and `RequireVerified`, putting user id,
        role and verified state on the request context
  - [x] Add `internal/middleware/csrf.go` — double-submit token checked on every unsafe method
  - [x] Verify: each endpoint behaves correctly by hand via curl, including the rejection paths

- [x] **Tests**
  - [x] `internal/httpapi/auth_test.go` — registration accepts a uwaterloo.ca address and rejects
        gmail.com; duplicate registration fails; verification flips `verified`; login sets the
        cookie; `GET /me` is 401 without it; `RequireVerified` is 403 for an unverified user
  - [x] `internal/auth/*_test.go` — password and token unit tests
  - [x] Verify: `go test ./...` passes

- [x] **Final verification**
  - [x] `go build ./...`, `go vet ./...` and `go test ./...` all pass
  - [x] Generators produce no diff when re-run
  - [x] Update `index.md` — set Phase 1 to Complete
  - [x] Move `docs/specs/active/platform-foundation/phase-1-accounts-and-verification.md` →
        `docs/specs/implemented/platform-foundation/phase-1-accounts-and-verification.md`

---

## Completion Criteria

- [x] All checklist items completed and verified
- [x] A `uwaterloo.ca` address can register, verify, log in and read `GET /me`
- [x] A non-UW address is rejected at registration with the standard error envelope
- [x] An unverified account is refused by `RequireVerified`
- [x] No raw token value is ever persisted
- [x] No regressions in related areas
- [x] `index.md` phase status set to Complete
- [x] Phase spec doc moved to `docs/specs/implemented/platform-foundation/`

---

## Implementation Notes

**Key files changed:**

- `migrations/000002_users.*` — `users`, `email_verification_tokens`,
  `password_reset_tokens`, `refresh_tokens`. Unique indexes on `lower(email)` and
  `lower(username)`; a check constraint enforcing the `uwaterloo.ca` rule in the schema,
  not only in the handler that happens to insert today.
- `internal/db/queries/users.sql`, `tokens.sql` — the "GetLive*" queries filter on
  consumed/revoked/expired in SQL, so a caller cannot forget the check.
- `internal/auth/password.go` — bcrypt at cost 12, explicit policy, and `DummyHash` /
  `WasteComparison` so a login for an unknown address costs the same as a known one.
- `internal/auth/token.go` — HS256 access tokens with pinned algorithm, issuer and audience;
  `NewTokenServiceAt` injects a clock for expiry tests.
- `internal/auth/random.go` — 32-byte opaque tokens, stored only as SHA-256.
- `internal/auth/cookie.go` — session/refresh/CSRF cookie shaping.
- `internal/auth/context.go`, `requestinfo.go` — principal, refresh token and user agent on
  the request context.
- `internal/email/sender.go` — `Sender` interface plus the log-only placeholder.
- `internal/middleware/auth.go` — `Authenticate` (populate-only) and `CSRF`.
- `internal/httpapi/auth.go` — the seven auth handlers and `GetMe`.
- `internal/httpapi/responses.go` — response types that set cookies, the strict server's
  intended extension point for anything the contract cannot express.
- `internal/httpapi/mapping.go`, `helpers.go` — row-to-contract mapping and pg error checks.
- `internal/config/config.go` — `JWT_SECRET` (required, min 32 bytes) and `APP_URL`.
- `api/openapi.yaml` — eight new operations, `User` and the request schemas.

**Divergences from plan:**

1. **`RequireAuth`/`RequireVerified` are not gin middleware.** oapi-codegen registers every
   route in one call, so gin middleware cannot be attached per-route without restating the
   contract's security rules in Go. Instead `middleware.Authenticate` populates the principal
   and never rejects, and handlers enforce via `auth.PrincipalFrom(ctx)`. The OpenAPI
   `security` block remains the declaration of what needs a session. `RequireVerified` has no
   caller yet — nothing in Phase 1 requires verification; Phase 4's write endpoints are its
   first users.
2. **`r.ContextWithFallback = true` was required and is not obvious.** The generated strict
   handlers receive the `*gin.Context` as their `context.Context`, and gin does *not* fall
   through to the request's context unless this is set. Without it every value middleware
   attaches is invisible and every authenticated request looks anonymous. Found by three
   failing tests, not by reading.
3. **oapi-codegen's default error handlers were replaced.** They answer `{"msg": "..."}`,
   which is not the documented envelope, and the handler-error default writes the raw Go
   error into the response body. Both now route through `apierror`, so a request-binding
   failure returns the same shape as everything else and internal errors are logged rather
   than returned.
4. **The refresh cookie is scoped to `/auth`, not `/auth/refresh`.** Scoping it to the
   refresh endpoint alone meant the browser never sent it to `/auth/logout`, so logout could
   not revoke the token it exists to revoke. Caught by `TestLogoutRevokesTheSession`.
5. **An extra query, `CountUsersByEmailOrUsername`**, so a duplicate can be answered as a
   clean 409 while the unique indexes remain the real guarantee against a concurrent insert.
6. **`GetUserByUsername` is unused so far.** Written as the checklist specified; the first
   caller will be profile lookup in a later feature.

**Verification run:**

- `go build ./...`, `go vet ./...`, `go test ./...` — all pass. Both generators idempotent.
- Migration `up` → version 2 with four tables; `down-one` → only `schema_migrations` remains;
  `up` again → version 2.
- Registration: `someone@gmail.com` → 400 with a field-level email problem;
  `meil@uwaterloo.ca` → 201, `verified:false`, confirmation link logged;
  duplicate → 409 whose body is byte-identical whether the email or the username collided.
- Domain check rejects `evil-uwaterloo.ca` and `uwaterloo.ca.attacker.com`, accepts
  `edu.uwaterloo.ca`.
- Verification: link from the log flips `verified` to true; replaying it → 400.
- Login before verification succeeds with `verified:false`, as intended.
- Wrong password and unknown address return byte-identical 401 bodies.
- `GET /me` → 200 with a session, 401 without, 401 with a forged cookie.
- CSRF: `POST /auth/logout` with session cookies but no `X-CSRF-Token` → 403.
- Refresh rotates the token; replaying the previous one → 401; after logout → 401.
- Password reset: unknown address → 202 with no email sent; real address → 202, link logged,
  confirm → 204, old password → 401, new password → 200, replayed token → 400.
- Database inspection: token tables hold 32-byte hashes only — searching for the known raw
  reset token returns 0 rows, and for the plaintext password 0 rows; `password_hash` is a
  60-character `$2a$12$` bcrypt string.
- `INSERT` of a `gmail.com` address directly into `users` is refused by the check constraint
  `users_email_is_uwaterloo`.
