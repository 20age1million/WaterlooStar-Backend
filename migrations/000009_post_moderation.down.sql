-- Only what 000009 added. Dropping a column drops any constraint that names it,
-- so each CHECK is dropped by name first and nothing older is touched.
DROP INDEX IF EXISTS housing_requests_public_created_idx;
DROP INDEX IF EXISTS listings_public_created_idx;

ALTER TABLE housing_requests DROP CONSTRAINT IF EXISTS housing_requests_removal_whole;
ALTER TABLE housing_requests
    DROP COLUMN IF EXISTS removed_reason,
    DROP COLUMN IF EXISTS removed_by,
    DROP COLUMN IF EXISTS removed_at;

ALTER TABLE listings DROP CONSTRAINT IF EXISTS listings_removal_whole;
ALTER TABLE listings
    DROP COLUMN IF EXISTS removed_reason,
    DROP COLUMN IF EXISTS removed_by,
    DROP COLUMN IF EXISTS removed_at;
