# API surface

## Endpoints

The API has forty-two endpoints, fourteen of them admin-only. Everything is
described in [the OpenAPI contract](../api/openapi.yaml); this is the public and
student surface. The admin surface is in [Admin](admin.md#endpoints).

| Method | Path | Auth | Purpose |
|---|---|---|---|
| GET | `/healthz` | — | Service and database health |
| POST | `/auth/register` | — | Create an account (uwaterloo.ca only) |
| POST | `/auth/verify` | — | Redeem a confirmation token |
| POST | `/auth/verify/resend` | — | Send a new confirmation link; the same 202 for any address |
| POST | `/auth/login` | — | Start a session; sets the cookies |
| POST | `/auth/refresh` | refresh cookie | Rotate the session |
| POST | `/auth/logout` | session | End the session |
| POST | `/auth/password-reset` | — | Request a reset email |
| POST | `/auth/password-reset/confirm` | — | Set a new password |
| GET | `/me` | session | The signed-in user |
| GET | `/listings` | — | Browse Housing Available, paginated |
| GET | `/listings/{id}` | — | One listing in full |
| GET | `/listings/availability` | — | Listings open in each month of a year, under the browse filters |
| POST | `/listings` | verified | Post a place |
| PATCH | `/listings/{id}` | owner | Edit it |
| POST | `/listings/{id}/status` | owner | Publish, pause or archive |
| DELETE | `/listings/{id}` | owner | Take it down (archives) |
| GET | `/me/listings` | session | Your own, any status |
| GET | `/requests` | — | Browse Looking for Housing, paginated |
| GET | `/requests/{id}` | — | One request in full |
| POST | `/requests` | verified | Post what you need |
| PATCH | `/requests/{id}` | owner | Edit it |
| POST | `/requests/{id}/status` | owner | Publish, pause or archive |
| DELETE | `/requests/{id}` | owner | Take it down (archives) |
| GET | `/me/requests` | session | Your own, any status |
| POST | `/requests/{id}/offers` | verified | Answer with one of your listings |
| GET | `/requests/{id}/offers` | poster or offerer | The offers on a request |
| DELETE | `/offers/{id}` | offerer | Withdraw yours |

Browsing is public by design: an account is only needed to post, to offer, or
to message a poster once messaging ships.

**Offers are not public.** The student who posted a request sees every live
offer on it; an owner sees only their own. And an offer is only as alive as the
listing behind it — taking a place down removes its offers from view, with no
second step.

## Rate limits

The authentication endpoints and the three write endpoints answer **429** with a
`Retry-After` header when an allowance is spent. The limits are in
`internal/ratelimit/policy.go`, each with a comment saying what it protects.

| Endpoint | Keyed on | Allowance |
|---|---|---|
| `POST /auth/login` | the email address | 5 **failed** attempts per 15 minutes |
| `POST /auth/register` | the email address | 3 per hour, 30 per 10 minutes overall |
| `POST /auth/password-reset` | the email address | 3 per hour, 30 per hour overall |
| `POST /auth/verify/resend` | the email address | 3 per hour, 30 per hour overall |
| `POST /auth/verify`, `POST /auth/password-reset/confirm` | the token | 10 per hour, 60 per 10 minutes overall |
| `POST /listings`, `POST /requests`, `POST /requests/{id}/offers` | the user | 30 per hour |

Four things about it are deliberate:

- **Only a failed login costs anything.** Signing in correctly, however often,
  never meets the limit. You throttle guessing, not using.
- **Nothing is ever locked.** Allowances refill on a clock. A lockout would let
  an attacker deny a student their own account for free.
- **`POST /auth/refresh` is not limited.** The frontend refreshes on navigation,
  so limiting it would sign people out for browsing. The refresh token is already
  single-use and rotated.
- **The key is an identity, not an IP address.** This service has no public URL,
  so every request reaches it from the frontend's address; counting per IP here
  would treat the whole internet as one client. Per-IP limiting belongs at the
  reverse proxy — see [`deploy/RATE-LIMITING.md`](../deploy/RATE-LIMITING.md), which
  has the nginx rule.

The state lives in the process, so a restart forgets the counters and a second
replica would double every limit. Both are recorded rather than hidden.

## Listing terms

An owner can allow **shorter stays**: any stay of at least `min_stay_months`
inside the listed dates, at the same rent. Without it a listing is whole lease
only. Search still returns only listings that cover the whole window asked for;
with a window, `sort=match` puts the ones a student can take for exactly that
window first.

An owner can also give a **bill estimate**: the tenant's monthly share of the
bills the rent does not include. `all_in_cents` is rent plus that estimate, and is
null when the owner has not given one. The API never guesses a figure, so
`all_in_max_cents` and `sort=allInAsc` treat an unstated estimate as unknown, not
as zero.
