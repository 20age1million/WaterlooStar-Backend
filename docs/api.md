# API surface

## Endpoints

The API has fifty-one endpoints, fourteen of them admin-only. Everything is
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
| POST | `/requests/{id}/offers` | verified | Answer with one of your listings; opens or joins a conversation with the poster |
| GET | `/requests/{id}/offers` | poster or offerer | The offers on a request |
| DELETE | `/offers/{id}` | offerer | Withdraw yours; its conversation stays |
| POST | `/listings/{id}/conversations` | verified | Message a listing's owner; opens a conversation |
| GET | `/me/conversations` | session | Your conversations, both sides, newest activity first |
| GET | `/me/conversations/unread` | session | Unread conversation and message counts |
| GET | `/conversations/{id}` | participant | One conversation |
| GET | `/conversations/{id}/messages` | participant | Its messages, newest first, paginated |
| POST | `/conversations/{id}/messages` | verified participant | Send a message |
| POST | `/conversations/{id}/read` | participant | Mark it read |
| POST | `/conversations/{id}/close` | participant | Close it; nobody can send afterwards |
| POST | `/conversations/{id}/share-address` | the listing's owner, verified | Share the exact address with the enquirer; cannot be undone |

Browsing is public by design: an account is only needed to post, to offer, or
to message a poster.

**The exact address is never public.** A listing's public location is its
`neighbourhood` and `cross_streets` (the nearest intersection). `address_line`,
the exact address with the house number, and the coordinates `lat`/`lng` are
on the owner's own reads (`GET /me/listings` and the responses to their own
writes) and on the admin surface, and nowhere else: `GET /listings`,
`GET /listings/{id}` and the listing inside an offer have no `address_line`
key and null coordinates. Search (`q`) does not index the address, so it
cannot be confirmed by probing. The one way the exact address reaches another
student is the owner sharing it in their conversation (below). No response
ever carries anyone's email, except your own on `/me`, or a phone number.

**Offers are not public.** The student who posted a request sees every live
offer on it; an owner sees only their own. And an offer is only as alive as the
listing behind it — taking a place down removes its offers from view, with no
second step.

**An offer opens a conversation, and the poster answers it there.** Offering a
listing against a request opens a conversation between you and the request's
poster about that listing, or joins the one you already have, in the same
transaction; a non-empty note becomes your message in it. Each offer carries
`conversation` (`id`, `status`, `poster_replied`), so a client can link to the
thread and show which offers are still waiting on the student. Offers have no
accept or decline: the poster replies, or refuses by closing the conversation.
If the poster closed the conversation about that listing, offering it again
answers 409 with `details.conversation_id`; one you closed yourself is
reopened. Several offers of one listing to one student share a conversation.
Withdrawing an offer leaves its conversation as it is. **An offer reveals
nothing**: the listing in it is the public shape, and the exact address
reaches the poster only if you share it in the conversation. You cannot offer
against your own request (400).

## Messages

A conversation is between two students about one listing: its **owner** and the
student asking about it, the **enquirer**. Only those two can see it; anyone
else gets 404, never 403.

- **No accept step.** Either side can write from the first message on.
- **Either side can close it.** After that nobody can send, and the enquirer
  cannot open another about the same listing: there is one conversation per
  listing and enquirer, and a second start answers 409 naming the first in
  `details.conversation_id`.
- **Either side can open it.** The enquirer from the listing page; the owner
  by offering the listing against the enquirer's request (see offers, above).
  A conversation the owner opened, with no note, has no messages yet:
  `last_message` is null and `created_at` is its last activity.
- **Opening a conversation reveals nothing.** No email, no phone number and no
  exact address. Neither do sending, replying, reading, closing or an offer.
- **The owner shares the exact address by hand.**
  `POST /conversations/{id}/share-address` sets `address_shared_at` once and
  fills the conversation's `place` (`address_line`, `lat`, `lng`) for both
  participants of that one conversation. It is the owner's alone (the enquirer
  gets 403), it covers that conversation only, and it **cannot be undone**:
  there is no un-share, so a client confirms first. Sharing again answers 200
  unchanged. It is refused with 409 while the conversation is closed, the
  enquirer is suspended, the listing is not available, or the listing has no
  address. `can_share_address` says whether the button should show.
- **Moderation withholds a shared address without un-sharing it.** While the
  listing is taken down or its owner is suspended, `place` is null and
  `address_shared_at` stays set; reinstating brings `place` back. The address
  is read live, so an owner's correction shows in the conversation.
- **Moderation makes it read-only, never hidden.** While the listing is taken
  down or the other side is suspended, the conversation stays readable and
  nothing can be sent. Reinstating restores it. A listing its owner paused or
  archived carries on: the two still need to talk about the lease.
- **Messages are append-only.** They cannot be edited or deleted, and admins
  have no surface for reading them.
- **Unread counts are computed**, from the other side's messages and your own
  read marker, never stored. Poll `GET /me/conversations/unread`; reads are not
  rate limited.

## Rate limits

The authentication endpoints and the six write endpoints answer **429** with a
`Retry-After` header when an allowance is spent. The limits are in
`internal/ratelimit/policy.go`, each with a comment saying what it protects.

| Endpoint | Keyed on | Allowance |
|---|---|---|
| `POST /auth/login` | the email address | 5 **failed** attempts per 15 minutes |
| `POST /auth/register` | the email address | 3 per hour, 30 per 10 minutes overall |
| `POST /auth/password-reset` | the email address | 3 per hour, 30 per hour overall |
| `POST /auth/verify/resend` | the email address | 3 per hour, 30 per hour overall |
| `POST /auth/verify`, `POST /auth/password-reset/confirm` | the token | 10 per hour, 60 per 10 minutes overall |
| `POST /listings`, `POST /requests`, `POST /requests/{id}/offers` | the user | 30 per hour (an offer's note is a message, and is bounded by this, not by the message limit) |
| `POST /listings/{id}/conversations` | the user | 10 per 2 hours, including attempts that find one already exists |
| `POST /conversations/{id}/messages` | the user | 60 per 10 minutes |
| `POST /conversations/{id}/share-address` | the user | 20 per hour, including shares that change nothing |

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
