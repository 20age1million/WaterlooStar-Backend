DROP TRIGGER IF EXISTS users_live_update ON users;
DROP TRIGGER IF EXISTS listings_live_update ON listings;
DROP TRIGGER IF EXISTS messages_live_insert ON messages;
DROP TRIGGER IF EXISTS conversations_live_update ON conversations;
DROP TRIGGER IF EXISTS conversations_live_insert ON conversations;

DROP FUNCTION IF EXISTS users_notify_update();
DROP FUNCTION IF EXISTS listings_notify_update();
DROP FUNCTION IF EXISTS messages_notify_insert();
DROP FUNCTION IF EXISTS conversations_notify_update();
DROP FUNCTION IF EXISTS conversations_notify_insert();
DROP FUNCTION IF EXISTS notify_conversation_event(uuid, uuid, text);
