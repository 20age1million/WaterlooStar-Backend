# Email Delivery — API Phases 13–14

**Status:** In Progress
**Branch:** `feature/email-delivery`
**Ticket / Work Item:** N/A
**Owner(s):** WaterlooStar backend
**Date:** 2026-10-07 (provider changed 2026-10-11)

---

## Purpose / Goal

Verification and password-reset emails reach the student's inbox, so a student
can sign up on the live site without an admin verifying them by hand. A student
whose link is lost or expired can ask for a new one.

---

## Problem Statement / Motivation

- **Nobody on the live site can finish signing up.** `internal/email` has one
  implementation, `LogSender`, which prints the link to the API log. Today a
  real student gets in only when an admin verifies the account by hand
  (Phase 10).
- **Password reset doesn't work either,** for the same reason.
- **A lost link is a dead end.** `Register` logs a failed send with the comment
  "let them request a new link", but no endpoint sends one. A verification link
  lasts 24 hours. A student who misses it cannot verify, and re-registering
  answers 409.
- **Everything later depends on it.** Contact requests and messaging (agreed
  2026-09-27) notify people by email. This feature comes before them.

---

## Proposed Solution / Design

Two phases.

1. **Phase 13 — Resend sender.** A `ResendSender` in `internal/email` implements
   the existing `Sender` interface through Resend's public email API. It is
   chosen at startup when `RESEND_API_KEY` is set. Without a
   key the service keeps `LogSender`, in production too, with a loud warning at
   startup. The From address comes from configuration.
2. **Phase 14 — A new verification link.** `POST /auth/verify/resend` takes an
   email address and answers 202 whatever the address. When it belongs to an
   unverified, unsuspended account, it spends the outstanding link and sends a
   new one. It is rate-limited like the password-reset request.

Resend is a sender only. Authentication stays the custom auth built in Phases 1
and 5.

**The provider changed on 2026-10-11, from Clerk to Resend** (the developer's
decision). Clerk was chosen on 2026-09-25 with its risk stated: its
transactional email endpoint is internal and not yet public. That risk came
true. With the developer's development key and then a production key on
`clerk.waterloostar.com`, `POST /v1/email` answered 404 `resource_not_found`,
while other calls with the same keys succeeded. The endpoint is not enabled for
the account. Clerk's trace IDs: `d70d5c63e09f4634c79d491e0747e500` and
`17066ea14e0bac5cd0e1b4c67f567840`.

### Key Components

- **`internal/email/resend.go`** (new): `ResendSender`. It posts a `Message` to
  Resend's `/emails` with `net/http`: the configured From mailbox, the
  recipient, the subject and a plain-text body. A message's ID is sent as the
  idempotency key. The send has its own timeout. An error names Resend's status
  and error name, and never carries the key, the link, the body or the
  recipient.
- **`internal/email/sender.go`**: `Message` gains an `ID`. The package comment
  stops saying no provider is chosen.
- **`internal/config`**: `RESEND_API_KEY` (optional) and `EMAIL_FROM`
  (defaults to `WaterlooStar <no-reply@waterloostar.com>`), parsed and
  validated at startup like every other setting.
- **`cmd/api`**: picks the sender from the configuration and logs which one.
- **`api/openapi.yaml`**, **`internal/httpapi/auth.go`**: the resend operation
  (Phase 14).
- **`internal/ratelimit/policy.go`**, **`internal/httpapi/ratelimit_mw.go`**: its
  allowances (Phase 14).

### Data / Control Flow

- Registration, a reset request and a resend request each create a token, build
  a `Message` and call `Sender.Send` inside the request, as today.
- `ResendSender.Send` calls Resend with a short timeout. Success means Resend
  accepted the message, not that it was delivered.
- A failed send is logged with the user's ID and Resend's error. The response
  does not change: registration still answers 201, and a reset or resend
  request still answers 202. The student asks for a new link (Phase 14).
- The message's ID is derived from its token, so a send repeated for the same
  link is not delivered twice. A new link is a new message.

---

## Layers / Areas Affected

