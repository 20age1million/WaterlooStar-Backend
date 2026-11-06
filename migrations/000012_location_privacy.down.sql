ALTER TABLE conversations DROP COLUMN IF EXISTS address_shared_at;

-- Restore migration 4's search expression, address included.
ALTER TABLE listings DROP COLUMN IF EXISTS search;

ALTER TABLE listings
    ADD COLUMN search tsvector GENERATED ALWAYS AS (
        setweight(to_tsvector('english', coalesce(title, '')),         'A') ||
        setweight(to_tsvector('english', coalesce(neighbourhood, '')), 'B') ||
        setweight(to_tsvector('english', coalesce(address_line, '')),  'B') ||
        setweight(to_tsvector('english', coalesce(body, '')),          'C')
    ) STORED;

CREATE INDEX listings_search_idx ON listings USING gin (search);

ALTER TABLE listings DROP COLUMN IF EXISTS cross_streets;

COMMENT ON COLUMN listings.address_line IS NULL;
