-- A task-center pause/cancel request must not revoke a Telegram RPC already
-- in flight. Workers consume these flags when the current RPC returns or lease expires.
ALTER TABLE tenant_join_task_accounts
    ADD COLUMN IF NOT EXISTS pause_requested BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS cancel_requested BOOLEAN NOT NULL DEFAULT FALSE;

CREATE INDEX IF NOT EXISTS tenant_join_task_accounts_control_request_idx
    ON tenant_join_task_accounts(tenant_id,lease_until)
    WHERE status='processing' AND (pause_requested=TRUE OR cancel_requested=TRUE);
