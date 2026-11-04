CREATE TABLE system_alerts (
    id BIGSERIAL PRIMARY KEY,
    alert_key TEXT NOT NULL,
    severity TEXT NOT NULL CHECK (severity IN ('warning','critical')),
    status TEXT NOT NULL CHECK (status IN ('active','resolved')),
    title TEXT NOT NULL,
    details JSONB NOT NULL DEFAULT '{}'::jsonb,
    opened_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    resolved_at TIMESTAMPTZ
);
CREATE UNIQUE INDEX system_alerts_one_active_key_idx ON system_alerts(alert_key) WHERE status='active';
CREATE INDEX system_alerts_recent_idx ON system_alerts(status,opened_at DESC);
