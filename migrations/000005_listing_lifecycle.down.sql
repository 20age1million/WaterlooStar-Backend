DROP INDEX IF EXISTS listings_owner_created_idx;
ALTER TABLE listings DROP COLUMN IF EXISTS published_at;
