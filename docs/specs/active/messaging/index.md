# Messaging with Contact Privacy — API Phases 15–18

**Status:** In progress: Phases 15 and 16 complete
**Branch:** `feature/messaging` (Phase 15); each later phase stacks on the one before (`feature/messaging-privacy` for 16)
**Ticket / Work Item:** N/A
**Owner(s):** WaterlooStar backend
**Date:** 2026-10-15 (first draft); accepted 2026-10-16

> **Accepted 2026-10-16.** Every product question is decided; see Decisions.
> Three were decided by the developer. The developer delegated the rest to
> Claude on the same day, and Claude took the drafted recommendation for each.
> A marker such as *(Q-1, decided 2026-10-16)* points at the row that settles
> that point.

---

## Purpose / Goal

A verified student can message the owner of a listing from inside WaterlooStar,
and an owner can answer a housing request with a conversation. Nobody's contact
details are ever shown. The exact address of a place reaches a student only when
its owner chooses to share it with that student, inside their conversation.
Everyone else sees an approximate location.

---

## Problem Statement / Motivation

- **Nobody can contact anyone.** The listing page has a "Message {owner}"
  button and a pinned mobile bar with a "Message" button. Neither has a handler,
  and the API has no endpoint for them to call. The roadmap's "phone bar" is
  that mobile bar: it holds no phone number. Offers (Phase 7) end the same way:
  the student sees an offer and cannot answer it.
- **Launch waits on this.** On 2026-10-16 the developer decided that students
  are not promoted to until messaging works, in the backend and in a minimal
  frontend messaging phase. There is no interim phone or email contact path.
- **The exact address is public.** `address_line` is served to anyone by
  `GET /listings`, `GET /listings/{id}` and the listing embedded in every
  `RequestOffer`. It can also be found by searching, because `listings.search`
  indexes it (migration 4). On 2026-10-14 the developer settled that a listing
  shows an approximate location publicly and the exact address later, designed
  with messaging *(per `docs/ROADMAP.md`, "Address privacy")*. The public
  address was an implementation gap, not an open decision; Phase 16 closes it.
- **Answering an offer waits on this** *(per `docs/ROADMAP.md`, backend
  sequence)*. Phase 7 left out accept and decline because "accepting promises a
  conversation the product cannot host yet" *(per
  `docs/specs/implemented/housing-requests/index.md`)*.
- **Why now:** it is the next backend feature *(per `docs/ROADMAP.md`,
  2026-10-14)*, and it is now on the launch path. The contract has always
  promised it: the verification email says "Confirm this address to message
  posters", and `GET /listings` says an account is "only needed to save one or
  message the poster".

---

## Decisions — 2026-10-16

### Made by the developer

| Decision | What it means for this spec |
|---|---|
| **Messaging before launch.** Promotion waits on this feature and on a frontend messaging phase, as well as on the Admin Portal and frontend Phase 16. There is no stop-gap phone or email. | The roadmap changes, committed with this spec (`docs/ROADMAP.md`, "Goal and launch"). The address can be hidden before the frontend messaging phase ships, because no promoted audience depends on it yet; see Constraints. |
| **The owner shares the exact address by hand.** The listing owner presses "Share exact address" inside the conversation, whenever they choose. Nothing else reveals it: not a reply, not opening or answering a conversation, not an offer. For an offer, the owner is still the one who shares. | Conversations have no accept, confirm or decline step to carry the reveal. Sharing is its own owner-only action and its own recorded fact (Phase 16). Offers open a conversation, and the owner shares in it like any other (Phase 17). |
| **Only the exact address is revealed.** No email, no phone number. | No new identity data. Emails stay hidden everywhere. The frontend's "Phone and email stay hidden until you both agree to talk" copy needs rewording; see Frontend impact. |

### Decided by Claude under the developer's delegation

On 2026-10-16 the developer delegated the remaining product questions to
Claude. Each was decided by Claude under the developer's delegation,
2026-10-16, taking the option the draft recommended. The delegation counts as
the developer's acceptance.

