ALTER TABLE tenants
    ADD COLUMN IF NOT EXISTS keyword_actions_last_claimed_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS tenants_keyword_action_scheduler_idx
    ON tenants(keyword_actions_last_claimed_at NULLS FIRST, id)
    WHERE status='active';
