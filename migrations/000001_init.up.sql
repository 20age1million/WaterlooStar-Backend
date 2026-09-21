-- Baseline. No domain tables yet: users arrive in Phase 1, listings in Phase 2.
-- This migration establishes the conventions every later table depends on.

-- PostgreSQL 13+ provides gen_random_uuid() in core, so no pgcrypto extension is
-- needed. Fail loudly here rather than in a later migration if that is not true.
DO $$
BEGIN
    PERFORM gen_random_uuid();
EXCEPTION WHEN undefined_function THEN
    RAISE EXCEPTION 'gen_random_uuid() unavailable; PostgreSQL 13 or later is required';
END
$$;

-- Every table in this schema carries created_at/updated_at. This trigger keeps
-- updated_at honest without each write having to remember to set it.
CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