| # | Question | Decision | First phase it shapes |
|---|---|---|---|
| Q-1 | Does the other side accept a conversation before talking? | **No accept step.** Both sides can write from the first message. The start and send rate limits bound unwanted contact, and closing refuses it. | 15 |
| Q-2 | Can either side close a conversation, and is blocking needed? | **Either side can close.** Nobody can send in a closed conversation; it stays readable. No blocking in this feature. | 15 |
| Q-3 | What do suspension and takedown do to a conversation and to a shared address? | **Read-only, not hidden.** The conversation stays visible to the other side, marked unavailable, and nothing more can be sent. A shared address is withheld (not un-shared) while the listing is taken down or its owner is suspended. Reinstating restores both, because nothing was rewritten. | 15 (send rules), 16 (reveal condition) |
| Q-4 | Is every conversation about a listing? | **Yes.** `listing_id` is required; a request is answered by an offer, which is about a listing. | 15 |
| Q-5 | May an admin read message bodies? | **Never, in this feature.** No admin endpoints or ledger entries for conversations. A report flow, and any admin read tied to it, is a later spec. | 15 |
| Q-6 | Can messages be edited or deleted, and are they kept? | **Append-only and kept.** No edit, no delete, no retention job. | 15 |
| Q-7 | What is the public approximate location? | **`neighbourhood` and a new public `cross_streets`.** `address_line` becomes the private exact address. No data migration. | 16 |
| Q-8 | Can the owner un-share an address? | **No.** The API has no un-share; the frontend confirms before sharing. Moderation can still withhold it (Q-3). | 16 |
| Q-9 | Is sharing per conversation? | **Yes.** One owner, one student, one listing. No listing-wide setting. | 16 |
| Q-10 | Does an offer open a conversation, and does an offer keep an accept or decline? | **An offer opens or joins a conversation**, with the note as a message. Replying is the answer and closing is the refusal; offers get no status of their own. | 17 |
| Q-11 | Which events send email, and can a student opt out? | **Three:** a conversation opened, the address shared, and the first unread reply in a conversation. One per-user opt-out, a per-recipient daily cap and a global cap. Never the message text, never the address. | 18 |

Also decided by Claude under the developer's delegation, 2026-10-16: the
frontend messaging phase goes straight after frontend Phase 16, ahead of the
overhaul's later phases (`docs/ROADMAP.md`, "Frontend sequence").

---

## Proposed Solution / Design

A **conversation** is between two students about one listing: the listing's
owner, and the student asking about it (the *enquirer*). Either can open it: the
enquirer from the listing page, or the owner by offering the listing against the
enquirer's housing request. It has messages, a status, a read marker for each
side, and a record of whether the owner has shared the exact address. It is the
only way the exact address reaches anyone other than the owner and an admin.

### Talking

There is no accept step *(Q-1, decided 2026-10-16)*. Once a conversation exists, either side
can send messages. The rate limits bound how many conversations a student can
open and how fast anyone can send. Either side can **close** the conversation,
which stops new messages from both *(Q-2, decided 2026-10-16)*. Because there is one
conversation per listing and enquirer, closing also stops that enquirer opening
another about that listing. Closing is the way to refuse unwanted contact.

| Opened by | How | Phase |
|---|---|---|
| The enquirer | "Message the owner" on a listing | 15 |
| The owner | Offering the listing against the enquirer's request | 17 |

### Sharing the exact address

The owner can share the exact address in a conversation at any time, from its
first moment on *(decided 2026-10-16)*. Only the owner can do it, and only in
their own conversation. It is:

- **per conversation** *(Q-9, decided 2026-10-16)*: sharing with one student shares with
  that student only, about that listing only;
- **recorded**: `conversations.address_shared_at` is set, and the conversation
  shows when it happened, so both sides see it in the thread;
- **not undoable through the API** *(Q-8, decided 2026-10-16)*: there is no "stop sharing".
  A student who has seen an address cannot be made to forget it, so the product
  does not pretend otherwise. Moderation can still withhold it (below).

