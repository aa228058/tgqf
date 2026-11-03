CREATE TABLE tenant_message_chats (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    telegram_account_id UUID NOT NULL,
    chat_id BIGINT NOT NULL,
    chat_title TEXT NOT NULL DEFAULT '',
    chat_type TEXT NOT NULL CHECK (chat_type IN ('private','group','channel')),
    username TEXT NOT NULL DEFAULT '',
    member_count INTEGER NOT NULL DEFAULT 0 CHECK (member_count >= 0),
    is_verified BOOLEAN NOT NULL DEFAULT FALSE,
    photo_id TEXT NOT NULL DEFAULT '',
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (tenant_id, telegram_account_id)
      REFERENCES telegram_accounts(tenant_id, id) ON DELETE CASCADE,
    UNIQUE (tenant_id, telegram_account_id, chat_id)
);

ALTER TABLE tenant_message_chats ENABLE ROW LEVEL SECURITY;
ALTER TABLE tenant_message_chats FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_message_chats_tenant_policy ON tenant_message_chats
  USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
  WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

CREATE INDEX tenant_message_chats_recent_idx
  ON tenant_message_chats(tenant_id, last_seen_at DESC);
