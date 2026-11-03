CREATE TABLE tenants (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','disabled','expired')),
    account_limit INTEGER NOT NULL DEFAULT 5 CHECK (account_limit > 0),
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID REFERENCES tenants(id) ON DELETE CASCADE,
    username TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    role TEXT NOT NULL CHECK (role IN ('super_admin','tenant_admin')),
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','disabled')),
    password_changed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK ((role = 'super_admin' AND tenant_id IS NULL) OR (role = 'tenant_admin' AND tenant_id IS NOT NULL))
);
CREATE INDEX users_tenant_idx ON users(tenant_id);

CREATE TABLE activation_cards (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code_hash BYTEA NOT NULL UNIQUE,
    duration_days INTEGER NOT NULL CHECK (duration_days > 0),
    status TEXT NOT NULL DEFAULT 'unused' CHECK (status IN ('unused','activated','disabled')),
    activated_tenant_id UUID REFERENCES tenants(id),
    activated_at TIMESTAMPTZ,
    expires_at TIMESTAMPTZ,
    created_by UUID REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK ((status = 'activated' AND activated_tenant_id IS NOT NULL AND activated_at IS NOT NULL) OR status <> 'activated')
);

CREATE TABLE auth_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash BYTEA NOT NULL UNIQUE,
    device_label TEXT NOT NULL DEFAULT '',
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX auth_sessions_user_idx ON auth_sessions(user_id);

CREATE TABLE telegram_accounts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    phone TEXT NOT NULL,
    display_name TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','authorized','disconnected','error')),
    session_ciphertext BYTEA,
    session_key_version INTEGER,
    proxy_ciphertext BYTEA,
    last_error TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(tenant_id, phone),
    UNIQUE(tenant_id, id)
);
CREATE INDEX telegram_accounts_tenant_status_idx ON telegram_accounts(tenant_id, status);

CREATE TABLE telegram_device_profiles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    telegram_account_id UUID NOT NULL,
    platform TEXT NOT NULL DEFAULT 'ios' CHECK (platform IN ('android','ios','windows','macos','linux','desktop')),
    device_model TEXT NOT NULL,
    system_version TEXT NOT NULL,
    app_version TEXT NOT NULL,
    lang_code TEXT NOT NULL DEFAULT 'en',
    system_lang_code TEXT NOT NULL DEFAULT 'en',
    lang_pack TEXT NOT NULL DEFAULT '',
    generation_seed TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(telegram_account_id),
    FOREIGN KEY (tenant_id, telegram_account_id) REFERENCES telegram_accounts(tenant_id, id) ON DELETE CASCADE
);

CREATE TABLE audit_logs (
    id BIGSERIAL PRIMARY KEY,
    tenant_id UUID REFERENCES tenants(id) ON DELETE SET NULL,
    actor_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    action TEXT NOT NULL,
    resource_type TEXT NOT NULL DEFAULT '',
    resource_id TEXT NOT NULL DEFAULT '',
    details JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX audit_logs_tenant_created_idx ON audit_logs(tenant_id, created_at DESC);
