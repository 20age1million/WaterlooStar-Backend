DROP INDEX IF EXISTS listings_dates_idx;
DROP INDEX IF EXISTS listings_distance_idx;
DROP INDEX IF EXISTS listings_price_idx;
DROP INDEX IF EXISTS listings_search_idx;
ALTER TABLE listings DROP COLUMN IF EXISTS search;
