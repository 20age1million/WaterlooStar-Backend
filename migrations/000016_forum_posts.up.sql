-- Public title/body discussions. Existing account and housing rows are untouched.
CREATE TABLE forum_posts (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    author_id  uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    title      varchar(200) NOT NULL,
    body       text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz,

    CONSTRAINT forum_posts_title_length CHECK (
        char_length(btrim(title)) BETWEEN 1 AND 200
        AND title ~ '[^[:space:]]'
    ),
    CONSTRAINT forum_posts_body_length CHECK (
        char_length(btrim(body)) BETWEEN 1 AND 8000
        AND body ~ '[^[:space:]]'
    ),

    -- The same generated full-text convention used by housing. Only public
    -- post text is searched, never account information.
    search tsvector GENERATED ALWAYS AS (
        setweight(to_tsvector('english', title), 'A') ||
        setweight(to_tsvector('english', body), 'B')
    ) STORED
);

CREATE INDEX forum_posts_created_idx
    ON forum_posts (created_at DESC, id DESC) WHERE deleted_at IS NULL;
CREATE INDEX forum_posts_author_idx ON forum_posts (author_id);
CREATE INDEX forum_posts_search_idx
    ON forum_posts USING gin (search) WHERE deleted_at IS NULL;

CREATE TRIGGER forum_posts_set_updated_at
    BEFORE UPDATE ON forum_posts
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
