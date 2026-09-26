# Phase 7 — Offers

**Status:** Complete
**Feature:** [Housing Requests](./index.md)
**Objective:** An owner can answer a request with one of their own listings, and the student sees it.

---

## Scope

### In scope

- `request_offers`: an owner, a request, one of that owner's listings, an
  optional note.
- Three operations: make an offer, list the offers on a request, withdraw one.
- The rule that an offer is only as alive as the listing behind it.

### Out of scope

- Accepting or declining. Decided with the developer: accepting promises a
  conversation the product cannot host yet.
- Notifying the student. No email provider.
- Creating a listing and an offer in one call. The owner posts the listing, then
  offers it — two calls the API already has, which the frontend can present as
  one flow.

---

## Dependencies / Prerequisites

- Phase 6 — there is nothing to offer against until requests exist.
- The listing write path (Phase 4), which is what an offer points at.

---

## Implementation Steps

- [x] **Preparation**
  - [x] Confirm Phase 6 is complete
  - [x] Update `index.md` — set Phase 7 to In Progress

- [x] **Schema**
  - [x] Add a migration pair creating `request_offers`: request, listing, owner,
        note, `withdrawn_at`, timestamps
  - [x] Unique on (request, listing): offering the same place twice is noise,
        not emphasis
  - [x] Cascade from both request and listing, and index the lookup by request
  - [x] Verify: migrate up and down cleanly

- [x] **Queries**
  - [x] Add `internal/db/queries/offers.sql` — `CreateOffer`,
        `ListOffersForRequest`, `GetOfferForOwner`, `WithdrawOffer`,
        `CountOffersForRequest`
  - [x] `ListOffersForRequest` joins the listing and returns only offers whose
        listing is still published and which are not withdrawn
  - [x] Verify: query tests, including an offer whose listing is archived after
        the fact — it must disappear without anything else being written

- [x] **Contract**
  - [x] Add the offer schema and paths to `api/openapi.yaml`
  - [x] The offer carries the listing summary, so the student sees rent, dates
        and distance without a second call
  - [x] Verify: generators regenerate cleanly and idempotently

- [x] **Handlers**
  - [x] Add `internal/httpapi/offers.go`: create, list, withdraw
  - [x] Only a verified student may offer; only the listing's owner may offer
        it; a listing that is not published cannot be offered
  - [x] A request's offers are visible to the student who posted it and to the
        owner who made each one — not to everyone, so owners cannot read each
        other's answers
  - [x] Someone else's offer answers 404 on withdraw
  - [x] Verify: by hand — two accounts, one posts a request, the other offers a
        listing, the poster sees it, a third account sees nothing

- [x] **Tests**
  - [x] Query tests for each new query
  - [x] Handler tests in `internal/httpapi/offers_test.go`: the offer round
        trip, the visibility rule, withdrawal, and an archived listing removing
        its offer
  - [x] Extend the in-memory fake
  - [x] Verify: `go test ./...` passes

- [x] **Documentation**
  - [x] Update `README.md` and `CLAUDE.md` with the request and offer surface
  - [x] Update `docs/specs/INDEX.md` — it currently claims Discovery is next,
        which shipped two phases ago
  - [x] Verify: the phase table and the gap list match what is built

- [x] **Final verification**
  - [x] `go vet ./...` and `go build ./...` pass
  - [x] All tests pass, with and without a database available
  - [x] Update `index.md` — set Phase 7 to Complete
  - [x] Move `docs/specs/active/housing-requests/phase-7-offers.md` →
        `docs/specs/implemented/housing-requests/phase-7-offers.md`
  - [x] Move `docs/specs/active/housing-requests/index.md` →
        `docs/specs/implemented/housing-requests/index.md`

---

## Completion Criteria

- [x] All checklist items complete
- [x] An owner can offer a published listing of theirs against a request, with
      an optional note, and withdraw it again
- [x] The student who posted the request sees the offers, including the rent,
      dates and distance of each place
- [x] Another owner cannot see the offers on a request they did not post
- [x] Taking a listing down removes its offers from view, with no second step
- [x] The same listing cannot be offered twice against one request

---

## Implementation Notes

**Delivered as specified**, and the feature is complete: `request_offers`, seven
queries, three operations.

Verified against the real API as well as by test, signed in as two seeded
accounts:

- meil offered a published listing against danielo's seeded request → 201, the
  offer carrying the listing's real rent and title
- the same listing again → 409
- danielo (the poster) saw it with rent $845 and the note; the request's count
  read 1
- danielo could **not** withdraw it → 404: an offer belongs to the owner who
  made it
- meil took the listing down → the poster saw 0 offers and the count fell to 0,
  with nothing else touched

**The counter column was removed rather than maintained.** Phase 6 added
`housing_requests.offers`, and honouring it would have meant updating it from
`listings` whenever a place was taken down — a number that could only ever be
right by accident. Migration 7 drops it and the read queries count visible
offers inline. This is the same trap as the `published_at` one Phase 5 recorded,
caught before it shipped rather than after.

One consequence worth stating: dropping the column also dropped the CHECK that
guarded it, which covered `views` as well. The migration puts that half back.

**Withdrawal is a timestamp, not a delete.** The row stays, so the unique
constraint still holds and withdrawing is not a way around "one offer per
listing per request". Re-offering the same place answers 409 with a message
saying it was withdrawn, rather than a constraint violation.

**Visibility is asymmetric and tested both ways.** The poster sees every live
offer; an owner sees only their own. A test puts two owners on one request and
checks each sees exactly one.

**Not seeded.** The seed writes listings and requests but no offers, so the live
site shows the feature empty until someone uses it. Seeding them would need
pairs that make sense together, and the checklist did not ask for it.

**Still nothing tells the student.** An offer is only seen when they next open
the site. That is the email gap, not this feature's.
