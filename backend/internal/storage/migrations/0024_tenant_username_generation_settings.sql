CREATE TABLE tenant_username_generation_settings (
    tenant_id UUID PRIMARY KEY REFERENCES tenants(id) ON DELETE CASCADE,
    prefix TEXT NOT NULL DEFAULT 'tg',
    random_length INTEGER NOT NULL DEFAULT 10 CHECK (random_length BETWEEN 1 AND 30),
    charset TEXT NOT NULL DEFAULT 'alphanumeric' CHECK (charset IN ('letters','alphanumeric')),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (length(prefix) + random_length BETWEEN 5 AND 32),
    CHECK (prefix ~ '^[A-Za-z][A-Za-z0-9_]*$' OR prefix = '')
);
ALTER TABLE tenant_username_generation_settings ENABLE ROW LEVEL SECURITY;
ALTER TABLE tenant_username_generation_settings FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_username_generation_settings_tenant_policy
    ON tenant_username_generation_settings
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

ALTER TABLE account_profile_update_jobs
    ADD COLUMN username_prefix TEXT NOT NULL DEFAULT 'tg',
    ADD COLUMN username_random_length INTEGER NOT NULL DEFAULT 10,
    ADD COLUMN username_charset TEXT NOT NULL DEFAULT 'alphanumeric'
        CHECK (username_charset IN ('letters','alphanumeric'));