The address shown is the listing's current `address_line`, read live. If the
owner corrects it later, the conversation shows the correction.

### What stays private, and where it shows *(Q-7, decided 2026-10-16)*

| Field | Public reads | Owner's own reads | Admin reads | Conversation, after the owner shares |
|---|---|---|---|---|
| `neighbourhood`, `cross_streets` (new) | yes | yes | yes | yes |
| `address_line` (exact address) | **no** | yes | yes | yes, to that enquirer |
| `lat`, `lng` | **null** | yes | yes | yes, to that enquirer |
| `distance_m`, commute minutes | yes | yes | yes | yes |
| Either side's email | no | — | yes | **no** *(decided 2026-10-16)* |

"Public reads" means `GET /listings`, `GET /listings/{id}`, the listing inside
a `RequestOffer`, and the listing summary inside a conversation. "Owner's own
reads" are `GET /me/listings` and the responses to the owner's own writes.

`lat` and `lng` count as part of the exact address: they locate the same door.
They are null on every row today, and are revealed with the address once the
maps feature writes them. There is no phone number anywhere in the schema, and
this feature adds none *(decided 2026-10-16)*.

A shared address is withheld again, without being un-shared, while the listing
is taken down or its owner is suspended *(Q-3, decided 2026-10-16)*. Reinstating restores
it, because nothing was rewritten.

### Key Components

- **`migrations/000011_conversations`** (Phase 15): `conversations` and
  `messages`.
- **`internal/db/queries/conversations.sql`** (Phase 15): starting a
  conversation, the inbox, one conversation for a participant, messages,
  closing, read markers and unread counts.
- **`internal/httpapi/conversations.go`, `messages.go`** (Phase 15): handlers.
  Mapping is in `internal/httpapi/mapping.go`.
- **`migrations/000012_location_privacy`** (Phase 16):
  `listings.cross_streets`, `listings.search` rebuilt without `address_line`,
  and `conversations.address_shared_at`.
- **`toListing`'s `exact` argument** (Phase 16): an explicit flag at every call
  site. It follows the pattern of `toUser(row, includeEmail)` in
  `internal/httpapi/mapping.go`: exposing the address takes a deliberate `true`.
- **`migrations/000013_offer_conversations`** (Phase 17):
  `request_offers.conversation_id`, and a backfill of conversations for live
  offers.
- **`internal/email/sender.go`** (Phase 18): notification message builders sent
  through the existing `email.Sender` *(per
  `docs/specs/implemented/email-delivery/index.md`)*.
- **`internal/ratelimit/policy.go`, `internal/httpapi/ratelimit_mw.go`**: new
  rules, and allowlist entries for the new write routes.

### Data / Control Flow

- **Starting.** A verified student opens a published listing that is not their
  own and sends a first message: `POST /listings/{id}/conversations`. The
  listing is loaded through `GetPublishedListing`, so a draft, paused,
  taken-down or suspended owner's listing answers 404, as it does everywhere.
  The conversation and its first message are written in one transaction,
  through the `db.TxRunner` the admin writes already use. A second attempt for
  the same listing answers 409 and names the existing conversation, open or
  closed.
- **The inbox.** `GET /me/conversations` lists both sides: places the student
  asked about, and conversations about the student's own listings. The newest
  activity comes first. Each row has the listing summary, the other person
  (`ListingOwner` shape, never an email), the last message, the unread count and
  what the viewer may do next. `GET /me/conversations/unread` returns the
  counts the header and the "Desk" home need.
- **Reading.** `GET /conversations/{id}` and `GET /conversations/{id}/messages`.
  Anyone who is not a participant gets 404, never 403.
  `POST /conversations/{id}/read` moves the viewer's read marker to now.
- **Sending.** `POST /conversations/{id}/messages`, by either participant while
  the conversation is open, both accounts are active and the listing is not
  taken down *(Q-3, decided 2026-10-16)*.
