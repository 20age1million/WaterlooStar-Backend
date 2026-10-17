-- Offers open conversations.
--
-- An owner's offer against a housing request opens a conversation with the
-- request's poster about the offered listing, or joins the one the two already
-- have about it (one conversation per listing and enquirer, from 000011). The
-- poster answers the offer by replying there; an offer has no accept or
-- decline of its own. The offer's note becomes a message from the owner.
--
-- An offer reveals nothing. Nothing here, and nothing in the offer path, sets
-- conversations.address_shared_at: the owner shares the exact address by hand
-- (000012), or not at all.

-- The conversation this offer opened or joined. Several offers can point at
-- one conversation: the same owner may offer the same listing against two
-- requests by one student. SET NULL rather than CASCADE: an offer is the
-- owner's record of having answered, and outlives the thread if that is ever
-- deleted (today only deleting the listing or an account does, and those
-- cascade to the offer as well).
ALTER TABLE request_offers
    ADD COLUMN conversation_id uuid REFERENCES conversations (id) ON DELETE SET NULL;

CREATE INDEX request_offers_conversation_idx ON request_offers (conversation_id);

-- ------------------------------------------------------------------ backfill
--
-- Every offer made before this migration that is still live gets the
-- conversation it would have opened or joined had it been made today.
--
-- "Live" is not withdrawn, and not hidden by moderation: an offer whose
-- listing or request a moderator took down, or whose owner or poster is
-- suspended, is left without a conversation (conversation_id stays NULL).
-- Opening a thread for it would put something moderation hid into a
-- student's inbox as new. An offer on a listing its owner paused or archived
-- is included: that is the owner's own state, and it does not stop a
-- conversation (Phase 15). An offer made against its owner's own request is
-- left out too: a conversation is between two people (conversations_two_people).
--
-- The backfill shares nothing: address_shared_at is left NULL on every
-- conversation it creates, and untouched on any it joins.

-- A temporary table, dropped at the end, so the filter is written once. sqlc
-- reads this file too, and the DROP is what keeps it from generating a model
-- for the table.
CREATE TEMPORARY TABLE offer_backfill AS
SELECT
    o.id         AS offer_id,
    o.listing_id,
    o.owner_id,
    r.poster_id,
    o.note,
    o.created_at
FROM request_offers o
JOIN housing_requests r ON r.id = o.request_id
JOIN listings l         ON l.id = o.listing_id
JOIN users ou           ON ou.id = o.owner_id
JOIN users pu           ON pu.id = r.poster_id
WHERE o.withdrawn_at IS NULL
  AND o.owner_id <> r.poster_id
  AND l.removed_at IS NULL
  AND r.removed_at IS NULL
  AND ou.suspended_at IS NULL
  AND pu.suspended_at IS NULL;

-- One conversation per listing and poster, opened by the owner at the time of
-- the earliest offer. A pair that already has one keeps it, open or closed.
INSERT INTO conversations (listing_id, owner_id, enquirer_id, opened_by, created_at, updated_at)
SELECT DISTINCT ON (b.listing_id, b.poster_id)
    b.listing_id, b.owner_id, b.poster_id, b.owner_id, b.created_at, b.created_at
FROM offer_backfill b
ORDER BY b.listing_id, b.poster_id, b.created_at
ON CONFLICT (listing_id, enquirer_id) DO NOTHING;

UPDATE request_offers o
SET conversation_id = c.id
FROM offer_backfill b
JOIN conversations c ON c.listing_id = b.listing_id AND c.enquirer_id = b.poster_id
WHERE o.id = b.offer_id;

-- Each non-empty note becomes a message from the owner, at the time the offer
-- was made, so the thread reads in the order things were said. Not into a
-- conversation that is closed: a student who closed a conversation about this
-- listing refused contact about it, and the backfill does not get round that.
-- The note is still on the offer itself.
--
-- The NOT EXISTS makes a second run harmless: the down migration keeps these
-- messages (see there), so down-one and up again would otherwise post every
-- note twice.
INSERT INTO messages (conversation_id, sender_id, body, created_at)
SELECT c.id, b.owner_id, btrim(b.note), b.created_at
FROM offer_backfill b
JOIN conversations c ON c.listing_id = b.listing_id AND c.enquirer_id = b.poster_id
WHERE btrim(b.note) <> ''
  AND c.closed_at IS NULL
  AND NOT EXISTS (
      SELECT 1 FROM messages m
      WHERE m.conversation_id = c.id
        AND m.sender_id = b.owner_id
        AND m.created_at = b.created_at
        AND m.body = btrim(b.note)
  )
ORDER BY b.created_at;

DROP TABLE offer_backfill;
