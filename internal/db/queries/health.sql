-- Health check. Trivial on purpose: it exists to prove the whole chain — pool,
-- sqlc codegen, generated method, real round-trip — works end to end, before any
-- domain table exists to query.

-- name: Ping :one
SELECT 1::int AS ok;
