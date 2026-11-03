ALTER TABLE message_reply_jobs DROP CONSTRAINT IF EXISTS message_reply_jobs_status_check;
ALTER TABLE message_reply_jobs ADD CONSTRAINT message_reply_jobs_status_check CHECK (status IN ('pending','processing','sent','failed','paused'));
CREATE INDEX message_reply_jobs_paused_idx ON message_reply_jobs(tenant_id, updated_at DESC) WHERE status='paused';
