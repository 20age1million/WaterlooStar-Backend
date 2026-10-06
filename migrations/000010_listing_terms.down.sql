-- Only what 000010 added. Each CHECK is dropped by name before its columns, so
-- nothing older is touched.
ALTER TABLE listings
    DROP CONSTRAINT IF EXISTS listings_bills_estimate_range,
    DROP CONSTRAINT IF EXISTS listings_min_stay_within_lease,
    DROP CONSTRAINT IF EXISTS listings_min_stay_with_shorter_stays;

ALTER TABLE listings
    DROP COLUMN IF EXISTS bills_estimate_cents,
    DROP COLUMN IF EXISTS min_stay_months,
    DROP COLUMN IF EXISTS shorter_stays;
