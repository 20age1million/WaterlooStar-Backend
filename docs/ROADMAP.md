# WaterlooStar project roadmap

## Status

**Backend:** Phases 0–14 are shipped: accounts, housing discovery and posting,
requests and offers, rate limits, admin moderation, listing terms and email
delivery. **Messaging with contact privacy is built** (Phases 15–18) and
awaits merge as four stacked pull requests: Phase 15, conversations and
messages, on `feature/messaging` (#17); Phase 16, address privacy and sharing,
on `feature/messaging-privacy` (#18); Phase 17, offers opening conversations,
on `feature/messaging-offers` (#19); Phase 18, email notifications, on
`feature/messaging-notifications` (pull request to be opened). None is merged.
[Backend specs index](specs/INDEX.md).

**Frontend:** housing discovery and posting, requests and offers, session refresh,
the housing rework and Phase 12's new verification link are shipped.
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
once four pieces are done: the frontend Admin Portal, frontend Phase 16 clean-up,
the backend's messaging with contact privacy, and a minimal frontend messaging
phase. On **2026-10-16** the developer put messaging before launch: there is no
interim contact path, and no phone number or email is published in the meantime.
The backend's part is built (Phases 15–18, 2026-10-17, awaiting merge), so
promotion now waits only on the frontend's three pieces and on those merges.
The listing page's pinned mobile bar holds no phone number; the frontend
messaging phase wires up its Message button.

The target is before the Winter 2027 term, in January 2027. If time runs short,
cut scope rather than move the date. The community forum is out of scope for
this roadmap.

## Frontend sequence — decided 2026-10-14, messaging moved 2026-10-16

1. **Admin Portal.** Write a new spec: the earlier `feature/admin-portal` branch
   is lost. Phase 13 builds the shell, access guard, overview and action ledger;
   non-admins get 404, as they do from the API. Phase 14 adds user management;
   Phase 15 adds post moderation. The backend admin contract is complete, so
   this work is frontend-only.
2. **UI/UX Overhaul, from Phase 16.** Phase 16 removes all fixture and fake data:
   the home page feed, section counts and term figures; the invented numbers on
   `/about`; and the fake question thread on the listing page. It removes header
   links to features that do not exist: Community, Messages and Saved. It also
   replaces the home page with the signed-in **variant A, “Desk”**, chosen by the
   developer on 2026-10-14. This dashboard shows the student's own business:
   offers waiting on them first, then their places and requests with status,
   then new places for their dates and students looking. Messages, questions,
   saved places and saved searches have labelled skeletons until those features
   exist. The prototype is on the frontend's local branch
   `t3/signed-in-home-prototypes`, commit `0761309`; Phase 16 builds it properly.
   The overhaul's later phases are defined in its spec and follow the messaging
   phase below. Mobile layout is its last phase. Small UI fixes happen any time
   and are not roadmap items.
3. **Messaging, before promotion** — moved ahead on 2026-10-16. A minimal
   phase, numbered by the frontend's own spec, straight after Phase 16 (that
   placement was decided by Claude under the developer's delegation,
   2026-10-16): the listing page's Message buttons start a conversation; the
   inbox and the thread; the owner's “Share exact address” action; closing a
   conversation; and replying to an offer. It restores the header's Messages
   link that Phase 16 removes and fills the Desk home's messages skeleton. It
   can start once backend messaging Phases 15–16 are on `main`; replying to
   offers needs backend Phase 17 (built on `feature/messaging-offers`, not yet
   merged: offers carry `conversation`, and the poster replies there). Before backend Phase 16 merges, the frontend
   stops reading the exact address from public listings, as a small UI fix.
   Backend Phase 18 sends the notification emails, which link to
   `/messages/{conversation id}`, so the thread must live at that path; the
   phase also adds an "Email me about messages" switch
   (`PATCH /me/preferences`, `email_notifications` on `GET /me`), because every
   email tells the student to turn notifications off in their settings.
   Anything beyond the minimum gets its own spec later.
4. **Photos.**
5. **Question thread.**
6. **Poster profiles.**
7. **Saved searches.**
8. **Real maps.**

## Backend sequence — decided 2026-10-14, revised 2026-10-16

**Messaging with contact privacy**, including the address rule below, is
built: backend Phases 15–18, finished 2026-10-17 and awaiting merge as four
stacked pull requests. It ran in parallel with the frontend Admin Portal and
UI/UX Overhaul. Since **2026-10-16** promotion waits on it. Answering an offer
is part of it (backend Phase 17): an offer opens a conversation with the
student who posted the request, who answers it there by replying (decided by
Claude under the developer's delegation, 2026-10-16). Email notifications are
Phase 18. Its spec is
[docs/specs/implemented/messaging/](specs/implemented/messaging/index.md).

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
stays public. **Shipped in backend messaging Phase 16** (2026-10-16, on
`feature/messaging-privacy`, not yet merged): `address_line` is the private
exact address, on the owner's own reads and the admin surface only; public
reads show `neighbourhood` and a new `cross_streets`, with null coordinates;
search no longer indexes the address; and `POST
/conversations/{id}/share-address` is the owner's share, per conversation, with
no un-share. It merges only after the frontend's `fix/public-address` (frontend
pull request #19) has merged and deployed, so no public page reads a field that
has gone. No data moved: `address_line` was always separate from
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
