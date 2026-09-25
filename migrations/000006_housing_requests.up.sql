-- "Looking for Housing" posts: what a student needs, rather than what they have.
--
-- The mirror image of listings, and deliberately the same shape — an owner
-- reading requests should be able to filter them the way a student filters
-- listings. Every fact the card shows is a column, for the same reason as
-- there: a sentence cannot be filtered.
--
-- No address of any kind. A request says "somewhere within 2 km of campus",
-- which is why these are public while carrying much less about the person than
-- a listing carries about a place.

CREATE TABLE housing_requests (
    id        uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    poster_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,

    title varchar(200) NOT NULL,
    body  text         NOT NULL DEFAULT '',

    -- The most they will pay, in cents. A listing's price_cents is what is
    -- asked; this is the ceiling it has to fall under.
    budget_cents integer NOT NULL,

    start_date   date        NOT NULL,
    end_date     date        NOT NULL,
    lease_months integer     NOT NULL,
    term_tag     varchar(40) NOT NULL,

    -- How many people the place is for, including the poster: "two roommates
    -- looking for a 2-bed" is one request, not two.
    occupants           integer NOT NULL DEFAULT 1,
    pets                boolean NOT NULL DEFAULT false,
    furnished_preferred boolean NOT NULL DEFAULT false,
    parking_needed      boolean NOT NULL DEFAULT false,
    laundry_needed      boolean NOT NULL DEFAULT false,

    -- Metres, matching listings.distance_m, so "within 2 km of campus" compares
    -- two integers rather than converting units at query time. Null means no
    -- preference at all, which is not the same as a large radius.
    max_distance_m integer,
    -- Where they would like to be, in words: "Northdale", "anywhere on a bus
    -- route". Searched, never used as a filter.
    neighbourhood  varchar(80) NOT NULL DEFAULT '',

    -- draft -> published -> paused -> archived, as listings. Only published
    -- rows are served publicly.
    status varchar(12) NOT NULL DEFAULT 'published',

    -- Stamped the first time the request goes live, including on creation.
    -- listings.published_at is only set by a later status change, so a listing
    -- created as published has none; that inconsistency is not copied here.
    published_at timestamptz,

    views  integer NOT NULL DEFAULT 0,
    offers integer NOT NULL DEFAULT 0,

    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT housing_requests_budget_positive    CHECK (budget_cents > 0),
    CONSTRAINT housing_requests_dates_ordered      CHECK (end_date > start_date),
    CONSTRAINT housing_requests_lease_positive     CHECK (lease_months > 0 AND lease_months <= 24),
    CONSTRAINT housing_requests_occupants_sane     CHECK (occupants >= 1 AND occupants <= 12),
    CONSTRAINT housing_requests_distance_positive  CHECK (max_distance_m IS NULL OR max_distance_m > 0),
    CONSTRAINT housing_requests_counters_positive  CHECK (views >= 0 AND offers >= 0),
    CONSTRAINT housing_requests_status_known       CHECK (status IN ('draft', 'published', 'paused', 'archived'))
);

-- The default order: published rows, newest first.
CREATE INDEX housing_requests_status_created_at_idx ON housing_requests (status, created_at DESC);
-- "My requests", every status, newest first.
CREATE INDEX housing_requests_poster_created_idx    ON housing_requests (poster_id, created_at DESC);

CREATE TRIGGER housing_requests_set_updated_at
    BEFORE UPDATE ON housing_requests
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Generated rather than maintained by a trigger, as on listings: PostgreSQL
-- keeps it in step with the row, so there is nothing to forget.
ALTER TABLE housing_requests
    ADD COLUMN search tsvector GENERATED ALWAYS AS (
        setweight(to_tsvector('english', coalesce(title, '')),         'A') ||
        setweight(to_tsvector('english', coalesce(neighbourhood, '')), 'B') ||
        setweight(to_tsvector('english', coalesce(body, '')),          'C')
    ) STORED;

CREATE INDEX housing_requests_search_idx ON housing_requests USING gin (search);

-- The ranges the rail filters on, scoped to the only rows ever served publicly.
CREATE INDEX housing_requests_budget_idx   ON housing_requests (budget_cents) WHERE status = 'published';
CREATE INDEX housing_requests_distance_idx ON housing_requests (max_distance_m) WHERE status = 'published';
CREATE INDEX housing_requests_dates_idx    ON housing_requests (start_date, end_date) WHERE status = 'published';
