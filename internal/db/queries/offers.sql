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
--
-- Each offer also carries the conversation it opened or joined (Phase 17):
-- whether it is closed, and whether the request's poster has written in it,
-- which is how an offer is answered. conversation_closed_at is null both for
-- an open conversation and for none; o.conversation_id tells them apart.
-- name: ListOffersForRequest :many
SELECT
    sqlc.embed(o),
    sqlc.embed(l),
    u.username   AS owner_username,
    u.avatar_url AS owner_avatar_url,
    u.verified   AS owner_verified,
    c.closed_at  AS conversation_closed_at,
    EXISTS (
        SELECT 1 FROM messages m
        WHERE m.conversation_id = o.conversation_id
          AND m.sender_id = r.poster_id
    ) AS poster_replied
FROM request_offers o
JOIN housing_requests r ON r.id = o.request_id
JOIN listings l ON l.id = o.listing_id
JOIN users u    ON u.id = o.owner_id
LEFT JOIN conversations c ON c.id = o.conversation_id
WHERE o.request_id = sqlc.arg('request_id')
  AND o.withdrawn_at IS NULL
  AND l.status = 'published'
  AND l.removed_at IS NULL
  -- An offer from a suspended owner is hidden like one whose listing is down.
  AND u.suspended_at IS NULL
ORDER BY o.created_at DESC;

-- The same count the read queries compute inline, for the write path's
-- responses. Visible offers only, on the same two conditions.
-- name: CountOffersForRequest :one
SELECT count(*)
FROM request_offers o
JOIN listings l ON l.id = o.listing_id
JOIN users u    ON u.id = o.owner_id
WHERE o.request_id = sqlc.arg('request_id')
  AND o.withdrawn_at IS NULL
  AND l.status = 'published'
  AND l.removed_at IS NULL
  AND u.suspended_at IS NULL;

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

-- Points an offer at the conversation it opened or joined. Written in the same
-- transaction as the offer, by CreateOffer's handler; nothing else sets it.
-- name: SetOfferConversation :one
UPDATE request_offers
SET conversation_id = sqlc.arg('conversation_id')::uuid
WHERE id = sqlc.arg('id')::uuid
RETURNING *;
