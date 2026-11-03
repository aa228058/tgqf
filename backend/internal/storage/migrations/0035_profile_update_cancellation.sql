ALTER TABLE account_profile_update_jobs
    DROP CONSTRAINT IF EXISTS account_profile_update_jobs_status_check;

ALTER TABLE account_profile_update_jobs
    ADD CONSTRAINT account_profile_update_jobs_status_check
    CHECK (status IN ('pending','processing','paused','completed','failed','cancelled'));
