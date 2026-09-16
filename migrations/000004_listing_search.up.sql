-- Search and the indexes the filters need.

-- A generated column rather than a trigger: PostgreSQL keeps it in step with the
-- row automatically, so there is no trigger anyone can forget to write. Weighted
-- so a match in the title outranks one buried in the description.
ALTER TABLE listings
    ADD COLUMN search tsvector GENERATED ALWAYS AS (
        setweight(to_tsvector('english', coalesce(title, '')),         'A') ||
        setweight(to_tsvector('english', coalesce(neighbourhood, '')), 'B') ||
        setweight(to_tsvector('english', coalesce(address_line, '')),  'B') ||
        setweight(to_tsvector('english', coalesce(body, '')),          'C')
    ) STORED;

CREATE INDEX listings_search_idx ON listings USING gin (search);

-- Every sort the hub offers, and the ranges it filters on. Each is scoped to
-- published rows, which is the only set ever served.
CREATE INDEX listings_price_idx    ON listings (price_cents)  WHERE status = 'published';
CREATE INDEX listings_distance_idx ON listings (distance_m)   WHERE status = 'published';
CREATE INDEX listings_dates_idx    ON listings (start_date, end_date) WHERE status = 'published';