- **Closing.** `POST /conversations/{id}/close`, by either participant
  *(Q-2, decided 2026-10-16)*. It stays readable; nothing more can be sent.
- **Sharing (Phase 16).** `POST /conversations/{id}/share-address`, by the
  owner only. It sets `address_shared_at` once; sharing again changes nothing.
  `Conversation.place` carries `address_line`, `lat` and `lng`. It is non-null
  only when `address_shared_at` is set, the listing is not taken down and its
  owner is not suspended. The condition is in the SQL, as the public read
  filters are.
- **Offers (Phase 17).** Making an offer opens a conversation from the owner to
  the request's poster in the same transaction, or joins the one they already
  have about that listing. A non-empty note becomes a message. Withdrawing an
  offer leaves the conversation alone: nothing was revealed by the offer, so
  there is nothing to take back. The poster answers an offer by replying in its
  conversation *(Q-10, decided 2026-10-16)*.
- **Email (Phase 18)** *(Q-11, decided 2026-10-16)*. A conversation being opened, the
  address being shared, and the first unread reply in a conversation each send
  one email to the other side, within caps. The email names the person and the
  listing and links to the conversation. It never contains the message text or
  the address.

### Computed, not stored

Unread counts and "last activity" are computed from `messages` and the read
markers by the read queries. They are not stored as counters, which is the
lesson of the dropped `housing_requests.offers` column *(per `CLAUDE.md`,
"Offer counts are computed, never stored")*. The index on
`messages (conversation_id, created_at DESC)` is what keeps this cheap.

---

## Frontend impact

This repository does not change the frontend. These are the follow-ups the
frontend messaging phase (and one small fix before it) will need, recorded here
so the contract can be judged against them. The frontend numbers its own phases;
its next ones are 13–15 (Admin Portal) and 16 (overhaul), so the messaging phase
is "a frontend messaging phase" here.

- **Before backend Phase 16 merges (a small UI fix, any time):** stop reading
  `addressLine` from public reads. Today it is read in
  `src/components/HousingCard.tsx` (the photo placeholder label),
  `src/app/housing/Compare.tsx` and the location line in `src/lib/format.ts`.
  `/housing/mine` and the edit form read `/me/listings`, which keeps the field.
- **Copy that becomes untrue:**
  - `src/app/housing/[id]/ListingDetailClient.tsx`, under the Message button:
    "Phone and email stay hidden until you both agree to talk." There is no
    phone, nobody agrees to anything, and the address is the owner's to share.
    Suggested: "Your email is never shown. {owner} can share the exact address
    in your conversation. Keep deposits and viewings off other platforms."
  - The same file's map note, "Sketch — exact address shared once you message":
    messaging alone shares nothing. Suggested: "Sketch — the owner can share the
    exact address in your conversation."
  - `src/fixtures/housing.ts`, "Phone numbers and emails are hidden until both
    sides agree to talk". Frontend Phase 16 removes the fixtures; if the copy
    survives, it needs the same rewording.
  - `src/app/housing/ListingForm.tsx`: the `address_line` field's "Shown on the
    listing" hint and "Hazel St" placeholder. It becomes the private exact
    address, house number included, "shared only with students you choose".
    A cross-streets field is added *(Q-7, decided 2026-10-16)*.
- **What the frontend messaging phase wires up:** the "Message {owner}" button
  and the mobile bar's "Message" button (start a conversation); the inbox and
  the thread; the owner's "Share exact address" action, with a confirmation,
  since it cannot be undone *(Q-8, decided 2026-10-16)*; the shared address in the thread;
  closing; replying to an offer from its conversation; the header's Messages
  link that frontend Phase 16 removes; and the Desk home's messages skeleton.
- Regenerated types: `Listing.address_line` becomes optional;
  `cross_streets`, the `Conversation` family and `RequestOffer.conversation`
  appear.

---

## Layers / Areas Affected

