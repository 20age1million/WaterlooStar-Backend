-- Offers stop pointing at conversations.
--
-- The conversations the up migration backfilled, and the note messages it
-- posted in them, deliberately stay. By the time anyone rolls this back,
-- students may have replied in those threads, and removing the conversations
-- would delete messages people wrote (messages cascade from conversations).
-- They are ordinary conversations, opened by the owner, and remain readable
-- and usable without the link from the offer.
DROP INDEX IF EXISTS request_offers_conversation_idx;

ALTER TABLE request_offers DROP COLUMN IF EXISTS conversation_id;
