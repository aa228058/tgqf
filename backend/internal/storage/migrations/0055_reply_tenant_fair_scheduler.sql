ALTER TABLE tenants
    ADD COLUMN reply_jobs_last_claimed_at TIMESTAMPTZ;

CREATE INDEX tenants_reply_scheduler_idx
    ON tenants(reply_jobs_last_claimed_at, id)
    WHERE status = 'active';
