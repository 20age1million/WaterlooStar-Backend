-- The operator: suspension on accounts, and the ledger of what admins do.
--
-- Suspension is a timestamp rather than a flag or a deletion, in the same shape
-- as request_offers.withdrawn_at: reversible, self-dating, and impossible to
-- confuse with never having happened. Nothing reads these columns until Phase
-- 10; they exist now so the ledger and the account view have one shape from the
-- start.

ALTER TABLE users
    ADD COLUMN suspended_at   timestamptz,
    -- Nullable, and SET NULL rather than cascade: deleting the admin who acted
    -- must not reinstate or erase the account they suspended.
    ADD COLUMN suspended_by   uuid REFERENCES users (id) ON DELETE SET NULL,
    ADD COLUMN suspend_reason text;

-- A suspension without a reason, or a reason without a suspension, is a
-- half-written change.
ALTER TABLE users
    ADD CONSTRAINT users_suspension_whole
    CHECK ((suspended_at IS NULL) = (suspend_reason IS NULL));

-- Append-only. The query surface has no UPDATE and no DELETE against this
-- table, and a test in internal/httpapi fails if one appears.
CREATE TABLE admin_actions (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    -- NULL when the change was made from the host with cmd/admin, and when the
    -- acting account has since been deleted: the ledger outlives its actors.
    actor_id     uuid REFERENCES users (id) ON DELETE SET NULL,
    action       varchar(20) NOT NULL,
    subject_type varchar(10) NOT NULL,
    subject_id   uuid        NOT NULL,
    reason       text        NOT NULL,
    -- The before and after of this particular change, e.g. {"from": "user",
    -- "to": "admin"}. Shape varies by action; the action names it.
    detail       jsonb       NOT NULL DEFAULT '{}'::jsonb,
    created_at   timestamptz NOT NULL DEFAULT now(),

    -- Every action the feature will ever write, so a typo in a handler fails
    -- here rather than entering the ledger. Phases 10 and 11 write the rest.
    CONSTRAINT admin_actions_action_known CHECK (action IN (
        'set_role', 'suspend', 'reinstate', 'verify', 'remove_post', 'restore_post'
    )),
    CONSTRAINT admin_actions_subject_known CHECK (subject_type IN ('user', 'listing', 'request')),
    -- A ledger of empty reasons is a ledger of nothing.
    CONSTRAINT admin_actions_reason_present CHECK (length(btrim(reason)) > 0),
    CONSTRAINT admin_actions_reason_length  CHECK (length(reason) <= 1000)
);

-- The ledger page, newest first.
CREATE INDEX admin_actions_created_idx ON admin_actions (created_at DESC);
-- "What happened to this account" is one index scan.
CREATE INDEX admin_actions_subject_idx ON admin_actions (subject_type, subject_id, created_at DESC);
