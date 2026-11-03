CREATE TABLE tenant_proxy_checks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    proxy_id UUID NOT NULL,
    check_type TEXT NOT NULL DEFAULT 'telegram_dc_tunnel',
    status TEXT NOT NULL CHECK (status IN ('success','failed')),
    latency_ms BIGINT NOT NULL DEFAULT 0 CHECK (latency_ms >= 0),
    message TEXT NOT NULL DEFAULT '',
    checked_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (tenant_id, proxy_id)
        REFERENCES tenant_proxies(tenant_id, id)
        ON DELETE CASCADE
);

ALTER TABLE tenant_proxy_checks ENABLE ROW LEVEL SECURITY;
ALTER TABLE tenant_proxy_checks FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_proxy_checks_tenant_policy ON tenant_proxy_checks
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

CREATE INDEX tenant_proxy_checks_recent_idx
    ON tenant_proxy_checks(tenant_id, proxy_id, checked_at DESC);
