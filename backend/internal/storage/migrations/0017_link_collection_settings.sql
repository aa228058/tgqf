CREATE TABLE tenant_link_collection_settings (
 tenant_id UUID PRIMARY KEY REFERENCES tenants(id) ON DELETE CASCADE,
 enabled BOOLEAN NOT NULL DEFAULT TRUE,
 from_private_messages BOOLEAN NOT NULL DEFAULT TRUE,
 from_groups BOOLEAN NOT NULL DEFAULT TRUE,
 from_channels BOOLEAN NOT NULL DEFAULT TRUE,
 updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
ALTER TABLE tenant_link_collection_settings ENABLE ROW LEVEL SECURITY;
ALTER TABLE tenant_link_collection_settings FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_link_collection_settings_tenant_policy ON tenant_link_collection_settings USING (tenant_id = NULLIF(current_setting('app.tenant_id',true),'')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id',true),'')::uuid);
