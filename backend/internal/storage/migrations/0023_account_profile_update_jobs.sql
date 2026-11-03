CREATE TABLE account_profile_update_jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    batch_id UUID NOT NULL DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    account_id UUID NOT NULL,
    requested_by_user_id UUID NOT NULL REFERENCES users(id),
    first_name TEXT NOT NULL DEFAULT '',
    last_name TEXT NOT NULL DEFAULT '',
    about TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending','processing','paused','completed','failed')),
    attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    next_run_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    lease_until TIMESTAMPTZ,
    last_error TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (tenant_id, account_id)
        REFERENCES telegram_accounts(tenant_id, id) ON DELETE CASCADE,
    CHECK (length(first_name) > 0 OR length(last_name) > 0 OR length(about) > 0),
    CHECK ((status='processing' AND lease_until IS NOT NULL)
        OR (status<>'processing' AND lease_until IS NULL))
);
CREATE UNIQUE INDEX account_profile_update_one_active_idx
    ON account_profile_update_jobs(tenant_id,account_id)
    WHERE status IN ('pending','processing','paused');
CREATE INDEX account_profile_update_claim_idx
    ON account_profile_update_jobs(tenant_id,status,next_run_at,created_at);
CREATE INDEX account_profile_update_batch_idx
    ON account_profile_update_jobs(tenant_id,batch_id,created_at);
