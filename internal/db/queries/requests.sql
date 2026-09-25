-- "Looking for Housing" posts. Only published rows are ever served, and that
-- stays here in the SQL so a handler cannot forget it.
--
-- The filters read from the owner's side: a request's budget is a ceiling, so
-- "budget_min" asks who can afford at least this much, and its distance is a
-- radius, so "distance_max" asks who would accept a place this far out.

-- name: ListRequests :many
SELECT
    sqlc.embed(r),
    u.username   AS poster_username,
    u.avatar_url AS poster_avatar_url,
    u.verified   AS poster_verified
FROM housing_requests r
JOIN users u ON u.id = r.poster_id
WHERE r.status = 'published'
  AND (sqlc.narg('search')::text IS NULL
       OR r.search @@ websearch_to_tsquery('english', sqlc.narg('search')::text))
  -- A request wants a place for its own window: it overlaps what an owner has
  -- if it starts by then and runs to at least the end of it.
  AND (sqlc.narg('start_after')::date IS NULL OR r.start_date <= sqlc.narg('start_after')::date)
  AND (sqlc.narg('end_before')::date  IS NULL OR r.end_date   >= sqlc.narg('end_before')::date)
  -- Budget is what they will pay at most, so an owner asking $900 wants
  -- budget_min = 90000: everyone whose ceiling reaches their rent.
  AND (sqlc.narg('budget_min')::int    IS NULL OR r.budget_cents >= sqlc.narg('budget_min')::int)
  AND (sqlc.narg('budget_max')::int    IS NULL OR r.budget_cents <= sqlc.narg('budget_max')::int)
  -- "Would this person consider a place this far out?" A request with no stated
  -- radius is included: no preference is not the same as a narrow one, which is
  -- the opposite of how a listing's unknown distance is treated.
  AND (sqlc.narg('distance_min')::int  IS NULL
       OR r.max_distance_m IS NULL
       OR r.max_distance_m >= sqlc.narg('distance_min')::int)
  AND (sqlc.narg('occupants_max')::int IS NULL OR r.occupants <= sqlc.narg('occupants_max')::int)
  AND (sqlc.narg('pets')::bool         IS NULL OR r.pets = sqlc.narg('pets')::bool)
  AND (sqlc.narg('furnished')::bool    IS NULL OR r.furnished_preferred = sqlc.narg('furnished')::bool)
  AND (sqlc.narg('parking')::bool      IS NULL OR r.parking_needed = sqlc.narg('parking')::bool)
  AND (sqlc.narg('laundry')::bool      IS NULL OR r.laundry_needed = sqlc.narg('laundry')::bool)
  AND (sqlc.narg('verified_only')::bool IS NULL
       OR sqlc.narg('verified_only')::bool = false
       OR u.verified = true)
ORDER BY
    CASE WHEN sqlc.arg('sort')::text = 'budgetDesc' THEN r.budget_cents END DESC,
    CASE WHEN sqlc.arg('sort')::text = 'budgetAsc'  THEN r.budget_cents END ASC,
    CASE WHEN sqlc.arg('sort')::text = 'soonest'    THEN r.start_date END ASC,
    -- 'new' and 'match' both fall through to newest first, as on listings.
    r.created_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: CountRequests :one
SELECT count(*)
FROM housing_requests r
JOIN users u ON u.id = r.poster_id
WHERE r.status = 'published'
  AND (sqlc.narg('search')::text IS NULL
       OR r.search @@ websearch_to_tsquery('english', sqlc.narg('search')::text))
  AND (sqlc.narg('start_after')::date IS NULL OR r.start_date <= sqlc.narg('start_after')::date)
  AND (sqlc.narg('end_before')::date  IS NULL OR r.end_date   >= sqlc.narg('end_before')::date)
  AND (sqlc.narg('budget_min')::int    IS NULL OR r.budget_cents >= sqlc.narg('budget_min')::int)
  AND (sqlc.narg('budget_max')::int    IS NULL OR r.budget_cents <= sqlc.narg('budget_max')::int)
  AND (sqlc.narg('distance_min')::int  IS NULL
       OR r.max_distance_m IS NULL
       OR r.max_distance_m >= sqlc.narg('distance_min')::int)
  AND (sqlc.narg('occupants_max')::int IS NULL OR r.occupants <= sqlc.narg('occupants_max')::int)
  AND (sqlc.narg('pets')::bool         IS NULL OR r.pets = sqlc.narg('pets')::bool)
  AND (sqlc.narg('furnished')::bool    IS NULL OR r.furnished_preferred = sqlc.narg('furnished')::bool)
  AND (sqlc.narg('parking')::bool      IS NULL OR r.parking_needed = sqlc.narg('parking')::bool)
  AND (sqlc.narg('laundry')::bool      IS NULL OR r.laundry_needed = sqlc.narg('laundry')::bool)
  AND (sqlc.narg('verified_only')::bool IS NULL
       OR sqlc.narg('verified_only')::bool = false
       OR u.verified = true);

