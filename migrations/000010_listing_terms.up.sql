-- Listing terms: whether a student may rent part of the listed dates, and the
-- owner's estimate of the bills the rent does not cover.
--
-- Existing listings take the defaults — whole lease only, estimate unknown —
-- with no backfill: nobody has said otherwise, and guessing on an owner's behalf
-- is exactly what this feature exists not to do.

ALTER TABLE listings
    ADD COLUMN shorter_stays        boolean NOT NULL DEFAULT false,
    -- The shortest stay an owner accepts when shorter_stays is on.
    ADD COLUMN min_stay_months      integer,
    -- The tenant's monthly share of every bill the rent does not include, as
    -- the owner estimates it. 0 means nothing extra; NULL means not stated, and
    -- is never read as 0.
    ADD COLUMN bills_estimate_cents integer;

ALTER TABLE listings
    ADD CONSTRAINT listings_min_stay_with_shorter_stays
        CHECK ((min_stay_months IS NULL) = (NOT shorter_stays)),
    ADD CONSTRAINT listings_min_stay_within_lease
        CHECK (min_stay_months IS NULL OR (min_stay_months >= 1 AND min_stay_months <= lease_months)),
    ADD CONSTRAINT listings_bills_estimate_range
        CHECK (bills_estimate_cents IS NULL OR (bills_estimate_cents >= 0 AND bills_estimate_cents <= 100000));
