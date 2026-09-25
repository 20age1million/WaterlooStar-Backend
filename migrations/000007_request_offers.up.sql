-- An owner answering a request with one of their own listings.
--
-- The offer points at a listing rather than repeating it, so the student sees a
-- real place with real rent and dates, and an offer cannot outlive the place
-- behind it: take the listing down and the offer stops being shown, with no
-- second thing to remember.

CREATE TABLE request_offers (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    request_id uuid NOT NULL REFERENCES housing_requests (id) ON DELETE CASCADE,
    listing_id uuid NOT NULL REFERENCES listings (id)         ON DELETE CASCADE,
    -- Denormalised from the listing so "my offers" and the ownership check need
    -- no join. It cannot drift: a listing never changes hands.
    owner_id   uuid NOT NULL REFERENCES users (id)            ON DELETE CASCADE,

    -- "This is two doors down from the library, if that helps." Optional: the
    -- listing says everything factual already.
    note text NOT NULL DEFAULT '',

    -- Withdrawn rather than deleted, so the student does not see an offer
    -- vanish without trace mid-conversation.
    withdrawn_at timestamptz,

    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    -- Offering the same place twice is noise, not emphasis.
    CONSTRAINT request_offers_one_per_listing UNIQUE (request_id, listing_id),
    CONSTRAINT request_offers_note_length CHECK (length(note) <= 1000)
);

-- Reading the offers on a request, newest first, is the only listing query.
CREATE INDEX request_offers_request_created_idx ON request_offers (request_id, created_at DESC);
-- An owner's own offers.
CREATE INDEX request_offers_owner_idx ON request_offers (owner_id);

CREATE TRIGGER request_offers_set_updated_at
    BEFORE UPDATE ON request_offers
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- The counter added in 000006 is dropped rather than maintained.
--
-- It could only ever be right by accident: an offer stops counting when its
-- listing is taken down, which happens in a table this counter knows nothing
-- about, so any stored number would drift away from what the student can
-- actually see. The read queries count the visible offers instead. A column
-- that lies is worse than a join — see the published_at trap recorded in
-- Phase 5.
ALTER TABLE housing_requests DROP COLUMN offers;

-- Dropping the column takes its CHECK with it, and that CHECK also guarded
-- views. Put that half back.
ALTER TABLE housing_requests
    ADD CONSTRAINT housing_requests_views_positive CHECK (views >= 0);
