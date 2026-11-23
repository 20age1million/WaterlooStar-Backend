-- Every public read keeps deletion and account suspension filtering in SQL.
-- name: ListForumPosts :many
SELECT
    sqlc.embed(p),
    u.username   AS author_username,
    u.avatar_url AS author_avatar_url,
    u.verified   AS author_verified
FROM forum_posts p
JOIN users u ON u.id = p.author_id
WHERE p.deleted_at IS NULL
  AND u.suspended_at IS NULL
  AND (sqlc.narg('search')::text IS NULL
       OR p.search @@ websearch_to_tsquery('english', sqlc.narg('search')::text))
ORDER BY p.created_at DESC, p.id DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: CountForumPosts :one
SELECT count(*)
FROM forum_posts p
JOIN users u ON u.id = p.author_id
WHERE p.deleted_at IS NULL
  AND u.suspended_at IS NULL
  AND (sqlc.narg('search')::text IS NULL
       OR p.search @@ websearch_to_tsquery('english', sqlc.narg('search')::text));

-- name: GetForumPost :one
SELECT
    sqlc.embed(p),
    u.username   AS author_username,
    u.avatar_url AS author_avatar_url,
    u.verified   AS author_verified
FROM forum_posts p
JOIN users u ON u.id = p.author_id
WHERE p.id = $1
  AND p.deleted_at IS NULL
  AND u.suspended_at IS NULL;

-- An account must still be active when the INSERT executes. Verification is
-- deliberately not required for forum posts.
-- name: CreateForumPost :one
INSERT INTO forum_posts (author_id, title, body)
SELECT u.id, sqlc.arg('title')::text, sqlc.arg('body')::text
FROM users u
WHERE u.id = sqlc.arg('author_id')::uuid
  AND u.suspended_at IS NULL
RETURNING *;

-- Ownership, deletion and current account state constrain the write itself.
-- name: UpdateForumPost :one
UPDATE forum_posts p
SET title = coalesce(sqlc.narg('title')::text, p.title),
    body = coalesce(sqlc.narg('body')::text, p.body)
WHERE p.id = sqlc.arg('id')::uuid
  AND p.author_id = sqlc.arg('author_id')::uuid
  AND p.deleted_at IS NULL
  AND EXISTS (
      SELECT 1 FROM users u
      WHERE u.id = p.author_id AND u.suspended_at IS NULL
  )
RETURNING p.*;

-- name: DeleteForumPost :one
UPDATE forum_posts p
SET deleted_at = now()
WHERE p.id = sqlc.arg('id')::uuid
  AND p.author_id = sqlc.arg('author_id')::uuid
  AND p.deleted_at IS NULL
  AND EXISTS (
      SELECT 1 FROM users u
      WHERE u.id = p.author_id AND u.suspended_at IS NULL
  )
RETURNING p.id;
