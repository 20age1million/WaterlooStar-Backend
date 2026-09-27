-- The operator's read surface, and the ledger.
--
-- Everything here is reached only through the /admin guard or cmd/admin. These
-- are the one set of queries allowed past the status = 'published' filter the
-- public reads carry, because an operator has to see what the public cannot.
--
-- admin_actions is append-only: there is an INSERT below and no UPDATE or
-- DELETE anywhere in this directory. A test in internal/httpapi fails if one is
-- added.

-- Accounts, filtered and paged, newest first. Search matches a fragment of the
-- email or the username, because an operator looking someone up has whichever
-- one the complaint quoted, and often only part of it.
-- name: ListUsersForAdmin :many
SELECT
    u.id, u.email, u.username, u.role, u.verified, u.created_at,
    u.suspended_at, u.suspend_reason,
    (SELECT count(*) FROM listings l         WHERE l.owner_id  = u.id) AS listing_count,
    (SELECT count(*) FROM housing_requests r WHERE r.poster_id = u.id) AS request_count,
    (SELECT count(*) FROM request_offers o   WHERE o.owner_id  = u.id) AS offer_count
FROM users u
WHERE (sqlc.narg('search')::text IS NULL
       OR u.email    ILIKE '%' || sqlc.narg('search')::text || '%'
       OR u.username ILIKE '%' || sqlc.narg('search')::text || '%')
  AND (sqlc.narg('role')::text     IS NULL OR u.role = sqlc.narg('role')::text)
  AND (sqlc.narg('verified')::bool IS NULL OR u.verified = sqlc.narg('verified')::bool)
  AND (sqlc.narg('suspended')::bool IS NULL
       OR (u.suspended_at IS NOT NULL) = sqlc.narg('suspended')::bool)
ORDER BY u.created_at DESC, u.id
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- The same filters, counted, so the page total describes the whole match.
-- name: CountUsersForAdmin :one
SELECT count(*)
FROM users u
WHERE (sqlc.narg('search')::text IS NULL
       OR u.email    ILIKE '%' || sqlc.narg('search')::text || '%'
       OR u.username ILIKE '%' || sqlc.narg('search')::text || '%')
  AND (sqlc.narg('role')::text     IS NULL OR u.role = sqlc.narg('role')::text)
  AND (sqlc.narg('verified')::bool IS NULL OR u.verified = sqlc.narg('verified')::bool)
  AND (sqlc.narg('suspended')::bool IS NULL
       OR (u.suspended_at IS NOT NULL) = sqlc.narg('suspended')::bool);

-- One account, with what it has posted. Counts include every status: an
-- operator needs to know about the draft as much as the published listing.
-- name: GetUserForAdmin :one
SELECT
    u.id, u.email, u.username, u.role, u.verified, u.created_at,
    u.suspended_at, u.suspend_reason,
    (SELECT count(*) FROM listings l         WHERE l.owner_id  = u.id) AS listing_count,
    (SELECT count(*) FROM housing_requests r WHERE r.poster_id = u.id) AS request_count,
    (SELECT count(*) FROM request_offers o   WHERE o.owner_id  = u.id) AS offer_count
FROM users u
WHERE u.id = sqlc.arg('id');

-- The last-admin guard reads this before any demotion.
-- name: CountAdmins :one
SELECT count(*) FROM users WHERE role = 'admin';

-- A role change. Only cmd/admin calls it in this phase; the portal gains it in
-- Phase 10. Always paired with InsertAdminAction in one transaction.
-- name: SetUserRole :one
UPDATE users SET role = sqlc.arg('role') WHERE id = sqlc.arg('id') RETURNING *;

-- The only write against admin_actions.
-- name: InsertAdminAction :one
INSERT INTO admin_actions (actor_id, action, subject_type, subject_id, reason, detail)
VALUES (
    sqlc.narg('actor_id'), sqlc.arg('action'), sqlc.arg('subject_type'),
    sqlc.arg('subject_id'), sqlc.arg('reason'), sqlc.arg('detail')
)
RETURNING *;

-- The ledger, newest first, with the actor's name. A null actor is the host.
-- name: ListAdminActions :many
SELECT
    sqlc.embed(a),
    u.username AS actor_username
FROM admin_actions a
LEFT JOIN users u ON u.id = a.actor_id
ORDER BY a.created_at DESC, a.id
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: CountAdminActions :one
SELECT count(*) FROM admin_actions;

-- What has been done to one account or post, newest first.
-- name: ListAdminActionsForSubject :many
SELECT
    sqlc.embed(a),
    u.username AS actor_username
FROM admin_actions a
LEFT JOIN users u ON u.id = a.actor_id
WHERE a.subject_type = sqlc.arg('subject_type') AND a.subject_id = sqlc.arg('subject_id')
ORDER BY a.created_at DESC, a.id;

-- The overview's numbers, in one round trip. Post counts include every status,
-- with published broken out, because "how much is live" and "how much exists"
-- are both questions an operator asks.
-- name: AdminOverviewCounts :one
SELECT
    (SELECT count(*) FROM users)                                  AS users_total,
    (SELECT count(*) FROM users WHERE verified)                   AS users_verified,
    (SELECT count(*) FROM users WHERE suspended_at IS NOT NULL)   AS users_suspended,
    (SELECT count(*) FROM users WHERE role = 'admin')             AS users_admin,
    (SELECT count(*) FROM listings)                               AS listings_total,
    (SELECT count(*) FROM listings WHERE status = 'published')    AS listings_published,
    (SELECT count(*) FROM housing_requests)                       AS requests_total,
    (SELECT count(*) FROM housing_requests WHERE status = 'published') AS requests_published,
    (SELECT count(*) FROM admin_actions
      WHERE created_at > now() - interval '7 days')               AS actions_last_week;
