CREATE TABLE tenant_join_tasks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    UNIQUE(tenant_id,id),
    name TEXT NOT NULL CHECK (length(name) BETWEEN 1 AND 120),
    target_group_id BIGINT REFERENCES public_groups(id) ON DELETE RESTRICT,
    target_link_id BIGINT REFERENCES public_group_links(id) ON DELETE RESTRICT,
    status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','queued','paused','completed','failed','cancelled')),
    authorization_confirmed BOOLEAN NOT NULL DEFAULT FALSE,
    authorization_note TEXT NOT NULL DEFAULT '',
    authorized_by UUID REFERENCES users(id) ON DELETE RESTRICT,
    authorized_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (target_group_id IS NOT NULL OR target_link_id IS NOT NULL),
    CHECK ((authorization_confirmed=FALSE AND authorized_by IS NULL AND authorized_at IS NULL) OR
           (authorization_confirmed=TRUE AND authorized_by IS NOT NULL AND authorized_at IS NOT NULL))
);
CREATE INDEX tenant_join_tasks_recent_idx ON tenant_join_tasks(tenant_id, created_at DESC);
CREATE INDEX tenant_join_tasks_queue_idx ON tenant_join_tasks(status, created_at) WHERE status='queued';
ALTER TABLE tenant_join_tasks ENABLE ROW LEVEL SECURITY;
ALTER TABLE tenant_join_tasks FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_join_tasks_tenant_policy ON tenant_join_tasks
 USING (tenant_id = NULLIF(current_setting('app.tenant_id',true),'')::uuid)
 WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id',true),'')::uuid);

CREATE TABLE tenant_join_task_accounts (
    id BIGSERIAL PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    task_id UUID NOT NULL,
    telegram_account_id UUID NOT NULL,
    status TEXT NOT NULL DEFAULT 'paused' CHECK (status IN ('queued','paused','request_pending','joined','already_member','failed','skipped','cancelled')),
    application_status TEXT NOT NULL DEFAULT 'not_started' CHECK (application_status IN ('not_started','request_pending','approved','declined','not_required','unknown')),
    attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts>=0),
    available_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    lease_until TIMESTAMPTZ,
    telegram_chat_id BIGINT,
    last_error TEXT NOT NULL DEFAULT '',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (tenant_id,task_id) REFERENCES tenant_join_tasks(tenant_id,id) ON DELETE CASCADE,
    FOREIGN KEY (tenant_id,telegram_account_id) REFERENCES telegram_accounts(tenant_id,id) ON DELETE CASCADE,
    UNIQUE(task_id,telegram_account_id)
);
CREATE INDEX tenant_join_task_accounts_task_idx ON tenant_join_task_accounts(tenant_id,task_id,status);
CREATE INDEX tenant_join_task_accounts_worker_idx ON tenant_join_task_accounts(status,available_at,lease_until,created_at) WHERE status IN ('queued','request_pending');
CREATE INDEX tenant_join_task_accounts_account_idx ON tenant_join_task_accounts(tenant_id,telegram_account_id,created_at DESC);
ALTER TABLE tenant_join_task_accounts ENABLE ROW LEVEL SECURITY;
ALTER TABLE tenant_join_task_accounts FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_join_task_accounts_tenant_policy ON tenant_join_task_accounts
 USING (tenant_id = NULLIF(current_setting('app.tenant_id',true),'')::uuid)
 WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id',true),'')::uuid);
