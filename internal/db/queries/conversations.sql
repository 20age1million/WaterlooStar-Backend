-- Conversations between a listing's owner and a student asking about it, and
-- their messages.
--
-- Two rules shape this file:
--
--   * Only the two participants can see a conversation. Every read by id takes
--     the asking user as well and returns no row for anyone else, so "not
--     yours" and "does not exist" are the same answer, decided here rather
--     than in a handler that might forget.
--   * These are the participants' own reads, like /me/..., so they do not hide
--     a conversation whose listing was taken down or whose other participant
--     is suspended. They report it instead (listing_removed, listing_available,
--     counterpart_suspended) and the handler makes the conversation read-only.
--   * The exact address appears only in the place_* columns, only once the
--     owner has shared it (address_shared_at), and is withheld while the
--     listing is taken down or its owner suspended.
--
-- Unread counts and last activity are computed here from messages and the read
-- markers, never stored.

-- name: CreateConversation :one
INSERT INTO conversations (listing_id, owner_id, enquirer_id, opened_by)
VALUES (
    sqlc.arg('listing_id'), sqlc.arg('owner_id'), sqlc.arg('enquirer_id'), sqlc.arg('opened_by')
)
RETURNING *;

-- name: CreateMessage :one
INSERT INTO messages (conversation_id, sender_id, body)
VALUES (sqlc.arg('conversation_id'), sqlc.arg('sender_id'), sqlc.arg('body'))
RETURNING *;

-- The existing conversation between this student and this listing, open or
-- closed, so starting a second one can answer 409 and name it. The unique
-- constraint is the real guarantee.
-- name: GetConversationByListingAndEnquirer :one
SELECT * FROM conversations
WHERE listing_id = sqlc.arg('listing_id') AND enquirer_id = sqlc.arg('enquirer_id');

-- One conversation, as one of its participants sees it. No row unless user_id
-- is the owner or the enquirer.
-- name: GetConversationForParticipant :one
SELECT
    sqlc.embed(c),
    l.title         AS listing_title,
    l.neighbourhood AS listing_neighbourhood,
    l.price_cents   AS listing_price_cents,
    l.start_date    AS listing_start_date,
    l.end_date      AS listing_end_date,
    -- What the public sees, by the same three filters the public reads use.
    (l.status = 'published' AND l.removed_at IS NULL AND o.suspended_at IS NULL)::bool AS listing_available,
    (l.removed_at IS NOT NULL)::bool AS listing_removed,
    cp.id         AS counterpart_id,
    cp.username   AS counterpart_username,
    cp.avatar_url AS counterpart_avatar_url,
    cp.verified   AS counterpart_verified,
    (cp.suspended_at IS NOT NULL)::bool AS counterpart_suspended,
    -- The exact address: null unless the reveal join below matched.
    pl.address_line AS place_address_line,
    pl.lat          AS place_lat,
    pl.lng          AS place_lng,
    lm.sender_id  AS last_message_sender_id,
    lm.body       AS last_message_body,
    lm.created_at AS last_message_created_at,
    (SELECT count(*) FROM messages m
     WHERE m.conversation_id = c.id
       AND m.sender_id <> sqlc.arg('user_id')::uuid
       AND m.created_at > coalesce(
             CASE WHEN c.owner_id = sqlc.arg('user_id')::uuid THEN c.owner_read_at ELSE c.enquirer_read_at END,
             '-infinity'::timestamptz)
    ) AS unread_count
FROM conversations c
JOIN listings l ON l.id = c.listing_id
JOIN users o    ON o.id = c.owner_id
JOIN users cp   ON cp.id = CASE WHEN c.owner_id = sqlc.arg('user_id')::uuid THEN c.enquirer_id ELSE c.owner_id END
-- The reveal condition. The listing again, matched only once the owner has
-- shared its address in this conversation, and not while the listing is taken
-- down or its owner suspended: withheld, never un-shared, so reinstating
-- restores it. It is in the SQL, as the public read filters are, so no handler
-- can forget it. A join rather than a CASE because sqlc types a CASE column as
-- interface{}, and a left-joined table's columns as pointers.
LEFT JOIN listings pl ON pl.id = c.listing_id
    AND c.address_shared_at IS NOT NULL
    AND l.removed_at IS NULL
    AND o.suspended_at IS NULL
-- The newest message. Joined as the messages table itself, rather than as a
-- lateral subquery, so sqlc knows its columns are null for a conversation with
-- no messages yet.
LEFT JOIN messages lm ON lm.id = (
    SELECT m.id FROM messages m
    WHERE m.conversation_id = c.id
    ORDER BY m.created_at DESC, m.id DESC
    LIMIT 1
)
WHERE c.id = sqlc.arg('id')::uuid
  AND sqlc.arg('user_id')::uuid IN (c.owner_id, c.enquirer_id);

