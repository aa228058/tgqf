ALTER TABLE tenant_join_tasks
    ADD COLUMN available_at TIMESTAMPTZ NOT NULL DEFAULT NOW();
ALTER TABLE telegram_accounts
    ADD COLUMN join_flood_wait_until TIMESTAMPTZ;
CREATE INDEX tenant_join_tasks_available_idx
    ON tenant_join_tasks(status,available_at,created_at)
    WHERE status='queued';
