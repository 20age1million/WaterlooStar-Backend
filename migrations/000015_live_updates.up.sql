-- Live updates (Phase 19).
--
-- Every change a participant would want to see in an open conversation sends a
-- signal on the `conversation_events` channel. Each API process LISTENs on it
-- and forwards the signal to that student's open streams (GET /me/stream).
--
-- Triggers rather than NOTIFY calls in the handlers, so every writer is covered:
-- the API, cmd/admin's moderation, cmd/seed, and anything added later. NOTIFY
-- is delivered only when the transaction commits, so a rolled-back change
-- signals nothing, and identical signals within one transaction are delivered
-- once.
--
-- A signal names one recipient, one conversation and the kind of change, and
-- nothing else: never a message's text, an address or the other student's id.
-- The client re-reads through the ordinary endpoints, which apply every
-- visibility rule.

CREATE FUNCTION notify_conversation_event(recipient uuid, conversation uuid, change text)
RETURNS void AS $$
BEGIN
    PERFORM pg_notify('conversation_events',
        json_build_object('u', recipient, 'c', conversation, 'k', change)::text);
END;
$$ LANGUAGE plpgsql;

-- A conversation opened, from a listing or by an offer: both sides.
CREATE FUNCTION conversations_notify_insert() RETURNS trigger AS $$
BEGIN
    PERFORM notify_conversation_event(NEW.owner_id, NEW.id, 'opened');
    PERFORM notify_conversation_event(NEW.enquirer_id, NEW.id, 'opened');
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER conversations_live_insert
    AFTER INSERT ON conversations
    FOR EACH ROW EXECUTE FUNCTION conversations_notify_insert();

-- Closed, address shared, or read. A read goes only to the side that read:
-- the other side is never shown it.
CREATE FUNCTION conversations_notify_update() RETURNS trigger AS $$
BEGIN
    IF OLD.closed_at IS NULL AND NEW.closed_at IS NOT NULL THEN
        PERFORM notify_conversation_event(NEW.owner_id, NEW.id, 'closed');
        PERFORM notify_conversation_event(NEW.enquirer_id, NEW.id, 'closed');
    END IF;
    IF OLD.address_shared_at IS NULL AND NEW.address_shared_at IS NOT NULL THEN
        PERFORM notify_conversation_event(NEW.owner_id, NEW.id, 'address_shared');
        PERFORM notify_conversation_event(NEW.enquirer_id, NEW.id, 'address_shared');
    END IF;
    IF OLD.owner_read_at IS DISTINCT FROM NEW.owner_read_at THEN
        PERFORM notify_conversation_event(NEW.owner_id, NEW.id, 'read');
    END IF;
    IF OLD.enquirer_read_at IS DISTINCT FROM NEW.enquirer_read_at THEN
        PERFORM notify_conversation_event(NEW.enquirer_id, NEW.id, 'read');
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER conversations_live_update
    AFTER UPDATE ON conversations
    FOR EACH ROW EXECUTE FUNCTION conversations_notify_update();

-- A message: both sides, so the sender's other tabs show it too.
CREATE FUNCTION messages_notify_insert() RETURNS trigger AS $$
DECLARE
    c conversations%ROWTYPE;
BEGIN
    SELECT * INTO c FROM conversations WHERE id = NEW.conversation_id;
    PERFORM notify_conversation_event(c.owner_id, c.id, 'message');
    PERFORM notify_conversation_event(c.enquirer_id, c.id, 'message');
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER messages_live_insert
    AFTER INSERT ON messages
    FOR EACH ROW EXECUTE FUNCTION messages_notify_insert();

-- The listing taken down or restored, paused, archived or republished, or its
-- address corrected: whether the conversation can be written in, and what a
-- shared `place` shows, may have changed. Both sides of every conversation
-- about it.
CREATE FUNCTION listings_notify_update() RETURNS trigger AS $$
DECLARE
    c record;
BEGIN
    IF OLD.removed_at IS DISTINCT FROM NEW.removed_at
       OR OLD.status IS DISTINCT FROM NEW.status
       OR OLD.address_line IS DISTINCT FROM NEW.address_line THEN
        FOR c IN SELECT id, owner_id, enquirer_id FROM conversations WHERE listing_id = NEW.id LOOP
            PERFORM notify_conversation_event(c.owner_id, c.id, 'availability');
            PERFORM notify_conversation_event(c.enquirer_id, c.id, 'availability');
        END LOOP;
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER listings_live_update
    AFTER UPDATE ON listings
    FOR EACH ROW EXECUTE FUNCTION listings_notify_update();

-- An account suspended or reinstated: the other side of each of its
-- conversations. The account's own sessions end with the suspension.
CREATE FUNCTION users_notify_update() RETURNS trigger AS $$
DECLARE
    c record;
BEGIN
    IF OLD.suspended_at IS DISTINCT FROM NEW.suspended_at THEN
        FOR c IN
            SELECT id, CASE WHEN owner_id = NEW.id THEN enquirer_id ELSE owner_id END AS counterpart
            FROM conversations
            WHERE owner_id = NEW.id OR enquirer_id = NEW.id
        LOOP
            PERFORM notify_conversation_event(c.counterpart, c.id, 'availability');
        END LOOP;
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER users_live_update
    AFTER UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION users_notify_update();
