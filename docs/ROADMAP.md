# WaterlooStar project roadmap

## Status

**Backend:** Phases 0–14 are shipped: accounts, housing discovery and posting,
requests and offers, rate limits, admin moderation, listing terms and email
delivery. [Backend specs index](specs/INDEX.md).

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

## Goal and launch — decided 2026-10-14

The MVP lets students find housing and post housing. Promotion to students starts
once the frontend Admin Portal and Phase 16 clean-up are done. Until messaging
exists, contact goes through the phone bar already on the listing page.

The target is before the Winter 2027 term, in January 2027. If time runs short,
cut scope rather than move the date. The community forum is out of scope for
this roadmap.

## Frontend sequence — decided 2026-10-14

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
   Later phases are defined in the overhaul's spec. Mobile layout is its last
   phase. Small UI fixes happen any time and are not roadmap items.
3. **Messaging**, after the backend's messaging feature.
4. **Photos.**
5. **Question thread.**
6. **Poster profiles.**
7. **Saved searches.**
8. **Real maps.**

## Backend sequence — decided 2026-10-14

Next is **messaging with contact privacy**, including the address rule below.
It starts now in parallel with the frontend Admin Portal and UI/UX Overhaul.
“Accept an offer” waits on messaging.

Then come **saves, views and the question thread**. Each feature starts with its
own backend specification.

This explicitly replaces the order agreed on **2026-09-27**, which put saves,
views and questions before messaging with contact privacy.

## Product decisions — decided 2026-10-14

### Address privacy

A listing shows only an approximate location, such as a neighbourhood or
intersection. The exact address is revealed once contact is confirmed. Design
this as part of messaging with contact privacy.

This settles the previously deferred question of whether the street address
stays public. The current API still exposes it; the privacy change ships with
messaging, rather than through this documentation change. The schema keeps
`address_line` separate from `neighbourhood`, so a private field can be added
without moving data. The earlier context is in the
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
