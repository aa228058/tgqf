-- Actual Telegram memberships are account-owned facts, independent from the
-- tenant-wide public group catalogue. Deleting a catalogue entry must never
-- remove an account's observed membership.
CREATE TABLE telegram_account_groups (
    id BIGSERIAL PRIMARY KEY,
    tenant_id UUID NOT NULL,
    account_id UUID NOT NULL REFERENCES telegram_accounts(id) ON DELETE CASCADE,
    chat_id BIGINT NOT NULL,
    chat_type TEXT NOT NULL CHECK (chat_type IN ('group', 'supergroup', 'channel')),
    title TEXT NOT NULL DEFAULT '',
    username TEXT NOT NULL DEFAULT '',
    membership_status TEXT NOT NULL DEFAULT 'member',
    is_member BOOLEAN NOT NULL DEFAULT TRUE,
    first_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_synced_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    left_at TIMESTAMPTZ,
    UNIQUE (account_id, chat_id)
);

CREATE INDEX telegram_account_groups_tenant_account_idx
    ON telegram_account_groups (tenant_id, account_id, is_member, title);
CREATE INDEX telegram_account_groups_chat_idx
    ON telegram_account_groups (chat_id);

ALTER TABLE telegram_account_groups ENABLE ROW LEVEL SECURITY;
ALTER TABLE telegram_account_groups FORCE ROW LEVEL SECURITY;
CREATE POLICY telegram_account_groups_tenant_policy ON telegram_account_groups
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

-- One row per account records the outcome of its latest full membership scan.
-- A failed/incomplete scan must not mark previously observed memberships as left.
CREATE TABLE telegram_account_group_sync_state (
    tenant_id UUID NOT NULL,
    account_id UUID PRIMARY KEY REFERENCES telegram_accounts(id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'never' CHECK (status IN ('never', 'running', 'succeeded', 'failed')),
    group_count INTEGER NOT NULL DEFAULT 0 CHECK (group_count >= 0),
    last_started_at TIMESTAMPTZ,
    last_synced_at TIMESTAMPTZ,
    last_error TEXT NOT NULL DEFAULT '',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE telegram_account_group_sync_state ENABLE ROW LEVEL SECURITY;
ALTER TABLE telegram_account_group_sync_state FORCE ROW LEVEL SECURITY;
CREATE POLICY telegram_account_group_sync_state_tenant_policy ON telegram_account_group_sync_state
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
