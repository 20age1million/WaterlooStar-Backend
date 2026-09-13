-- Accounts, and the three kinds of token an account's lifecycle needs.
--
-- Every token table stores a SHA-256 hash of the token, never the token itself.
-- A stolen database dump therefore yields nothing usable: the raw value exists
-- only in the email that carried it, or in the holder's cookie.

CREATE TABLE users (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email         varchar(255) NOT NULL,
    username      varchar(32)  NOT NULL,
    password_hash text         NOT NULL,
    role          varchar(20)  NOT NULL DEFAULT 'user',
    verified      boolean      NOT NULL DEFAULT false,
    avatar_url    text,
    level         integer      NOT NULL DEFAULT 1,
    star_points   integer      NOT NULL DEFAULT 0,
    created_at    timestamptz  NOT NULL DEFAULT now(),
    updated_at    timestamptz  NOT NULL DEFAULT now(),

    CONSTRAINT users_level_positive       CHECK (level >= 1),
    CONSTRAINT users_star_points_positive CHECK (star_points >= 0),
    CONSTRAINT users_role_known           CHECK (role IN ('user', 'moderator', 'admin')),
    -- The product is gated on being a Waterloo student, so the rule lives in the
    -- schema too rather than only in the handler that happens to insert today.
    CONSTRAINT users_email_is_uwaterloo   CHECK (lower(email) LIKE '%@uwaterloo.ca')
);

-- Addresses are compared case-insensitively; a unique index on lower(email) gives
-- that without depending on the citext extension.
CREATE UNIQUE INDEX users_email_lower_key    ON users (lower(email));
CREATE UNIQUE INDEX users_username_lower_key ON users (lower(username));

CREATE TRIGGER users_set_updated_at
    BEFORE UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Proves control of the address at registration. One-shot: consumed_at is set
-- when redeemed, so a token cannot be replayed.
CREATE TABLE email_verification_tokens (
    token_hash  bytea PRIMARY KEY,
    user_id     uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    expires_at  timestamptz NOT NULL,
    consumed_at timestamptz,
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX email_verification_tokens_user_id_idx ON email_verification_tokens (user_id);

CREATE TABLE password_reset_tokens (
    token_hash  bytea PRIMARY KEY,
    user_id     uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    expires_at  timestamptz NOT NULL,
    consumed_at timestamptz,
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX password_reset_tokens_user_id_idx ON password_reset_tokens (user_id);

-- Long-lived opaque tokens backing the short-lived access JWT. Rotated on every
-- refresh: the presented token is revoked as the replacement is issued, so a
-- stolen refresh token stops working the moment the real user refreshes.
CREATE TABLE refresh_tokens (
    token_hash bytea PRIMARY KEY,
    user_id    uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz,
    user_agent text,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX refresh_tokens_user_id_idx ON refresh_tokens (user_id);
