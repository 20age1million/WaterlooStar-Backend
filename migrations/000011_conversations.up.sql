-- Conversations between two students about one listing, and their messages.
--
-- A conversation has two participants: the listing's owner, and the student
-- asking about it (the enquirer). Either side may write from the first message
-- on; there is no accept step, because accepting would gate nothing. Opening a
-- conversation reveals nothing about either side - no email, no phone and no
-- exact address. The address is the owner's to share by hand, inside the
-- conversation, which a later migration adds.
--
-- Two things are computed rather than stored, for the reason recorded when the
-- housing_requests.offers counter was dropped (000007): a stored number drifts
-- from what it summarises.
--
--   * Unread counts come from messages and each side's read marker.
--   * "Last activity" is the newest message, or created_at when there is none.
--
-- The index on messages (conversation_id, created_at DESC) is what keeps both
-- cheap.

CREATE TABLE conversations (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    listing_id uuid NOT NULL REFERENCES listings (id) ON DELETE CASCADE,
    -- Denormalised from the listing, as on request_offers, so the participant
    -- check needs no join. It cannot drift: a listing never changes hands.
    owner_id    uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    enquirer_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    -- Which side opened it. Today always the enquirer, from the listing page;
    -- an owner's offer opens one from the other side.
    opened_by uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,

    -- Status is derived, not stored: closed_at IS NULL is open. A status
    -- column beside these could disagree with them.
    closed_at timestamptz,
    closed_by uuid REFERENCES users (id) ON DELETE CASCADE,

    -- Each side's read marker. A message from the other side newer than yours
    -- is unread. Null until the side first marks the conversation read.
    owner_read_at    timestamptz,
    enquirer_read_at timestamptz,

    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    -- One conversation per place per student. Closing therefore also stops
    -- that student opening another about the same listing.
    CONSTRAINT conversations_one_per_listing_enquirer UNIQUE (listing_id, enquirer_id),
    CONSTRAINT conversations_two_people CHECK (owner_id <> enquirer_id),
    CONSTRAINT conversations_opened_by_participant CHECK (opened_by IN (owner_id, enquirer_id)),
    CONSTRAINT conversations_closure_whole CHECK ((closed_at IS NULL) = (closed_by IS NULL)),
    CONSTRAINT conversations_closed_by_participant
        CHECK (closed_by IS NULL OR closed_by IN (owner_id, enquirer_id))
);

-- The inbox reads both sides.
CREATE INDEX conversations_owner_idx    ON conversations (owner_id);
CREATE INDEX conversations_enquirer_idx ON conversations (enquirer_id);

CREATE TRIGGER conversations_set_updated_at
    BEFORE UPDATE ON conversations
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Append-only: no updated_at, and no UPDATE or DELETE against this table in
-- internal/db/queries. A message cannot be edited or deleted, which keeps what
-- a conversation shows trustworthy if someone reports it.
CREATE TABLE messages (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    conversation_id uuid NOT NULL REFERENCES conversations (id) ON DELETE CASCADE,
    sender_id       uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    -- Stored trimmed by the handler; this is the backstop.
    body            text NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT messages_body_length CHECK (length(btrim(body)) BETWEEN 1 AND 4000)
);

-- A conversation's messages newest first, its last message, and its unread
-- count all read this.
CREATE INDEX messages_conversation_created_idx ON messages (conversation_id, created_at DESC);
