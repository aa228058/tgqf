ALTER TABLE tenant_join_task_accounts
    ADD COLUMN approval_check_lease_until TIMESTAMPTZ;

CREATE INDEX tenant_join_task_approval_due_idx
    ON tenant_join_task_accounts(tenant_id, approval_check_lease_until, updated_at)
    WHERE status = 'request_pending';
