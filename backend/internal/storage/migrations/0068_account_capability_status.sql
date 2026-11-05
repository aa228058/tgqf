-- Persist capability restrictions only when Telegram explicitly reports them.
-- A restriction on one chat must not be promoted to an account-wide restriction.
CREATE TABLE telegram_account_capability_status (
    tenant_id UUID NOT NULL,
    account_id UUID NOT NULL,
    capability TEXT NOT NULL CHECK (capability IN ('private_message','group_message','media_send','group_join','message_send')),
    chat_id BIGINT NOT NULL DEFAULT 0,
    status TEXT NOT NULL CHECK (status IN ('restricted','cooldown','unknown')),
    rpc_error TEXT NOT NULL DEFAULT '',
    reason TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL DEFAULT '',
    last_observed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (tenant_id, account_id, capability, chat_id),
    FOREIGN KEY (tenant_id, account_id)
      REFERENCES telegram_accounts(tenant_id, id) ON DELETE CASCADE
);

ALTER TABLE telegram_account_capability_status ENABLE ROW LEVEL SECURITY;
ALTER TABLE telegram_account_capability_status FORCE ROW LEVEL SECURITY;
CREATE POLICY telegram_account_capability_status_tenant_policy
  ON telegram_account_capability_status
  USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
  WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

CREATE INDEX telegram_account_capability_status_recent_idx
  ON telegram_account_capability_status(tenant_id, account_id, last_observed_at DESC);
