ALTER TABLE tenant_keyword_rules
    ADD COLUMN IF NOT EXISTS reply_text TEXT NOT NULL DEFAULT ''
        CHECK (length(reply_text) <= 4096);

CREATE TABLE IF NOT EXISTS keyword_action_jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    keyword_match_id BIGINT NOT NULL UNIQUE,
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
    FOREIGN KEY (tenant_id, keyword_match_id)
      REFERENCES tenant_keyword_matches(tenant_id, id) ON DELETE CASCADE,
    FOREIGN KEY (tenant_id, telegram_account_id)
      REFERENCES telegram_accounts(tenant_id, id) ON DELETE CASCADE
);
ALTER TABLE keyword_action_jobs ENABLE ROW LEVEL SECURITY;
ALTER TABLE keyword_action_jobs FORCE ROW LEVEL SECURITY;
CREATE POLICY keyword_action_jobs_tenant_policy ON keyword_action_jobs
 USING (tenant_id = NULLIF(current_setting('app.tenant_id',true),'')::uuid)
 WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id',true),'')::uuid);
CREATE INDEX keyword_action_jobs_worker_idx ON keyword_action_jobs(status,available_at,lease_until,created_at)
 WHERE status IN ('pending','processing');
