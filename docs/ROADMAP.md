# WaterlooStar project roadmap

## Status

**Backend:** Phases 0–14 are shipped: accounts, housing discovery and posting,
requests and offers, rate limits, admin moderation, listing terms and email
delivery. **Messaging with contact privacy is shipped** (Phases 15–18,
merged in #17 and #21, deployed 2026-11-07). **Live updates** (Phase 19,
`GET /me/stream`) merged in #22 on 2026-11-09.
[Backend specs index](specs/INDEX.md).

**Frontend:** housing discovery and posting, requests and offers, session refresh,
the housing rework, Phase 12's new verification link, the Admin Portal (phases
13–15), the public-address fix and the home page refresh are shipped.
**Messaging** (frontend Phases 16–17): Phase 16, the inbox and thread with live
updates, merged in frontend #24 on 2026-11-09; Phase 17 (sharing the exact
address, cross streets, replying to offers, `/settings`) is built on
`feature/messaging-privacy`, in review as frontend #25.
[Frontend specs index](https://github.com/20age1million/WaterlooStar-Frontend/blob/main/docs/specs/INDEX.md).

## How to update this file

This is the single roadmap for the backend and frontend. Update it in the PR
that changes the plan, and date each developer decision. Each repository still
numbers and specifies its own phases; they meet only at
[api/openapi.yaml](../api/openapi.yaml). A roadmap decision sets direction;
implementation still starts with an accepted spec in the repository doing the
work. The frontend links here rather than keeping its own roadmap.

## Goal and launch — decided 2026-10-14, launch gate revised 2026-10-16

The MVP lets students find housing and post housing. Promotion to students starts
once four pieces are done: the frontend Admin Portal, the overhaul's clean-up (now frontend Phase 18),
the backend's messaging with contact privacy, and a minimal frontend messaging
phase. On **2026-10-16** the developer put messaging before launch: there is no
interim contact path, and no phone number or email is published in the meantime.
The backend's part is shipped (Phases 15–18, deployed 2026-11-07) and the
Admin Portal merged on 2026-11-06 and frontend messaging Phase 16 on
2026-11-09, so promotion now waits on frontend messaging Phase 17 (in review)
and the overhaul's clean-up.
The listing page's pinned mobile bar holds no phone number; frontend Phase 16
wired up its Message button.

The target is before the Winter 2027 term, in January 2027. If time runs short,
cut scope rather than move the date. The community forum is out of scope for
this roadmap.

## Frontend sequence — decided 2026-10-14, messaging moved 2026-10-16 and 2026-11-08

1. **Admin Portal** (Phases 13–15). Shipped 2026-11-06.
2. **Messaging, Phases 16–17** — before promotion (2026-10-16), and on
   **2026-11-08** moved ahead of the overhaul, taking Phase 16. Spec:
   the frontend's `docs/specs/implemented/messaging/`. Phase 16 merged on
   2026-11-09 (frontend #24); Phase 17 was built the same day on
   `feature/messaging-privacy`. Direction A, "Inbox", chosen
   from three demos on 2026-11-07. Phase 16: the inbox and thread at
   `/messages` and `/messages/{conversation id}` (where every notification email
   links), starting a conversation from the listing page, sending, closing,
   the header's Messages link with its unread count, and live updates from
   backend Phase 19's `GET /me/stream` (a channel, not polling, decided
   2026-11-08). Phase 17: the owner's "Share exact address", cross streets,
   replying to offers, and an "Email me about messages" switch on a `/settings`
   page (`PATCH /me/preferences`), because every email tells the student to
   turn notifications off in their settings. Backend Phase 19 merged first,
   the same day (#22).
3. **UI/UX Overhaul, from Phase 18.** It removes the remaining fixture and fake
   data (the invented numbers on `/about`, the fake question thread on the
   listing page; the home page's were removed on 2026-11-07). It replaces the
   home page with the signed-in **variant A, "Desk"**, chosen by the developer
   on 2026-10-14: offers waiting on the student first, then their places and
   requests with status, then new places for their dates and students looking.
   Its messages panel reads real conversations; questions, saved places and
   saved searches have labelled skeletons until those features exist. The
   prototype is on the frontend's local branch `t3/signed-in-home-prototypes`,
   commit `0761309`. Mobile layout is its last phase. Small UI fixes happen any
   time and are not roadmap items.
4. **Photos.**
5. **Question thread.**
6. **Poster profiles.**
7. **Saved searches.**
8. **Real maps.**

## Backend sequence — decided 2026-10-14, revised 2026-10-16

**Messaging with contact privacy**, including the address rule below, is
built and deployed: backend Phases 15–18, finished 2026-10-17, merged as four
stacked pull requests. It ran in parallel with the frontend Admin Portal and
UI/UX Overhaul. Since **2026-10-16** promotion waits on it. Answering an offer
is part of it (backend Phase 17): an offer opens a conversation with the
student who posted the request, who answers it there by replying (decided by
Claude under the developer's delegation, 2026-10-16). Email notifications are
Phase 18. Its spec is
[docs/specs/implemented/messaging/](specs/implemented/messaging/index.md).

**Live updates** (Phase 19) followed on **2026-11-08**, when the developer
chose a push channel over polling for the frontend's messaging pages and asked
for both to be built at the same time: `GET /me/stream`, server-sent events fed
by PostgreSQL `LISTEN`/`NOTIFY`. Its spec is
[docs/specs/implemented/live-updates/](specs/implemented/live-updates/index.md).
It merged in #22 on 2026-11-09, just before the frontend's messaging Phase 16.

Next come **saves, views and the question thread**, not yet specified. Each
feature starts with its own backend specification.

This explicitly replaces the order agreed on **2026-09-27**, which put saves,
views and questions before messaging with contact privacy.

## Product decisions — decided 2026-10-14

### Address privacy

A listing shows only an approximate location, such as a neighbourhood or
intersection. The exact address is revealed only when the listing's owner
shares it by hand, with one student, using “Share exact address” inside their
conversation (decided **2026-10-16**). Nothing else reveals it: not opening a
conversation, not a reply, not an offer. Only the exact address is ever
revealed; no email or phone number is shown or collected for contact. Design
this as part of messaging with contact privacy.

This settles the previously deferred question of whether the street address
stays public. **Shipped in backend messaging Phase 16** (2026-10-16; on
`feature/messaging` via #18, reaching `main` with #21): `address_line` is the private
exact address, on the owner's own reads and the admin surface only; public
reads show `neighbourhood` and a new `cross_streets`, with null coordinates;
search no longer indexes the address; and `POST
/conversations/{id}/share-address` is the owner's share, per conversation, with
no un-share. It could reach `main` only after the frontend's `fix/public-address` (frontend
pull request #19) had merged, so no public page reads a field that has gone;
that merged on 2026-11-06. No data moved: `address_line` was always separate from
`neighbourhood`. The earlier context is in the
[Platform Foundation spec](specs/implemented/platform-foundation/index.md).

### Admin creation

The first admin is created on the host with `cmd/admin promote`. Every later
admin is granted by an existing admin through the portal using
`POST /admin/users/{id}/role`. Both paths are already implemented in the backend;
[Admin](admin.md) documents their use and the safeguards.

### Providers still to be chosen

The developer chooses **image hosting** when photos start and **map/geocoding
providers** when maps start, not now. These remain deliberately deferred, with
placeholders behind interfaces. Email is already Resend, behind `email.Sender`.
