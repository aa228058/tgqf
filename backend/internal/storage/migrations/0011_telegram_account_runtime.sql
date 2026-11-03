CREATE TABLE telegram_account_runtime (
    tenant_id UUID NOT NULL,
    account_id UUID NOT NULL,
    reconnect_attempts INTEGER NOT NULL DEFAULT 0 CHECK (reconnect_attempts >= 0 AND reconnect_attempts <= 30),
    reconnect_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (tenant_id, account_id),
    CONSTRAINT telegram_account_runtime_account_fk
        FOREIGN KEY (tenant_id, account_id)
        REFERENCES telegram_accounts(tenant_id, id)
        ON DELETE CASCADE
);

ALTER TABLE telegram_account_runtime ENABLE ROW LEVEL SECURITY;
ALTER TABLE telegram_account_runtime FORCE ROW LEVEL SECURITY;
CREATE POLICY telegram_account_runtime_tenant_policy ON telegram_account_runtime
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

CREATE INDEX telegram_account_runtime_reconnect_idx
    ON telegram_account_runtime(reconnect_at);
