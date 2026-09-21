-- "Housing Available" posts.
--
-- Every fact the three screens display is a column, not a sentence. That is the
-- product's stated difference from a classifieds board — "rent, dates, distance,
-- bedrooms and utilities are structured fields" — and the filter rail in the
-- next feature can only work if it is literally true here.
--
-- The internal word for this is *renter*; it appears nowhere in the API.

CREATE TABLE listings (
    id       uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,

    title      varchar(200) NOT NULL,
    body       text         NOT NULL DEFAULT '',
    -- The poster's own terms, shown as a list on the detail page.
    conditions text[]       NOT NULL DEFAULT '{}',

    -- Money in cents. Storing dollars as a float would eventually produce rent
    -- of $844.99999.
    price_cents   integer NOT NULL,
    deposit_cents integer,

    -- The co-op calendar is the whole premise, so dates are dates.
    start_date   date        NOT NULL,
    end_date     date        NOT NULL,
    lease_months integer     NOT NULL,
    -- Display label for the term ("Winter term", "8 months"). Derived from the
    -- dates today, but kept because a poster may word it their own way.
    term_tag     varchar(40) NOT NULL,

    -- room: one bedroom in a shared house. studio: no separate bedroom.
    -- unit: a whole place.
    unit_type      varchar(10) NOT NULL DEFAULT 'room',
    bedrooms_total integer     NOT NULL DEFAULT 1,
    -- "1 of 4 bed" — which one of bedrooms_total is on offer. Null for a whole unit.
    bedroom_of     integer,
    bathrooms      numeric(3, 1) NOT NULL DEFAULT 1,
    -- shared | private | ensuite
    bath_type      varchar(10) NOT NULL DEFAULT 'shared',

    furnished boolean NOT NULL DEFAULT false,
    -- Which bills are included: internet, hydro, water, heat, gas.
    utilities text[]  NOT NULL DEFAULT '{}',
    parking   boolean NOT NULL DEFAULT false,
    pets      boolean NOT NULL DEFAULT false,
    laundry   boolean NOT NULL DEFAULT false,

    -- Address is PUBLIC for now. The design implies an exact address revealed
    -- only after both sides agree to talk; that decision is deferred, and this
    -- column is deliberately separate from `neighbourhood` so a future private
    -- field can be added without moving data.
    address_line  varchar(200) NOT NULL,
    neighbourhood varchar(80)  NOT NULL DEFAULT '',
    lat           numeric(9, 6),
    lng           numeric(9, 6),
    -- Straight-line metres to campus. Metres, not km, so the radius filter in
    -- the next feature compares integers.
    distance_m    integer,

    -- Travel times shown on the card and the detail page. Placeholders until a
    -- routing provider is chosen; a listing may have none.
    commute_minutes    integer,
    -- walk | bus
    commute_mode       varchar(10) NOT NULL DEFAULT 'walk',
    minutes_to_transit integer,
    minutes_to_grocery integer,

    -- draft -> published -> paused -> archived. Only published rows are served.
    status varchar(12) NOT NULL DEFAULT 'published',

    -- Denormalised counters. Maintained by the features that own the events:
    -- views and saves in Phase 6, replies by the question thread.
    views   integer NOT NULL DEFAULT 0,
    replies integer NOT NULL DEFAULT 0,
    saves   integer NOT NULL DEFAULT 0,

    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT listings_price_positive     CHECK (price_cents > 0),
    CONSTRAINT listings_deposit_positive   CHECK (deposit_cents IS NULL OR deposit_cents >= 0),
    CONSTRAINT listings_dates_ordered      CHECK (end_date > start_date),
    CONSTRAINT listings_lease_positive     CHECK (lease_months > 0),
    CONSTRAINT listings_counters_positive  CHECK (views >= 0 AND replies >= 0 AND saves >= 0),
    CONSTRAINT listings_bedrooms_positive  CHECK (bedrooms_total >= 0),
    CONSTRAINT listings_bedroom_of_valid   CHECK (bedroom_of IS NULL OR (bedroom_of >= 1 AND bedroom_of <= bedrooms_total)),
    CONSTRAINT listings_status_known       CHECK (status IN ('draft', 'published', 'paused', 'archived')),
    CONSTRAINT listings_unit_type_known    CHECK (unit_type IN ('room', 'studio', 'unit')),
    CONSTRAINT listings_bath_type_known    CHECK (bath_type IN ('shared', 'private', 'ensuite')),
    CONSTRAINT listings_commute_mode_known CHECK (commute_mode IN ('walk', 'bus'))
);

-- The default listing order: published rows, newest first.
CREATE INDEX listings_status_created_at_idx ON listings (status, created_at DESC);
CREATE INDEX listings_owner_id_idx          ON listings (owner_id);

CREATE TRIGGER listings_set_updated_at
    BEFORE UPDATE ON listings
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Created now so the read path can return an empty array rather than omitting
-- the field. Nothing populates it until photo upload arrives.
CREATE TABLE listing_photos (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    listing_id uuid    NOT NULL REFERENCES listings (id) ON DELETE CASCADE,
    url        text    NOT NULL,
    position   integer NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX listing_photos_listing_id_position_idx ON listing_photos (listing_id, position);
