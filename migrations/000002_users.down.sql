DROP TABLE IF EXISTS refresh_tokens;
DROP TABLE IF EXISTS password_reset_tokens;
DROP TABLE IF EXISTS email_verification_tokens;
DROP TRIGGER IF EXISTS users_set_updated_at ON users;
DROP TABLE IF EXISTS users;
