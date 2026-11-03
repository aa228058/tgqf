ALTER TABLE message_reply_jobs
    ADD COLUMN IF NOT EXISTS idempotency_key TEXT,
    ADD COLUMN IF NOT EXISTS request_hash TEXT;

CREATE UNIQUE INDEX IF NOT EXISTS message_reply_jobs_tenant_idempotency_idx
    ON message_reply_jobs(tenant_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL;

