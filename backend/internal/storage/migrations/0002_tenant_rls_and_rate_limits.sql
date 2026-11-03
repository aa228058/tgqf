CREATE TABLE request_rate_limits (
    key_hash BYTEA PRIMARY KEY,
    window_started_at TIMESTAMPTZ NOT NULL,
    attempts INTEGER NOT NULL CHECK (attempts >= 0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX request_rate_limits_updated_idx ON request_rate_limits(updated_at);

ALTER TABLE telegram_accounts ENABLE ROW LEVEL SECURITY;
ALTER TABLE telegram_accounts FORCE ROW LEVEL SECURITY;
CREATE POLICY telegram_accounts_tenant_policy ON telegram_accounts
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

ALTER TABLE telegram_device_profiles ENABLE ROW LEVEL SECURITY;
ALTER TABLE telegram_device_profiles FORCE ROW LEVEL SECURITY;
CREATE POLICY telegram_device_profiles_tenant_policy ON telegram_device_profiles
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
