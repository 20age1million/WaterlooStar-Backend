-- Only published rows are ever served. Filtering here rather than in Go means a
-- handler cannot forget and expose somebody's draft.

-- Filtered browse. Every parameter is optional and they compose: the
-- "@param IS NULL OR ..." shape means one prepared statement serves every
-- combination, with no string building anywhere near user input.
--
-- Only published rows are ever returned, and that stays here in the SQL so a
-- handler cannot forget it.

-- name: ListListings :many
SELECT
    sqlc.embed(l),
    u.username    AS owner_username,
    u.avatar_url  AS owner_avatar_url,
    u.verified    AS owner_verified
FROM listings l
JOIN users u ON u.id = l.owner_id
WHERE l.status = 'published'
  AND (sqlc.narg('search')::text IS NULL
       OR l.search @@ websearch_to_tsquery('english', sqlc.narg('search')::text))
  -- A listing is available for a wanted window if it starts by then and runs
  -- to at least the end of it.
  AND (sqlc.narg('start_after')::date  IS NULL OR l.start_date <= sqlc.narg('start_after')::date)
  AND (sqlc.narg('end_before')::date   IS NULL OR l.end_date   >= sqlc.narg('end_before')::date)
  AND (sqlc.narg('price_min')::int     IS NULL OR l.price_cents >= sqlc.narg('price_min')::int)
  AND (sqlc.narg('price_max')::int     IS NULL OR l.price_cents <= sqlc.narg('price_max')::int)
  -- A listing with no recorded distance is excluded when a limit is asked for:
  -- "within 2 km" cannot honestly include "distance unknown".
  AND (sqlc.narg('distance_max')::int  IS NULL OR l.distance_m <= sqlc.narg('distance_max')::int)
  AND (sqlc.narg('bedrooms_min')::int  IS NULL OR l.bedrooms_total >= sqlc.narg('bedrooms_min')::int)
  AND (sqlc.narg('furnished')::bool    IS NULL OR l.furnished = sqlc.narg('furnished')::bool)
  AND (sqlc.narg('parking')::bool      IS NULL OR l.parking   = sqlc.narg('parking')::bool)
  AND (sqlc.narg('pets')::bool         IS NULL OR l.pets      = sqlc.narg('pets')::bool)
  AND (sqlc.narg('laundry')::bool      IS NULL OR l.laundry   = sqlc.narg('laundry')::bool)
  -- Every requested utility must be included, not just one of them.
  AND (sqlc.narg('utilities')::text[]  IS NULL OR l.utilities @> sqlc.narg('utilities')::text[])
  AND (sqlc.narg('verified_only')::bool IS NULL
       OR sqlc.narg('verified_only')::bool = false
       OR u.verified = true)
ORDER BY
    CASE WHEN sqlc.arg('sort')::text = 'priceAsc'  THEN l.price_cents END ASC,
    CASE WHEN sqlc.arg('sort')::text = 'priceDesc' THEN l.price_cents END DESC,
    -- NULLS LAST: a listing with no distance should not lead a distance sort.
    CASE WHEN sqlc.arg('sort')::text = 'distance'  THEN l.distance_m END ASC NULLS LAST,
    -- 'new' and 'match' both fall through to newest first. Real match scoring
    -- needs the viewer's own request, which does not exist yet.
    l.created_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: CountListings :one
SELECT count(*)
FROM listings l
JOIN users u ON u.id = l.owner_id
WHERE l.status = 'published'
  AND (sqlc.narg('search')::text IS NULL
       OR l.search @@ websearch_to_tsquery('english', sqlc.narg('search')::text))
  AND (sqlc.narg('start_after')::date  IS NULL OR l.start_date <= sqlc.narg('start_after')::date)
  AND (sqlc.narg('end_before')::date   IS NULL OR l.end_date   >= sqlc.narg('end_before')::date)
  AND (sqlc.narg('price_min')::int     IS NULL OR l.price_cents >= sqlc.narg('price_min')::int)
  AND (sqlc.narg('price_max')::int     IS NULL OR l.price_cents <= sqlc.narg('price_max')::int)
  AND (sqlc.narg('distance_max')::int  IS NULL OR l.distance_m <= sqlc.narg('distance_max')::int)
  AND (sqlc.narg('bedrooms_min')::int  IS NULL OR l.bedrooms_total >= sqlc.narg('bedrooms_min')::int)
  AND (sqlc.narg('furnished')::bool    IS NULL OR l.furnished = sqlc.narg('furnished')::bool)
  AND (sqlc.narg('parking')::bool      IS NULL OR l.parking   = sqlc.narg('parking')::bool)
  AND (sqlc.narg('pets')::bool         IS NULL OR l.pets      = sqlc.narg('pets')::bool)
  AND (sqlc.narg('laundry')::bool      IS NULL OR l.laundry   = sqlc.narg('laundry')::bool)
  AND (sqlc.narg('utilities')::text[]  IS NULL OR l.utilities @> sqlc.narg('utilities')::text[])
  AND (sqlc.narg('verified_only')::bool IS NULL
       OR sqlc.narg('verified_only')::bool = false
       OR u.verified = true);

