CREATE TABLE IF NOT EXISTS worker_instances (
    instance_id TEXT PRIMARY KEY,
    hostname TEXT NOT NULL DEFAULT '',
    process_id INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL CHECK (status IN ('running','stopped')),
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    heartbeat_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    stopped_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS worker_instances_heartbeat_idx ON worker_instances(heartbeat_at DESC);
