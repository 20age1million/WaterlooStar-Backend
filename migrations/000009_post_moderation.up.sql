-- Takedown: a moderator's axis, separate from the owner's status.
--
-- status belongs to the owner — it is what the Phase 4 and Phase 6 write paths
-- change. A moderator who set status = 'archived' would be overruled the moment
-- the owner republished, and the owner's own choice would be lost. So a
-- takedown is its own column set, the public reads check both, and the owner's
-- write path refuses to touch a post while it is removed.

ALTER TABLE listings
    ADD COLUMN removed_at     timestamptz,
    ADD COLUMN removed_by     uuid REFERENCES users (id) ON DELETE SET NULL,
    ADD COLUMN removed_reason text;

ALTER TABLE listings
    ADD CONSTRAINT listings_removal_whole
    CHECK ((removed_at IS NULL) = (removed_reason IS NULL));

ALTER TABLE housing_requests
    ADD COLUMN removed_at     timestamptz,
    ADD COLUMN removed_by     uuid REFERENCES users (id) ON DELETE SET NULL,
    ADD COLUMN removed_reason text;

ALTER TABLE housing_requests
    ADD CONSTRAINT housing_requests_removal_whole
    CHECK ((removed_at IS NULL) = (removed_reason IS NULL));

-- The public reads now filter on both columns and order by recency. These
-- cover exactly that set; the existing published-only partial indexes still
-- apply, since this predicate implies theirs.
CREATE INDEX listings_public_created_idx
    ON listings (created_at DESC)
    WHERE status = 'published' AND removed_at IS NULL;

CREATE INDEX housing_requests_public_created_idx
    ON housing_requests (created_at DESC)
    WHERE status = 'published' AND removed_at IS NULL;