-- name: GetPublishedListing :one
SELECT
    sqlc.embed(l),
    u.username    AS owner_username,
    u.avatar_url  AS owner_avatar_url,
    u.verified    AS owner_verified
FROM listings l
JOIN users u ON u.id = l.owner_id
WHERE l.id = $1 AND l.status = 'published';

-- name: ListPhotosForListing :many
SELECT * FROM listing_photos
WHERE listing_id = $1
ORDER BY position, created_at;

-- Photos for a page of listings in one round-trip, rather than one query per
-- listing. Nothing populates the table yet, but the shape is what the list
-- endpoint needs the moment it does.
-- name: ListPhotosForListings :many
SELECT * FROM listing_photos
WHERE listing_id = ANY($1::uuid[])
ORDER BY listing_id, position, created_at;

-- name: CreateListing :one
INSERT INTO listings (
    owner_id, title, body, conditions,
    price_cents, deposit_cents,
    start_date, end_date, lease_months, term_tag,
    unit_type, bedrooms_total, bedroom_of, bathrooms, bath_type,
    furnished, utilities, parking, pets, laundry,
    address_line, neighbourhood, lat, lng, distance_m,
    commute_minutes, commute_mode, minutes_to_transit, minutes_to_grocery,
    status, views, replies, created_at
) VALUES (
    $1, $2, $3, $4,
    $5, $6,
    $7, $8, $9, $10,
    $11, $12, $13, $14, $15,
    $16, $17, $18, $19, $20,
    $21, $22, $23, $24, $25,
    $26, $27, $28, $29,
    $30, $31, $32, $33
)
RETURNING *;

-- name: DeleteAllListings :exec
DELETE FROM listings;

-- ---------------------------------------------------------------- write path

-- The owner's own listings, every status. The public endpoints show only
-- published rows, so this is the only way to see a paused or archived post.
-- name: ListListingsByOwner :many
SELECT
    sqlc.embed(l),
    u.username    AS owner_username,
    u.avatar_url  AS owner_avatar_url,
    u.verified    AS owner_verified
FROM listings l
JOIN users u ON u.id = l.owner_id
WHERE l.owner_id = $1
ORDER BY l.created_at DESC;

-- Fetch for an ownership check. Returns the row whatever its status, so a
-- handler can tell "not yours" from "does not exist" — and then deliberately
-- answer 404 for both.
-- name: GetListingForOwner :one
SELECT * FROM listings WHERE id = $1;

-- name: UpdateListing :one
-- Every field is optional: an edit form sends what changed, and COALESCE leaves
-- the rest alone. A PUT would make every omitted field a deletion.
UPDATE listings SET
    title          = coalesce(sqlc.narg('title'),          title),
    body           = coalesce(sqlc.narg('body'),           body),
    conditions     = coalesce(sqlc.narg('conditions'),     conditions),
    price_cents    = coalesce(sqlc.narg('price_cents'),    price_cents),
    deposit_cents  = coalesce(sqlc.narg('deposit_cents'),  deposit_cents),
    start_date     = coalesce(sqlc.narg('start_date'),     start_date),
    end_date       = coalesce(sqlc.narg('end_date'),       end_date),
    lease_months   = coalesce(sqlc.narg('lease_months'),   lease_months),
    term_tag       = coalesce(sqlc.narg('term_tag'),       term_tag),
    unit_type      = coalesce(sqlc.narg('unit_type'),      unit_type),
    bedrooms_total = coalesce(sqlc.narg('bedrooms_total'), bedrooms_total),
    bedroom_of     = coalesce(sqlc.narg('bedroom_of'),     bedroom_of),
    bathrooms      = coalesce(sqlc.narg('bathrooms'),      bathrooms),
    bath_type      = coalesce(sqlc.narg('bath_type'),      bath_type),
    furnished      = coalesce(sqlc.narg('furnished'),      furnished),
    utilities      = coalesce(sqlc.narg('utilities'),      utilities),
    parking        = coalesce(sqlc.narg('parking'),        parking),
    pets           = coalesce(sqlc.narg('pets'),           pets),
    laundry        = coalesce(sqlc.narg('laundry'),        laundry),
    address_line   = coalesce(sqlc.narg('address_line'),   address_line),
    neighbourhood  = coalesce(sqlc.narg('neighbourhood'),  neighbourhood),
    distance_m     = coalesce(sqlc.narg('distance_m'),     distance_m)
WHERE id = sqlc.arg('id')
RETURNING *;

-- published_at is set the first time a listing goes live and never moved, so
-- re-publishing after a pause does not make an old post look new.
-- name: SetListingStatus :one
UPDATE listings
-- Cast on both uses: referring to the same parameter as varchar in one place
-- and ::text in another makes PostgreSQL refuse to deduce a type for it
-- (SQLSTATE 42P08).
SET status = sqlc.arg('status')::text,
    published_at = CASE
        WHEN sqlc.arg('status')::text = 'published' AND published_at IS NULL THEN now()
        ELSE published_at
    END
WHERE id = sqlc.arg('id')
RETURNING *;