| Layer / Area | Change |
|---|---|
| Email | `ResendSender` (new); `Message.ID`; package comment |
| Configuration | `RESEND_API_KEY`, `EMAIL_FROM`; `.env.example` |
| Wiring | `cmd/api` chooses the sender |
| API contract | `POST /auth/verify/resend` (Phase 14) |
| Handlers | `ResendVerification`; `Register`'s comment names it |
| Rate limiting | Per-email and global allowances for the resend |
| Database | None. The existing token queries cover it; no migration |
| Dependencies | None. Resend's API is called with `net/http` |
| Tests | The sender against a fake Resend server; config; the resend handler and its limit |
| Deployment | `deploy/docker-compose.yml` passes the two settings; `.drone.yml`'s setup notes list them |
| Docs | `README.md` (endpoints, rate limits, running it), `CLAUDE.md` |

---

## Phase Tracker

| Phase | Title | Status | Location |
|---|---|---|---|
| [13 — Resend sender](../../implemented/email-delivery/phase-13-resend-sender.md) | Verification and reset emails delivered through Resend; the log stays the fallback | Complete | `docs/specs/implemented/email-delivery/` |
| [14 — A new verification link](./phase-14-resend-verification.md) | `POST /auth/verify/resend`, rate-limited, the same answer for any address | Ready | `docs/specs/active/email-delivery/` |

---

## Out of scope

- **HTML email.** Bodies stay plain text, as they are written today.
- **Delivery status.** Resend can report bounces and deliveries through
  webhooks. Nothing listens yet.
- **An outbox with retries.** Sending stays inside the request (the
  developer's decision). A failure is recovered by asking for a new link.
- **Notification emails** (offers, contact requests, messages). These come with
  the features that need them.
- **The frontend's "Send a new link" button.** That is the frontend's own
  phase, generated from this contract.

---

## Trade-offs / Alternatives Considered

- **Resend** rather than Clerk, Postmark or SES. Clerk was the first choice
  and its endpoint is not open to the account (see above). Resend's email API
  is public, documented and has a free tier. Postmark and SES were the other
  dedicated options offered. A future switch replaces only `resend.go`,
  because everything else sees `Sender`.
- **No SDK.** Resend's Go SDK reports most errors as bare strings without a
  status, and pulls in a large client for one call. A direct `net/http` call
  keeps the dependency list unchanged and decides exactly what an error may
  contain.
- **Sending inside the request** rather than from an outbox: the developer's
  decision. No table and no worker. The cost is that a Resend outage costs a
  student one link. The resend endpoint is how they recover.
- **Falling back to the log in production** rather than refusing to start: the
  developer's decision. A deploy without the key keeps working as it does
  today, and the startup warning says so. While it does, verification and
  reset links are credentials sitting in the API log. That is today's
  situation, and it ends once the key is set.
- **The resend takes an email address,** not a session, like the reset request.
  A student who closed the tab after registering has no session to use.

---

## Assumptions

- `waterloostar.com` can be verified as a sending domain in Resend, through
  DNS records the developer adds. Resend refuses a From address on an
  unverified domain.
- Resend's free tier is enough for verification and reset email at
  WaterlooStar's volume.

---

## Constraints

- **No test may call Resend.** CI has no key, and every test runs against a fake
  server.
- **Neither the key nor a link appears in an error.** The link is a credential.
  `LogSender` is the only thing that prints one, and it is used only when
  there is no key.
- **The resend never reveals whether an address has an account** *(per
  `CLAUDE.md`, as the reset request does)*. It gives the same status and the
  same body for every address.
- **Every new endpoint goes on the rate-limit allowlist,** or it is unlimited
  *(per `internal/httpapi/ratelimit_mw.go`)*.
- **The contract comes first** *(per `CLAUDE.md`)*. The resend is added to
  `api/openapi.yaml` before its handler.

---

## Success Criteria / Definition of Done

- With a key set, registering a `uwaterloo.ca` address delivers a verification
  email from `no-reply@waterloostar.com`, and its link verifies the account.
- Without a key, the service behaves as it does today and says so at startup.
- A student can ask for a new link, the old one stops working, and a fourth
  request within the hour answers 429.
- `go build ./...`, `go vet ./...` and `go test ./...` pass. The query guard
  still passes.
- All phase checklists completed and verified.

---

## References (internal only)

- The seam: `internal/email/sender.go`
- The call sites: `internal/httpapi/auth.go` (`issueVerificationEmail`,
  `issuePasswordResetEmail`)
- Rate limiting: `docs/specs/implemented/rate-limiting/index.md`,
  `deploy/RATE-LIMITING.md`
- Hand verification, the fallback until this lands:
  `docs/specs/implemented/admin-moderation/phase-10-account-management.md`
