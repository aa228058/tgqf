ALTER TABLE tenant_proxies ADD COLUMN is_default BOOLEAN NOT NULL DEFAULT FALSE;
CREATE UNIQUE INDEX tenant_proxies_one_default_idx ON tenant_proxies(tenant_id) WHERE is_default=TRUE;
