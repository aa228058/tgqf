CREATE TABLE unified_messages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    telegram_account_id UUID NOT NULL,
    chat_id BIGINT NOT NULL,
    message_id BIGINT NOT NULL,
    chat_type TEXT NOT NULL CHECK (chat_type IN ('private','group','channel')),
    chat_title TEXT NOT NULL DEFAULT '',
    sender_id BIGINT NOT NULL DEFAULT 0,
    reply_to_message_id BIGINT,
    reply_to_own_message BOOLEAN NOT NULL DEFAULT FALSE,
    is_outgoing BOOLEAN NOT NULL DEFAULT FALSE,
    message_text TEXT NOT NULL DEFAULT '',
    media_type TEXT NOT NULL DEFAULT '',
    message_date TIMESTAMPTZ NOT NULL,
    processed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (tenant_id, telegram_account_id)
      REFERENCES telegram_accounts(tenant_id, id) ON DELETE CASCADE,
    UNIQUE (telegram_account_id, chat_id, message_id)
);

ALTER TABLE unified_messages ENABLE ROW LEVEL SECURITY;
ALTER TABLE unified_messages FORCE ROW LEVEL SECURITY;
CREATE POLICY unified_messages_tenant_policy ON unified_messages
  USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
  WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

CREATE INDEX unified_messages_inbox_idx
  ON unified_messages(tenant_id, message_date DESC, id DESC)
  WHERE processed_at IS NULL;
CREATE INDEX unified_messages_account_idx
  ON unified_messages(tenant_id, telegram_account_id, message_date DESC);
