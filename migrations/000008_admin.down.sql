DROP TABLE IF EXISTS admin_actions;

ALTER TABLE users DROP CONSTRAINT IF EXISTS users_suspension_whole;

ALTER TABLE users
    DROP COLUMN IF EXISTS suspend_reason,
    DROP COLUMN IF EXISTS suspended_by,
    DROP COLUMN IF EXISTS suspended_at;
