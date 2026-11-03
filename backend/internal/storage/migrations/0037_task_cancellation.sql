ALTER TABLE message_reply_jobs
    DROP CONSTRAINT IF EXISTS message_reply_jobs_status_check;
ALTER TABLE message_reply_jobs
    ADD CONSTRAINT message_reply_jobs_status_check
    CHECK (status IN ('pending','processing','sent','failed','paused','cancelled'));

ALTER TABLE keyword_action_jobs
    DROP CONSTRAINT IF EXISTS keyword_action_jobs_status_check;
ALTER TABLE keyword_action_jobs
    ADD CONSTRAINT keyword_action_jobs_status_check
    CHECK (status IN ('pending','processing','sent','failed','paused','cancelled'));
