CREATE TABLE tenant_message_settings (
    tenant_id UUID PRIMARY KEY REFERENCES tenants(id) ON DELETE CASCADE,
    settings JSONB NOT NULL DEFAULT '{
      "enabled": true,
      "include_all_accounts": true,
      "account_ids": [],
      "chat_ids": [],
      "include_text": true,
      "include_media": true,
      "keywords": []
    }'::jsonb,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE tenant_message_settings ENABLE ROW LEVEL SECURITY;
ALTER TABLE tenant_message_settings FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_message_settings_tenant_policy ON tenant_message_settings
  USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
  WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
