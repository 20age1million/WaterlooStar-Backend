-- The operator's view of posts, and takedown.
--
-- Takedown is not status. status is the owner's; removed_at is the
-- moderator's. Restoring clears the moderator's columns and nothing else, so a
-- restored post is back in whatever status its owner had left it, having never
-- lost it.

-- EVERY listing, in any status, removed or not, whoever owns it.
--
-- This and ListRequestsForAdmin are the only queries in the repository that do
-- not filter on status = 'published', suspension and takedown. The rest of the
-- query files exist to keep that rule; these break it on purpose, because an
-- operator has to see what the public cannot. Reached only through the /admin
-- guard.
-- name: ListListingsForAdmin :many
SELECT
    sqlc.embed(l),
    u.username     AS owner_username,
    u.email        AS owner_email,
    u.avatar_url   AS owner_avatar_url,
    u.verified     AS owner_verified,
    u.suspended_at AS owner_suspended_at
FROM listings l
JOIN users u ON u.id = l.owner_id
WHERE (sqlc.narg('search')::text IS NULL
       OR l.search @@ websearch_to_tsquery('english', sqlc.narg('search')::text))
  AND (sqlc.narg('status')::text  IS NULL OR l.status = sqlc.narg('status')::text)
  AND (sqlc.narg('removed')::bool IS NULL OR (l.removed_at IS NOT NULL) = sqlc.narg('removed')::bool)
  AND (sqlc.narg('owner_id')::uuid IS NULL OR l.owner_id = sqlc.narg('owner_id')::uuid)
ORDER BY l.created_at DESC, l.id
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: CountListingsForAdmin :one
SELECT count(*)
FROM listings l
WHERE (sqlc.narg('search')::text IS NULL
       OR l.search @@ websearch_to_tsquery('english', sqlc.narg('search')::text))
  AND (sqlc.narg('status')::text  IS NULL OR l.status = sqlc.narg('status')::text)
  AND (sqlc.narg('removed')::bool IS NULL OR (l.removed_at IS NOT NULL) = sqlc.narg('removed')::bool)
  AND (sqlc.narg('owner_id')::uuid IS NULL OR l.owner_id = sqlc.narg('owner_id')::uuid);

-- EVERY request, in any status, removed or not. The same deliberate exception
-- as ListListingsForAdmin; see there. The offer count is the one the poster
-- sees: live offers on visible listings from active owners.
-- name: ListRequestsForAdmin :many
SELECT
    sqlc.embed(r),
    u.username     AS poster_username,
    u.email        AS poster_email,
    u.avatar_url   AS poster_avatar_url,
    u.verified     AS poster_verified,
    u.suspended_at AS poster_suspended_at,
    (SELECT count(*)
       FROM request_offers o
       JOIN listings ol ON ol.id = o.listing_id
       JOIN users ou    ON ou.id = o.owner_id
      WHERE o.request_id = r.id
        AND o.withdrawn_at IS NULL
        AND ol.status = 'published'
        AND ol.removed_at IS NULL
        AND ou.suspended_at IS NULL) AS offer_count
FROM housing_requests r
JOIN users u ON u.id = r.poster_id
WHERE (sqlc.narg('search')::text IS NULL
       OR r.search @@ websearch_to_tsquery('english', sqlc.narg('search')::text))
  AND (sqlc.narg('status')::text  IS NULL OR r.status = sqlc.narg('status')::text)
  AND (sqlc.narg('removed')::bool IS NULL OR (r.removed_at IS NOT NULL) = sqlc.narg('removed')::bool)
  AND (sqlc.narg('owner_id')::uuid IS NULL OR r.poster_id = sqlc.narg('owner_id')::uuid)
ORDER BY r.created_at DESC, r.id
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: CountRequestsForAdmin :one
SELECT count(*)
FROM housing_requests r
WHERE (sqlc.narg('search')::text IS NULL
       OR r.search @@ websearch_to_tsquery('english', sqlc.narg('search')::text))
  AND (sqlc.narg('status')::text  IS NULL OR r.status = sqlc.narg('status')::text)
  AND (sqlc.narg('removed')::bool IS NULL OR (r.removed_at IS NOT NULL) = sqlc.narg('removed')::bool)
  AND (sqlc.narg('owner_id')::uuid IS NULL OR r.poster_id = sqlc.narg('owner_id')::uuid);

-- Takedown. Only a post not already removed matches, so a second takedown
-- cannot overwrite the first one's time and reason; no row back means that.
-- name: RemoveListing :one
UPDATE listings
SET removed_at = now(), removed_by = sqlc.narg('actor_id'), removed_reason = sqlc.arg('reason')
WHERE id = sqlc.arg('id') AND removed_at IS NULL
RETURNING *;

-- name: RestoreListing :one
UPDATE listings
SET removed_at = NULL, removed_by = NULL, removed_reason = NULL
WHERE id = sqlc.arg('id') AND removed_at IS NOT NULL
RETURNING *;

-- name: RemoveRequest :one
UPDATE housing_requests
SET removed_at = now(), removed_by = sqlc.narg('actor_id'), removed_reason = sqlc.arg('reason')
WHERE id = sqlc.arg('id') AND removed_at IS NULL
RETURNING *;

-- name: RestoreRequest :one
UPDATE housing_requests
SET removed_at = NULL, removed_by = NULL, removed_reason = NULL
WHERE id = sqlc.arg('id') AND removed_at IS NOT NULL
RETURNING *;
