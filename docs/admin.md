# Admin

Every `/admin` path answers **404** to anyone who is not an admin, including an
anonymous caller, and the response is identical to an unknown path's. Nothing
tells a prober the surface exists.

The first admin is created on the host with `cmd/admin promote`. Every later
admin is granted by an existing admin through the portal, using
`POST /admin/users/{id}/role`. The backend already implements both paths; see
[the roadmap decision](ROADMAP.md#admin-creation). Only an existing admin can
call the HTTP role-change endpoint. The CLI also supports listing and demoting
admins:

```bash
go run ./cmd/admin promote you@uwaterloo.ca -reason "Site operator"
go run ./cmd/admin list
go run ./cmd/admin demote  you@uwaterloo.ca -reason "Stepped down"
```

The account must already exist, so register it first. A role change ends the
account's sessions, and the new role applies from the next login. The last
admin cannot be demoted.

**Suspension** is reversible and changes nothing an account posted. A suspended
account cannot log in (a 403, given only after the right password) or refresh,
its sessions end, and its listings, requests and offers leave every public read,
because the read queries filter on it. Reinstating brings all of it back as it
was. Every action needs a reason of at least ten characters. An admin cannot
suspend themselves or change their own role.

**Takedown** is a moderator's axis, separate from the owner's `status`. A removed
listing or request leaves every public read whatever its status, its owner still
sees it with the moderator's reason, and the owner cannot edit, republish or
archive it (409) until it is restored. Restoring returns it in whatever status
the owner had left it.

**Verifying by hand** is how a real student gets in when verification emails
only reach the log — a deploy without `RESEND_API_KEY`.

Every admin change, from the CLI or the portal, is written to
`admin_actions` in the same transaction as the change itself, with a reason.
The table is append-only: no query updates or deletes a row, and a test fails
if one is added.

## Endpoints

| Method | Path | Auth | Purpose |
|---|---|---|---|
| GET | `/admin/overview` | admin | Counts across the site |
| GET | `/admin/users` | admin | Every account, searchable and filterable |
| GET | `/admin/users/{id}` | admin | One account, with what admins have done to it |
| GET | `/admin/actions` | admin | The ledger of every admin change |
| POST | `/admin/users/{id}/suspend` | admin | Suspend an account |
| POST | `/admin/users/{id}/reinstate` | admin | Lift a suspension |
| POST | `/admin/users/{id}/verify` | admin | Verify an account by hand |
| POST | `/admin/users/{id}/role` | admin | Move an account between user and admin |
| GET | `/admin/listings` | admin | Every listing, in any state |
| POST | `/admin/listings/{id}/remove` | admin | Take a listing down |
| POST | `/admin/listings/{id}/restore` | admin | Restore it |
| GET | `/admin/requests` | admin | Every request, in any state |
| POST | `/admin/requests/{id}/remove` | admin | Take a request down |
| POST | `/admin/requests/{id}/restore` | admin | Restore it |
