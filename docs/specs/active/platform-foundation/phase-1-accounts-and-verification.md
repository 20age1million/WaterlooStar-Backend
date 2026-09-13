# Phase 1 — Accounts and Verified Students

**Status:** Ready
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

- [ ] **Preparation**
  - [ ] Confirm Phase 0 is marked Complete in `index.md`
  - [ ] Update `index.md` — set Phase 1 to In Progress

- [ ] **Schema**
  - [ ] Add `migrations/000002_users.up.sql` / `.down.sql` creating:
        `users` (`id` uuid default `gen_random_uuid()`, `email` citext-equivalent via a unique
        index on `lower(email)`, `username` unique, `password_hash`, `role` default `user`,
        `verified` default false, `avatar_url` null, `level` default 1, `star_points` default 0,
        `created_at`, `updated_at`)
  - [ ] Add `email_verification_tokens` (`token_hash` primary key, `user_id`, `expires_at`,
        `consumed_at` null) and `password_reset_tokens` with the same shape
  - [ ] Add `refresh_tokens` (`token_hash` primary key, `user_id`, `expires_at`, `revoked_at` null,
        `user_agent`, `created_at`)
  - [ ] Store only hashes of every token, never the raw value
  - [ ] Verify: `make migrate-up` then `make migrate-down` both succeed

- [ ] **Queries**
  - [ ] Add `internal/db/queries/users.sql` — create user, get by id, get by lower(email),
        get by username, mark verified, update password
  - [ ] Add `internal/db/queries/tokens.sql` — insert, fetch unconsumed by hash, consume,
        revoke refresh token, revoke all for user
  - [ ] Verify: `sqlc generate` produces compiling Go

- [ ] **Auth primitives**
  - [ ] Add `internal/auth/password.go` — bcrypt hash and compare at an explicit cost
  - [ ] Add `internal/auth/token.go` — mint and verify access JWTs carrying `sub`, `role`,
        `verified`, `exp`; short access lifetime with a longer opaque refresh token
  - [ ] Add `internal/auth/cookie.go` — shape the session cookie httpOnly, SameSite=Lax,
        Secure off only when the configured environment is development
  - [ ] Add `internal/auth/random.go` — cryptographically random token generation and hashing
  - [ ] Verify: unit tests cover hash round-trip, token expiry rejection, and tampered-signature rejection

- [ ] **Email placeholder**
  - [ ] Add `internal/email/sender.go` defining a `Sender` interface
  - [ ] Add a `LogSender` implementation writing the message and verification link via `slog`
  - [ ] Record in `README.md` that no provider is selected and where to plug one in
  - [ ] Verify: registration logs a usable verification link in development

- [ ] **Contract**
  - [ ] Extend `api/openapi.yaml` with `POST /auth/register`, `POST /auth/verify`,
        `POST /auth/login`, `POST /auth/refresh`, `POST /auth/logout`,
        `POST /auth/password-reset`, `POST /auth/password-reset/confirm`, `GET /me`
  - [ ] Add the `User` schema, exposing no password hash and no email of other users
  - [ ] Document the cookie-based security scheme and the CSRF header
  - [ ] Verify: `oapi-codegen` regenerates and the interface compiles

- [ ] **Handlers and middleware**
  - [ ] Implement registration — validate the address ends in `uwaterloo.ca`, reject duplicates
        without revealing which field collided, hash the password, issue a verification token
  - [ ] Implement verification, login, refresh with rotation, logout revoking the refresh token,
        and password reset
  - [ ] Implement `GET /me` returning the authenticated user
  - [ ] Add `internal/middleware/auth.go` — `RequireAuth` and `RequireVerified`, putting user id,
        role and verified state on the request context
  - [ ] Add `internal/middleware/csrf.go` — double-submit token checked on every unsafe method
  - [ ] Verify: each endpoint behaves correctly by hand via curl, including the rejection paths

- [ ] **Tests**
  - [ ] `internal/httpapi/auth_test.go` — registration accepts a uwaterloo.ca address and rejects
        gmail.com; duplicate registration fails; verification flips `verified`; login sets the
        cookie; `GET /me` is 401 without it; `RequireVerified` is 403 for an unverified user
  - [ ] `internal/auth/*_test.go` — password and token unit tests
  - [ ] Verify: `go test ./...` passes

- [ ] **Final verification**
  - [ ] `go build ./...`, `go vet ./...` and `go test ./...` all pass
  - [ ] Generators produce no diff when re-run
  - [ ] Update `index.md` — set Phase 1 to Complete
  - [ ] Move `docs/specs/active/platform-foundation/phase-1-accounts-and-verification.md` →
        `docs/specs/implemented/platform-foundation/phase-1-accounts-and-verification.md`

---

## Completion Criteria

- [ ] All checklist items completed and verified
- [ ] A `uwaterloo.ca` address can register, verify, log in and read `GET /me`
- [ ] A non-UW address is rejected at registration with the standard error envelope
- [ ] An unverified account is refused by `RequireVerified`
- [ ] No raw token value is ever persisted
- [ ] No regressions in related areas
- [ ] `index.md` phase status set to Complete
- [ ] Phase spec doc moved to `docs/specs/implemented/platform-foundation/`

---

## Implementation Notes

> *Added after completion. Fill in before committing the phase.*
>
> **Key files changed:**
> - `path/to/file`: what changed
>
> **Divergences from plan:**
> - <none / description>
>
> **Verification run:**
> - <command or action and its result>