| Layer / Area | Change |
|---|---|
| Database schema | `conversations`, `messages` (15); `listings.cross_streets`, `search` rebuilt, `conversations.address_shared_at` (16); `request_offers.conversation_id` (17); `users.email_notifications` (18) |
| Migrations | 000011–000014 |
| DB access layer | `conversations.sql` (new); `listings.sql` and `offers.sql` changed in 16 and 17 |
| Service layer | Participant checks, the send and close rules, the owner-only share, the reveal condition |
| API handlers | 8 new operations in Phase 15, 1 in Phase 16; changed listing and offer handlers in 16 and 17 |
| DTOs / contracts | `api/openapi.yaml`: a `messages` tag, `Conversation`, `Message`, pages, `ConversationPlace`; `Listing.address_line` becomes owner-only; `cross_streets`; `RequestOffer.conversation`; `ListingOwner`'s "both sides agree to talk" description reworded |
| Auth | Verified students only, to start, send or share. Participants only, to read. The owner only, to share. 404 for everyone else |
| Rate limiting | Allowlist entries for starting a conversation and for sending a message (15) and for sharing the address (16, added during implementation); notification caps in 18 |
| Email | Notification builders in Phase 18 |
| Tests | Query tests for every new query (`TestEveryQueryIsExercised`); handler tests; regression tests that no public read carries `address_line` |
| Configuration | None. `APP_URL` already builds email links |
| Deployment / Infra | None |
| Operational | `messages` grows without bound. No retention policy *(Q-6, decided 2026-10-16)* |

---

## Phase Tracker

| Phase | Title | Status | Location |
|---|---|---|---|
| [15 — Conversations and messages](../../implemented/messaging/phase-15-conversations.md) | Start a conversation about a listing, the inbox, messages, closing, unread counts | Complete | `docs/specs/implemented/messaging/` |
| [16 — Contact privacy and address sharing](../../implemented/messaging/phase-16-contact-privacy.md) | Exact address and coordinates off every public read and out of search; the owner shares the address in a conversation | Complete | `docs/specs/implemented/messaging/` |
| [17 — Offers open conversations](./phase-17-offer-conversations.md) | An offer opens or joins a conversation with the request's poster, who answers it there | Ready | `docs/specs/active/messaging/` |
| [18 — Message notifications](./phase-18-message-notifications.md) | Email when a conversation is opened, the address is shared, or a first reply waits unread | Ready | `docs/specs/active/messaging/` |

Phase 15 is additive and changes nothing the frontend uses today. Phase 16 is
the one breaking contract change; see Constraints. The frontend messaging phase
can start against Phases 15–16; Phase 17 adds replying to offers. Promotion
waits on all four *(decided 2026-10-16)*. Phase 18 is the final phase.

---

## Trade-offs / Alternatives Considered

- **Sharing is a separate act, not a side effect** *(decided 2026-10-16)*. The
  first draft tied the reveal to the other side accepting the conversation.
  That gave out the owner's address on the strength of the enquirer's consent
  (in an offer) or a single click (on a listing). The address is the owner's,
  so the owner's own deliberate action is what reveals it, and nothing else can
  reveal it by accident.
- **No accept step** *(Q-1, decided 2026-10-16)*. Accepting existed to gate the address.
  With the address gated by the owner's share, accepting would gate only the
  right to reply, which a student who wants to refuse can do by closing. One
  status fewer, two endpoints fewer, and no stalled "pending" threads.
- **Address hiding ships with sharing, not before it.** The privacy change could
  go first, as its own phase. But then the API would have no way to give anyone
  the exact address. Putting hiding and sharing in one phase (16), on top of
  conversations (15), means there is never a release where the address cannot
  be reached at all. The roadmap asks for this: "the privacy change ships with
  messaging".
- **One conversation per listing and enquirer**, not one per pair of people.
  The listing is what the conversation is about, it is what sharing shares, and
  it is what a takedown acts on. Two students talking about two places have two
  threads. Conversations without a listing are left out *(Q-4, decided 2026-10-16)*.
