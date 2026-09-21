-- When a listing first became visible. Distinct from created_at, which is when
-- it was drafted: a post written on Monday and published on Friday is four days
-- old in the feed, not one.
ALTER TABLE listings ADD COLUMN published_at timestamptz;

-- Everything already published counts as published when it was created.
UPDATE listings SET published_at = created_at WHERE status = 'published';

-- The owner's own listings, every status, newest first. Not partial on status:
-- this is exactly the query that must return drafts and archived posts.
CREATE INDEX listings_owner_created_idx ON listings (owner_id, created_at DESC);
