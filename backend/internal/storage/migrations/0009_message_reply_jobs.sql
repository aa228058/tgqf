CREATE TABLE message_reply_jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    unified_message_id UUID NOT NULL,
    telegram_account_id UUID NOT NULL,
    chat_id BIGINT NOT NULL,
    reply_to_message_id BIGINT NOT NULL,
    message_text TEXT NOT NULL CHECK (length(message_text) BETWEEN 1 AND 4096),
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','processing','sent','failed')),
    attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    available_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    lease_until TIMESTAMPTZ,
    telegram_message_id BIGINT,
    last_error TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (tenant_id, unified_message_id)
      REFERENCES unified_messages(tenant_id, id) ON DELETE CASCADE,
    FOREIGN KEY (tenant_id, telegram_account_id)
      REFERENCES telegram_accounts(tenant_id, id) ON DELETE CASCADE
);

ALTER TABLE message_reply_jobs ENABLE ROW LEVEL SECURITY;
ALTER TABLE message_reply_jobs FORCE ROW LEVEL SECURITY;
CREATE POLICY message_reply_jobs_tenant_policy ON message_reply_jobs
  USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
  WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

CREATE INDEX message_reply_jobs_worker_idx
  ON message_reply_jobs(status, available_at, lease_until, created_at)
  WHERE status IN ('pending','processing');
CREATE INDEX message_reply_jobs_tenant_idx
  ON message_reply_jobs(tenant_id, created_at DESC);