-- The inbox: both sides, newest activity first. status is 'open' or 'closed';
-- side is 'owner' (about your own listings) or 'enquirer' (ones you asked
-- about). Either may be null for no filter.
-- name: ListConversationsForUser :many
SELECT
    sqlc.embed(c),
    l.title         AS listing_title,
    l.neighbourhood AS listing_neighbourhood,
    l.price_cents   AS listing_price_cents,
    l.start_date    AS listing_start_date,
    l.end_date      AS listing_end_date,
    (l.status = 'published' AND l.removed_at IS NULL AND o.suspended_at IS NULL)::bool AS listing_available,
    (l.removed_at IS NOT NULL)::bool AS listing_removed,
    cp.id         AS counterpart_id,
    cp.username   AS counterpart_username,
    cp.avatar_url AS counterpart_avatar_url,
    cp.verified   AS counterpart_verified,
    (cp.suspended_at IS NOT NULL)::bool AS counterpart_suspended,
    -- The exact address: null unless the reveal join below matched.
    pl.address_line AS place_address_line,
    pl.lat          AS place_lat,
    pl.lng          AS place_lng,
    lm.sender_id  AS last_message_sender_id,
    lm.body       AS last_message_body,
    lm.created_at AS last_message_created_at,
    (SELECT count(*) FROM messages m
     WHERE m.conversation_id = c.id
       AND m.sender_id <> sqlc.arg('user_id')::uuid
       AND m.created_at > coalesce(
             CASE WHEN c.owner_id = sqlc.arg('user_id')::uuid THEN c.owner_read_at ELSE c.enquirer_read_at END,
             '-infinity'::timestamptz)
    ) AS unread_count
FROM conversations c
JOIN listings l ON l.id = c.listing_id
JOIN users o    ON o.id = c.owner_id
JOIN users cp   ON cp.id = CASE WHEN c.owner_id = sqlc.arg('user_id')::uuid THEN c.enquirer_id ELSE c.owner_id END
-- The reveal condition. The listing again, matched only once the owner has
-- shared its address in this conversation, and not while the listing is taken
-- down or its owner suspended: withheld, never un-shared, so reinstating
-- restores it. It is in the SQL, as the public read filters are, so no handler
-- can forget it. A join rather than a CASE because sqlc types a CASE column as
-- interface{}, and a left-joined table's columns as pointers.
LEFT JOIN listings pl ON pl.id = c.listing_id
    AND c.address_shared_at IS NOT NULL
    AND l.removed_at IS NULL
    AND o.suspended_at IS NULL
-- The newest message. Joined as the messages table itself, rather than as a
-- lateral subquery, so sqlc knows its columns are null for a conversation with
-- no messages yet.
LEFT JOIN messages lm ON lm.id = (
    SELECT m.id FROM messages m
    WHERE m.conversation_id = c.id
    ORDER BY m.created_at DESC, m.id DESC
    LIMIT 1
)
WHERE sqlc.arg('user_id')::uuid IN (c.owner_id, c.enquirer_id)
  AND (sqlc.narg('status')::text IS NULL
       OR (sqlc.narg('status')::text = 'open') = (c.closed_at IS NULL))
  AND (sqlc.narg('side')::text IS NULL
       OR (sqlc.narg('side')::text = 'owner') = (c.owner_id = sqlc.arg('user_id')::uuid))
ORDER BY coalesce(lm.created_at, c.created_at) DESC, c.id DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- The same filters, counted, so the page total describes the whole inbox.
-- name: CountConversationsForUser :one
SELECT count(*)
FROM conversations c
WHERE sqlc.arg('user_id')::uuid IN (c.owner_id, c.enquirer_id)
  AND (sqlc.narg('status')::text IS NULL
       OR (sqlc.narg('status')::text = 'open') = (c.closed_at IS NULL))
  AND (sqlc.narg('side')::text IS NULL
       OR (sqlc.narg('side')::text = 'owner') = (c.owner_id = sqlc.arg('user_id')::uuid));

-- A conversation's messages, newest first. The handler has already checked the
-- caller is a participant through GetConversationForParticipant.
-- name: ListMessages :many
SELECT * FROM messages
WHERE conversation_id = sqlc.arg('conversation_id')
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: CountMessages :one
SELECT count(*) FROM messages WHERE conversation_id = sqlc.arg('conversation_id');

-- Closing, by either participant. Only an open conversation matches, so the
-- first close's time and closer stand; no row means already closed (or not
-- yours), and the handler re-reads.
-- name: CloseConversation :one
UPDATE conversations
SET closed_at = now(), closed_by = sqlc.arg('user_id')::uuid
WHERE id = sqlc.arg('id')::uuid
  AND closed_at IS NULL
  AND sqlc.arg('user_id')::uuid IN (owner_id, enquirer_id)
RETURNING *;