-- name: GetPublishedRequest :one
SELECT
    sqlc.embed(r),
    u.username   AS poster_username,
    u.avatar_url AS poster_avatar_url,
    u.verified   AS poster_verified
FROM housing_requests r
JOIN users u ON u.id = r.poster_id
WHERE r.id = sqlc.arg('id') AND r.status = 'published';

-- ---------------------------------------------------------------- write path

-- The poster's own requests, every status. The public endpoints show only
-- published ones, so this is the only way to see a draft or a paused request.
-- name: ListRequestsByPoster :many
SELECT
    sqlc.embed(r),
    u.username   AS poster_username,
    u.avatar_url AS poster_avatar_url,
    u.verified   AS poster_verified
FROM housing_requests r
JOIN users u ON u.id = r.poster_id
WHERE r.poster_id = sqlc.arg('poster_id')
ORDER BY r.created_at DESC;

-- Any status, so ownership can be checked before an edit. "Not yours" and "does
-- not exist" answer alike in the handler.
-- name: GetRequestForOwner :one
SELECT * FROM housing_requests WHERE id = sqlc.arg('id');

-- published_at is stamped here, on creation, when the request goes straight to
-- published. listings.published_at is only set by a later status change, which
-- leaves a listing created as published without one; that is not repeated.
-- name: CreateRequest :one
INSERT INTO housing_requests (
    poster_id, title, body,
    budget_cents,
    start_date, end_date, lease_months, term_tag,
    occupants, pets, furnished_preferred, parking_needed, laundry_needed,
    max_distance_m, neighbourhood,
    views,
    status, published_at, created_at
) VALUES (
    $1, $2, $3,
    $4,
    $5, $6, $7, $8,
    $9, $10, $11, $12, $13,
    $14, $15,
    -- Only the seed sets this; a real request starts at zero.
    sqlc.arg('views'),
    sqlc.arg('status')::text,
    CASE WHEN sqlc.arg('status')::text = 'published' THEN now() END,
    sqlc.arg('created_at')
)
RETURNING *;

-- A partial edit: anything not given keeps its current value. A PUT would make
-- every omitted field a deletion.
-- name: UpdateRequest :one
UPDATE housing_requests
SET title               = coalesce(sqlc.narg('title'),               title),
    body                = coalesce(sqlc.narg('body'),                body),
    budget_cents        = coalesce(sqlc.narg('budget_cents'),        budget_cents),
    start_date          = coalesce(sqlc.narg('start_date'),          start_date),
    end_date            = coalesce(sqlc.narg('end_date'),            end_date),
    lease_months        = coalesce(sqlc.narg('lease_months'),        lease_months),
    term_tag            = coalesce(sqlc.narg('term_tag'),            term_tag),
    occupants           = coalesce(sqlc.narg('occupants'),           occupants),
    pets                = coalesce(sqlc.narg('pets'),                pets),
    furnished_preferred = coalesce(sqlc.narg('furnished_preferred'), furnished_preferred),
    parking_needed      = coalesce(sqlc.narg('parking_needed'),      parking_needed),
    laundry_needed      = coalesce(sqlc.narg('laundry_needed'),      laundry_needed),
    max_distance_m      = coalesce(sqlc.narg('max_distance_m'),      max_distance_m),
    neighbourhood       = coalesce(sqlc.narg('neighbourhood'),       neighbourhood)
WHERE id = sqlc.arg('id')
RETURNING *;

-- Cast on both uses of the parameter: referring to it as varchar in one place
-- and ::text in another makes PostgreSQL refuse to deduce a type (SQLSTATE
-- 42P08). That shipped once already on listings.
-- name: SetRequestStatus :one
UPDATE housing_requests
SET status = sqlc.arg('status')::text,
    published_at = CASE
        WHEN sqlc.arg('status')::text = 'published' AND published_at IS NULL THEN now()
        ELSE published_at
    END
WHERE id = sqlc.arg('id')
RETURNING *;

-- What cmd/seed calls before reseeding.
-- name: DeleteAllRequests :exec
DELETE FROM housing_requests;