- **A share is a timestamp on the conversation**, not a message row. Messages
  have a sender and a body the sender wrote; a share has neither. The client
  places the "{owner} shared the exact address" event in the thread by its time.
  If sharing is ever made revocable, the same column is cleared; no reshaping.
- **An offer opens a conversation; it does not reveal anything** *(Q-10, decided 2026-10-16)*. The poster answers by replying. Offers get no accept or decline status
  of their own, so offers and messages stay one mechanism, and the roadmap's
  "accept an offer" ships as "answer an offer in its conversation".
- **Messages are append-only** *(Q-6, decided 2026-10-16)*: no editing or deleting. This is
  the simplest option, and it keeps the evidence if someone reports harassment.
- **Offset pagination for messages** (`page`, `per_page`, newest first),
  matching every other list in the API *(per
  `docs/specs/implemented/housing-requests/index.md`, Assumptions)*. A message
  arriving while someone reads older pages shifts the offsets by one. The
  client removes duplicates by id. A keyset cursor would avoid this, at the cost
  of a second pagination style in the contract.
- **No admin endpoints for messages** *(Q-5, decided 2026-10-16)*. An admin acts on accounts
  and posts with the tools they already have. Reading message bodies is a
  separate decision.
- **No real-time delivery.** No WebSockets and no server-sent events. The
  frontend polls the unread counts, and Phase 18's emails reach students who
  are not on the site. With one API instance and a private network, a push
  channel adds infrastructure that nothing needs yet.
- **No new identity data** *(decided 2026-10-16)*. Nothing here adds a phone
  number or shows anyone's email. The conversation is the contact channel.

---

## Assumptions

- Message volume is small: students, not traffic. Computing unread counts with
  an indexed subquery is fast enough, and `messages` needs no partitioning.
- The frontend polls `GET /me/conversations/unread` at a modest interval.
  Polling is a read, and reads are not rate limited.
- Existing `address_line` values are street names without house numbers. The
  frontend form's placeholder is "Hazel St" and its hint says "Shown on the
  listing", and the seed uses street names. Making the field private exposes
  nothing new. Owners will need prompting to give the exact address before they
  share it.
- `lat`/`lng` are null on every row today. The seed writes nil, and
  `ListingInput` does not accept them. Nulling them on public reads changes no
  live response.
- The site is deployed but not yet promoted to students *(per
  `docs/ROADMAP.md`, "Goal and launch")*, so hiding the street name before the
  frontend messaging phase ships costs no one a contact path they rely on.
- One API instance. Rate-limit buckets are per process *(per `CLAUDE.md`)*.
- Phase 18: Resend's free tier also covers notification email at this volume.
  Its daily cap needs checking against expected traffic. The Email Delivery spec
  sized it for verification and reset mail only.

---

## Constraints

- **The contract comes first** *(per `CLAUDE.md`)*. Each phase edits
  `api/openapi.yaml` before its handlers, then regenerates.
- **Schema changes are migration pairs** *(per `CLAUDE.md`)*. Every query gets a
  test against PostgreSQL *(per `TestEveryQueryIsExercised`)*.
- **The one error envelope** *(per `internal/apierror`)*, including the 409s and
  429s added here.
- **Someone else's conversation answers 404, never 403**, as listings, requests
  and offers do.
- **Only verified students start conversations, send messages or share an
  address.** This is settled, not open: the verification email says "Confirm
  this address to message posters" (`internal/email/sender.go`), and
  `GET /listings` says "an account is only needed to save one or message the
  poster".
- **Only the listing's owner can share its exact address, by an explicit action
  in one conversation** *(decided 2026-10-16)*. No other endpoint, status
  change or side effect may set `address_shared_at` or fill `place`. A test
  enforces this.
- **Only the exact address is revealed; no email or phone, ever** *(decided
  2026-10-16)*.
- **A conversation can only be started on a listing the public can see.** The
  published-only, not-removed and owner-not-suspended filters stay in the SQL
  *(per `CLAUDE.md`)*.