-- Reopening, for one case only: the owner offers the listing against a
-- request by the student they had closed this conversation with, so the offer
-- joins it. Only a conversation closed by that owner matches. One the
-- enquirer closed never does: closing is how a student refuses contact about
-- a listing, and an offer must not get round it. Reopening touches nothing
-- else - not the read markers, and never address_shared_at. No row means it
-- was not closed by this owner.
-- name: ReopenConversation :one
UPDATE conversations
SET closed_at = NULL, closed_by = NULL
WHERE id = sqlc.arg('id')::uuid
  AND closed_by = sqlc.arg('owner_id')::uuid
  AND owner_id = sqlc.arg('owner_id')::uuid
RETURNING *;

-- The owner shares the listing's exact address in this conversation: the
-- one write that sets address_shared_at, and the only way the address reaches
-- another student. Set once - a share already made matches nothing, so its
-- time stands - and only by the owner, while the conversation is open, the
-- enquirer's account is available, the listing is available to the public and
-- it has an address to share. No row affected means already shared, refused,
-- or not this owner's; the handler re-reads and says which.
-- name: ShareConversationAddress :execrows
UPDATE conversations c
SET address_shared_at = now()
FROM listings l, users o, users e
WHERE c.id = sqlc.arg('id')::uuid
  AND c.owner_id = sqlc.arg('owner_id')::uuid
  AND c.address_shared_at IS NULL
  AND c.closed_at IS NULL
  AND l.id = c.listing_id
  AND l.status = 'published'
  AND l.removed_at IS NULL
  AND btrim(l.address_line) <> ''
  AND o.id = c.owner_id
  AND o.suspended_at IS NULL
  AND e.id = c.enquirer_id
  AND e.suspended_at IS NULL;

-- Moves the asking participant's read marker to now. The other side's marker
-- is untouched. No row affected means the user is not a participant.
-- name: MarkConversationRead :execrows
UPDATE conversations
SET owner_read_at    = CASE WHEN owner_id    = sqlc.arg('user_id')::uuid THEN now() ELSE owner_read_at END,
    enquirer_read_at = CASE WHEN enquirer_id = sqlc.arg('user_id')::uuid THEN now() ELSE enquirer_read_at END
WHERE id = sqlc.arg('id')::uuid
  AND sqlc.arg('user_id')::uuid IN (owner_id, enquirer_id);

-- What is waiting for a user across every conversation they are in: messages
-- from the other side newer than the user's own read marker.
-- name: UnreadCountsForUser :one
SELECT
    count(DISTINCT m.conversation_id) AS conversations,
    count(*)                          AS messages
FROM conversations c
JOIN messages m ON m.conversation_id = c.id
WHERE sqlc.arg('user_id')::uuid IN (c.owner_id, c.enquirer_id)
  AND m.sender_id <> sqlc.arg('user_id')::uuid
  AND m.created_at > coalesce(
        CASE WHEN c.owner_id = sqlc.arg('user_id')::uuid THEN c.owner_read_at ELSE c.enquirer_read_at END,
        '-infinity'::timestamptz);

-- Who to tell, by email, about something sender_id just did in this
-- conversation: the other participant, with what the handler needs to decide
-- whether to send (their switch, verification and suspension) and to write the
-- email (their address, the sender's username, the listing's title). Read once,
-- after the write commits. No row unless sender_id is a participant.
--
-- unread_through_message is how many messages the recipient has not read, up
-- to and including message_id: 1 means that message is the first one waiting
-- for them, which is when a reply email is due. Counting only up to that
-- message, rather than everything unread now, keeps two messages sent at once
-- from each seeing the other and both staying silent. Null message_id counts
-- nothing (0).
--
-- No message body and no address is read here: the email may contain neither.
-- name: GetNotificationRecipient :one
SELECT
    r.id                  AS recipient_id,
    r.email               AS recipient_email,
    r.verified            AS recipient_verified,
    (r.suspended_at IS NOT NULL)::bool AS recipient_suspended,
    r.email_notifications AS recipient_email_notifications,
    s.username            AS sender_username,
    l.title               AS listing_title,
    (SELECT count(*) FROM messages m
     WHERE m.conversation_id = c.id
       AND m.sender_id <> r.id
       AND m.created_at > coalesce(
             CASE WHEN c.owner_id = r.id THEN c.owner_read_at ELSE c.enquirer_read_at END,
             '-infinity'::timestamptz)
       AND m.created_at <= (SELECT t.created_at FROM messages t
                            WHERE t.id = sqlc.narg('message_id')::uuid
                              AND t.conversation_id = c.id)
    ) AS unread_through_message
FROM conversations c
JOIN listings l ON l.id = c.listing_id
JOIN users s    ON s.id = sqlc.arg('sender_id')::uuid
JOIN users r    ON r.id = CASE WHEN c.owner_id = sqlc.arg('sender_id')::uuid THEN c.enquirer_id ELSE c.owner_id END
WHERE c.id = sqlc.arg('conversation_id')::uuid
  AND sqlc.arg('sender_id')::uuid IN (c.owner_id, c.enquirer_id);
