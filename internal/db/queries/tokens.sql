-- Every lookup here filters on consumed/revoked and expiry in SQL rather than in
-- Go, so a caller cannot forget the check and accept a spent token.

-- name: CreateEmailVerificationToken :exec
INSERT INTO email_verification_tokens (token_hash, user_id, expires_at)
VALUES ($1, $2, $3);

-- name: GetLiveEmailVerificationToken :one
SELECT * FROM email_verification_tokens
WHERE token_hash = $1 AND consumed_at IS NULL AND expires_at > now();

-- name: ConsumeEmailVerificationToken :exec
UPDATE email_verification_tokens SET consumed_at = now()
WHERE token_hash = $1 AND consumed_at IS NULL;

-- A fresh request invalidates earlier outstanding ones, so a forwarded old email
-- cannot still be used.
-- name: ConsumeAllEmailVerificationTokensForUser :exec
UPDATE email_verification_tokens SET consumed_at = now()
WHERE user_id = $1 AND consumed_at IS NULL;

-- name: CreatePasswordResetToken :exec
INSERT INTO password_reset_tokens (token_hash, user_id, expires_at)
VALUES ($1, $2, $3);

-- name: GetLivePasswordResetToken :one
SELECT * FROM password_reset_tokens
WHERE token_hash = $1 AND consumed_at IS NULL AND expires_at > now();

-- name: ConsumePasswordResetToken :exec
UPDATE password_reset_tokens SET consumed_at = now()
WHERE token_hash = $1 AND consumed_at IS NULL;

-- name: ConsumeAllPasswordResetTokensForUser :exec
UPDATE password_reset_tokens SET consumed_at = now()
WHERE user_id = $1 AND consumed_at IS NULL;

-- name: CreateRefreshToken :exec
INSERT INTO refresh_tokens (token_hash, user_id, expires_at, user_agent)
VALUES ($1, $2, $3, $4);

-- name: GetLiveRefreshToken :one
SELECT * FROM refresh_tokens
WHERE token_hash = $1 AND revoked_at IS NULL AND expires_at > now();

-- name: RevokeRefreshToken :exec
UPDATE refresh_tokens SET revoked_at = now()
WHERE token_hash = $1 AND revoked_at IS NULL;

-- Used on password change: every existing session is ended, so a password reset
-- actually evicts whoever prompted it.
-- name: RevokeAllRefreshTokensForUser :exec
UPDATE refresh_tokens SET revoked_at = now()
WHERE user_id = $1 AND revoked_at IS NULL;

-- name: DeleteExpiredTokens :exec
WITH deleted_refresh AS (
    DELETE FROM refresh_tokens WHERE expires_at < now()
), deleted_verification AS (
    DELETE FROM email_verification_tokens WHERE expires_at < now()
)
DELETE FROM password_reset_tokens WHERE expires_at < now();