- **No public read carries `address_line` or exact coordinates** after Phase 16,
  and none can find a listing by its address. A test enforces this.
- **Rate limits key on identity, as constants in `policy.go`, through the
  allowlist** *(per `docs/specs/implemented/rate-limiting/index.md`)*. Each new
  write route goes on the allowlist, or it is unlimited.
- **`renter` and `rentee` appear nowhere.** The two sides are the *owner* and
  the *enquirer*.
- **No new configuration and no new service.** Email links use `APP_URL`.
- **Phase 16 needs one frontend fix first, not the frontend messaging phase.**
  The frontend reads `addressLine` on public surfaces (see Frontend impact).
  Drone deploys each repository from `main` on its own, so the frontend has to
  stop reading `address_line` from public reads before Phase 16 merges, or cards
  show "undefined". Phase 16 need not wait for the frontend messaging phase:
  promotion waits on that phase *(decided 2026-10-16)*, so no promoted audience
  loses the street name in between.
- **Every pull request leaves the documentation current**: `CLAUDE.md`,
  `README.md`, `docs/api.md`, `docs/ROADMAP.md`, `docs/specs/INDEX.md` *(per
  `feature-implement`, Step 4, item 4)*. The spec commit itself carries the
  2026-10-16 roadmap changes.

---

## Success Criteria / Definition of Done

- A verified student can start a conversation from a published listing that is
  not theirs. A second attempt answers 409 and names the existing conversation.
  An unverified student gets 403, and an unpublished listing gets 404.
- The owner sees it in their inbox as unread, and either side can send messages
  from then on. Each side's unread count rises with the other's messages and
  clears when they read.
- Either side can close the conversation, after which neither can send
  *(Q-2, decided 2026-10-16)*.
- A third account gets 404 for the conversation and its messages.
- No public read (`GET /listings`, `GET /listings/{id}`, the offer list, the
  conversation's listing summary) carries `address_line` or non-null
  coordinates. Searching an exact street address matches nothing that the
  street name alone would not.
- The owner still sees their exact address on `GET /me/listings`. An admin still
  sees it on the admin listing reads.
- The exact address appears in a conversation only after its owner has shared
  it there, and in no other conversation. Sending, replying, offering and
  closing never reveal it. It is withheld while the listing is taken down or
  its owner suspended, and returns on reinstatement *(Q-3, decided 2026-10-16)*.
- An offer opens or joins a conversation with the request's poster, who can
  reply in it. The address appears there only if the owner shares it.
- Starting conversations and sending messages answer 429 past their allowances,
  with `Retry-After` and the standard envelope.
- *(Phase 18)* The notification emails go to the right side, carry neither
  message text nor address, and stay within their caps. A failed send changes
  no response.
- `go build ./...`, `go vet ./...` and `go test ./...` pass, with and without
  `PG_TEST_DSN`. Generators are idempotent.
- All phase checklists completed and verified.
- No regressions in related areas.

---

## References (internal only)

- Decisions: `docs/ROADMAP.md` (2026-10-14: sequence, address privacy;
  2026-10-16: messaging before launch, the owner shares the address, the
  address only)
- The address column's history: `docs/specs/implemented/platform-foundation/index.md`
  (Constraints: public by default, schema kept separate from `neighbourhood`);
  `migrations/000003_listings.up.sql`
- Offers and the deferred accept: `docs/specs/implemented/housing-requests/index.md`,
  `phase-7-offers.md`; `migrations/000007_request_offers.up.sql`
- Suspension, takedown and the ledger: `docs/specs/implemented/admin-moderation/index.md`
- Limits: `docs/specs/implemented/rate-limiting/index.md`, `internal/ratelimit/policy.go`
- Email: `docs/specs/implemented/email-delivery/index.md`, `internal/email/sender.go`
- The consumer: `../WaterlooStar-Frontend/src/app/housing/[id]/ListingDetailClient.tsx`
  (the inert Message buttons, the pinned mobile bar, the contact copy)
