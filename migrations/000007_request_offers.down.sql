DROP TABLE IF EXISTS request_offers;

-- Restored as 000006 left it: the column, and the CHECK guarding both counters.
ALTER TABLE housing_requests DROP CONSTRAINT IF EXISTS housing_requests_views_positive;

ALTER TABLE housing_requests
    ADD COLUMN IF NOT EXISTS offers integer NOT NULL DEFAULT 0;

ALTER TABLE housing_requests
    ADD CONSTRAINT housing_requests_counters_positive CHECK (views >= 0 AND offers >= 0);
