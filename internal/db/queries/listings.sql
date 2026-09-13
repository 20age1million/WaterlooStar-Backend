-- Only published rows are ever served. Filtering here rather than in Go means a
-- handler cannot forget and expose somebody's draft.

-- name: ListPublishedListings :many
SELECT
    sqlc.embed(l),
    u.username    AS owner_username,
    u.avatar_url  AS owner_avatar_url,
    u.verified    AS owner_verified
FROM listings l
JOIN users u ON u.id = l.owner_id
WHERE l.status = 'published'
ORDER BY l.created_at DESC
LIMIT $1 OFFSET $2;

-- name: CountPublishedListings :one
SELECT count(*) FROM listings WHERE status = 'published';

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
