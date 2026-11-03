ALTER TABLE tenant_join_task_accounts
    DROP CONSTRAINT tenant_join_task_accounts_status_check;
ALTER TABLE tenant_join_task_accounts
    ADD CONSTRAINT tenant_join_task_accounts_status_check
    CHECK (status IN ('queued','processing','paused','request_pending','joined','already_member','failed','skipped','cancelled'));
CREATE INDEX tenant_join_task_accounts_claim_idx
    ON tenant_join_task_accounts(tenant_id,status,available_at,created_at)
    WHERE status='queued';
