# Phase 7 — Offers

**Status:** Ready
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

- [ ] **Preparation**
  - [ ] Confirm Phase 6 is complete
  - [ ] Update `index.md` — set Phase 7 to In Progress

- [ ] **Schema**
  - [ ] Add a migration pair creating `request_offers`: request, listing, owner,
        note, `withdrawn_at`, timestamps
  - [ ] Unique on (request, listing): offering the same place twice is noise,
        not emphasis
  - [ ] Cascade from both request and listing, and index the lookup by request
  - [ ] Verify: migrate up and down cleanly

- [ ] **Queries**
  - [ ] Add `internal/db/queries/offers.sql` — `CreateOffer`,
        `ListOffersForRequest`, `GetOfferForOwner`, `WithdrawOffer`,
        `CountOffersForRequest`
  - [ ] `ListOffersForRequest` joins the listing and returns only offers whose
        listing is still published and which are not withdrawn
  - [ ] Verify: query tests, including an offer whose listing is archived after
        the fact — it must disappear without anything else being written

- [ ] **Contract**
  - [ ] Add the offer schema and paths to `api/openapi.yaml`
  - [ ] The offer carries the listing summary, so the student sees rent, dates
        and distance without a second call
  - [ ] Verify: generators regenerate cleanly and idempotently

- [ ] **Handlers**
  - [ ] Add `internal/httpapi/offers.go`: create, list, withdraw
  - [ ] Only a verified student may offer; only the listing's owner may offer
        it; a listing that is not published cannot be offered
  - [ ] A request's offers are visible to the student who posted it and to the
        owner who made each one — not to everyone, so owners cannot read each
        other's answers
  - [ ] Someone else's offer answers 404 on withdraw
  - [ ] Verify: by hand — two accounts, one posts a request, the other offers a
        listing, the poster sees it, a third account sees nothing

- [ ] **Tests**
  - [ ] Query tests for each new query
  - [ ] Handler tests in `internal/httpapi/offers_test.go`: the offer round
        trip, the visibility rule, withdrawal, and an archived listing removing
        its offer
  - [ ] Extend the in-memory fake
  - [ ] Verify: `go test ./...` passes

- [ ] **Documentation**
  - [ ] Update `README.md` and `CLAUDE.md` with the request and offer surface
  - [ ] Update `docs/specs/INDEX.md` — it currently claims Discovery is next,
        which shipped two phases ago
  - [ ] Verify: the phase table and the gap list match what is built

- [ ] **Final verification**
  - [ ] `go vet ./...` and `go build ./...` pass
  - [ ] All tests pass, with and without a database available
  - [ ] Update `index.md` — set Phase 7 to Complete
  - [ ] Move `docs/specs/active/housing-requests/phase-7-offers.md` →
        `docs/specs/implemented/housing-requests/phase-7-offers.md`
  - [ ] Move `docs/specs/active/housing-requests/index.md` →
        `docs/specs/implemented/housing-requests/index.md`

---

## Completion Criteria

- [ ] All checklist items complete
- [ ] An owner can offer a published listing of theirs against a request, with
      an optional note, and withdraw it again
- [ ] The student who posted the request sees the offers, including the rent,
      dates and distance of each place
- [ ] Another owner cannot see the offers on a request they did not post
- [ ] Taking a listing down removes its offers from view, with no second step
- [ ] The same listing cannot be offered twice against one request
