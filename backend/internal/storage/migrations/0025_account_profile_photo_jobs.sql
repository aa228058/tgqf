ALTER TABLE account_profile_update_jobs
    ADD COLUMN photo_data BYTEA NOT NULL DEFAULT ''::bytea;

ALTER TABLE account_profile_update_jobs
    DROP CONSTRAINT IF EXISTS account_profile_update_jobs_check;

ALTER TABLE account_profile_update_jobs
    ADD CONSTRAINT account_profile_update_jobs_payload_check
    CHECK (
        length(first_name) > 0 OR length(last_name) > 0 OR length(about) > 0
        OR generate_username OR octet_length(photo_data) > 0
    ),
    ADD CONSTRAINT account_profile_update_jobs_photo_size_check
    CHECK (octet_length(photo_data) <= 5242880);
