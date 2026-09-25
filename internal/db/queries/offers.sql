-- An owner's answer to a request: one of their own listings, with an optional
-- note.
--
-- Visibility is not symmetric. The student who posted the request sees every
-- live offer on it; an owner sees only their own. Owners reading each other's
-- answers would turn a request into an auction.

-- name: CreateOffer :one
INSERT INTO request_offers (request_id, listing_id, owner_id, note)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- The offers on a request, as the student sees them: the listing comes with it,
-- so the reply shows real rent and dates rather than a pitch.
--
-- An offer is live only while its listing is: taking a listing down withdraws
-- its offers, with nothing else to remember. Same for an explicit withdrawal.
-- name: ListOffersForRequest :many
SELECT
    sqlc.embed(o),
    sqlc.embed(l),
    u.username   AS owner_username,
    u.avatar_url AS owner_avatar_url,
    u.verified   AS owner_verified
FROM request_offers o
JOIN listings l ON l.id = o.listing_id
JOIN users u    ON u.id = o.owner_id
WHERE o.request_id = sqlc.arg('request_id')
  AND o.withdrawn_at IS NULL
  AND l.status = 'published'
ORDER BY o.created_at DESC;

-- The same count the read queries compute inline, for the write path's
-- responses. Visible offers only, on the same two conditions.
-- name: CountOffersForRequest :one
SELECT count(*)
FROM request_offers o
JOIN listings l ON l.id = o.listing_id
WHERE o.request_id = sqlc.arg('request_id')
  AND o.withdrawn_at IS NULL
  AND l.status = 'published';

-- Any offer by id, for the ownership check before a withdrawal. "Not yours" and
-- "does not exist" answer alike in the handler.
-- name: GetOffer :one
SELECT * FROM request_offers WHERE id = sqlc.arg('id');

-- Withdrawn rather than deleted: the row stays, so the same listing cannot be
-- re-offered against the same request by working around the unique constraint.
-- name: WithdrawOffer :one
UPDATE request_offers
SET withdrawn_at = now()
WHERE id = sqlc.arg('id') AND withdrawn_at IS NULL
RETURNING *;

-- An owner's own offers, whatever became of them.
-- name: ListOffersByOwner :many
SELECT
    sqlc.embed(o),
    sqlc.embed(l)
FROM request_offers o
JOIN listings l ON l.id = o.listing_id
WHERE o.owner_id = sqlc.arg('owner_id')
ORDER BY o.created_at DESC;

-- Whether this owner has already offered this listing here, withdrawn or not.
-- The unique constraint is the real guarantee; this is so the handler can say
-- so in words rather than returning a constraint violation.
-- name: GetOfferForListing :one
SELECT * FROM request_offers
WHERE request_id = sqlc.arg('request_id') AND listing_id = sqlc.arg('listing_id');
