ALTER TABLE message_reply_jobs
    DROP CONSTRAINT IF EXISTS message_reply_jobs_status_check;

ALTER TABLE message_reply_jobs
    ADD CONSTRAINT message_reply_jobs_status_check
    CHECK (status IN ('pending','processing','sent','failed','paused'));
