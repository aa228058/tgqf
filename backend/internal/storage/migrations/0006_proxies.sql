CREATE TABLE tenant_proxies (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    protocol TEXT NOT NULL CHECK (protocol IN ('socks5','socks4','http','https')),
    host TEXT NOT NULL,
    port INTEGER NOT NULL CHECK (port BETWEEN 1 AND 65535),
    username TEXT NOT NULL DEFAULT '',
    password_ciphertext BYTEA,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, id),
    UNIQUE (tenant_id, name, host, port)
);

ALTER TABLE tenant_proxies ENABLE ROW LEVEL SECURITY;
ALTER TABLE tenant_proxies FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_proxies_tenant_policy ON tenant_proxies
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

ALTER TABLE telegram_accounts
    ADD COLUMN proxy_id UUID,
    ADD CONSTRAINT telegram_accounts_proxy_fk
    FOREIGN KEY (tenant_id, proxy_id)
    REFERENCES tenant_proxies(tenant_id, id)
    ON DELETE SET NULL;
CREATE INDEX telegram_accounts_proxy_idx ON telegram_accounts(tenant_id, proxy_id);
