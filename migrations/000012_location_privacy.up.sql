-- Location privacy: the exact address leaves public view, and the owner can
-- share it in one conversation.
--
-- address_line is now the PRIVATE exact address, house number included.
-- Migration 3's comment calls it "PUBLIC for now" and defers the decision;
-- that comment cannot be edited, so this one replaces it. The decision was
-- made on 2026-10-14 and 2026-10-16: a listing shows its approximate location
-- (neighbourhood and cross_streets) publicly, and its exact address and
-- coordinates reach only its owner, an admin, and a student the owner chose
-- to share them with in their conversation. No row's data moves: the column
-- keeps its name and its values, and only who may read it changes.
COMMENT ON COLUMN listings.address_line IS
    'Private exact address, house number included. Never on a public read; '
    'shown to the owner, admins, and a conversation whose owner shared it '
    '(conversations.address_shared_at). Not in the search index.';

-- The public approximate location, beside neighbourhood. Empty until the owner
-- gives one; there is nothing to backfill it from.
ALTER TABLE listings ADD COLUMN cross_streets varchar(80) NOT NULL DEFAULT '';

-- The search column is rebuilt without address_line, and with cross_streets.
-- A field that is hidden but still searchable can be confirmed by probing:
-- search for "123 Hazel St" and a hit tells you the address without ever
-- showing it. Dropping the column drops listings_search_idx with it, so the
-- index is created again below.
ALTER TABLE listings DROP COLUMN search;

ALTER TABLE listings
    ADD COLUMN search tsvector GENERATED ALWAYS AS (
        setweight(to_tsvector('english', coalesce(title, '')),         'A') ||
        setweight(to_tsvector('english', coalesce(neighbourhood, '')), 'B') ||
        setweight(to_tsvector('english', coalesce(cross_streets, '')), 'B') ||
        setweight(to_tsvector('english', coalesce(body, '')),          'C')
    ) STORED;

CREATE INDEX listings_search_idx ON listings USING gin (search);

-- When the listing's owner shared its exact address in this conversation.
-- Nullable; set once, by the owner's explicit share
-- (POST /conversations/{id}/share-address, the ShareConversationAddress
-- query), and never cleared: there is no un-share. Nothing else may set it -
-- not starting a conversation, not a message, not closing, not an offer.
-- Moderation withholds the address in the read queries without touching this
-- column, so reinstating restores it.
ALTER TABLE conversations ADD COLUMN address_shared_at timestamptz;
