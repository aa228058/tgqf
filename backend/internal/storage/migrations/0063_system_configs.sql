CREATE TABLE system_configs (
    key TEXT PRIMARY KEY,
    value JSONB NOT NULL,
    updated_by UUID REFERENCES users(id) ON DELETE SET NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO system_configs(key,value) VALUES
    ('registration_enabled','true'::jsonb),
    ('default_account_limit','5'::jsonb),
    ('auth_session_ttl_hours','720'::jsonb)
ON CONFLICT(key) DO NOTHING;
