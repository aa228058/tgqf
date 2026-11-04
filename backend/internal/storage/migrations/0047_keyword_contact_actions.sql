ALTER TABLE tenant_keyword_rules
    ADD COLUMN IF NOT EXISTS action_type TEXT NOT NULL DEFAULT 'text',
    ADD COLUMN IF NOT EXISTS contact_phone TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS contact_first_name TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS contact_last_name TEXT NOT NULL DEFAULT '';

ALTER TABLE tenant_keyword_rules
    ADD CONSTRAINT tenant_keyword_rules_action_type_check
        CHECK (action_type IN ('text','contact')),
    ADD CONSTRAINT tenant_keyword_rules_contact_config_check
        CHECK (
            (action_type='text' AND contact_phone='' AND contact_first_name='' AND contact_last_name='')
            OR
            (action_type='contact'
                AND length(contact_phone) BETWEEN 1 AND 64
                AND length(contact_first_name) BETWEEN 1 AND 128
                AND length(contact_last_name) <= 128
                AND reply_text=''
                AND reply_text_by_chat_type='{}'::jsonb)
        );

ALTER TABLE keyword_action_jobs
    ADD COLUMN IF NOT EXISTS action_type TEXT NOT NULL DEFAULT 'text',
    ADD COLUMN IF NOT EXISTS contact_phone TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS contact_first_name TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS contact_last_name TEXT NOT NULL DEFAULT '';

ALTER TABLE keyword_action_jobs
    DROP CONSTRAINT IF EXISTS keyword_action_jobs_message_text_check,
    ADD CONSTRAINT keyword_action_jobs_action_type_check
        CHECK (action_type IN ('text','contact')),
    ADD CONSTRAINT keyword_action_jobs_action_payload_check
        CHECK (
            (action_type='text' AND length(message_text) BETWEEN 1 AND 4096
                AND contact_phone='' AND contact_first_name='' AND contact_last_name='')
            OR
            (action_type='contact' AND message_text=''
                AND length(contact_phone) BETWEEN 1 AND 64
                AND length(contact_first_name) BETWEEN 1 AND 128
                AND length(contact_last_name) <= 128)
        );

CREATE INDEX IF NOT EXISTS keyword_action_jobs_action_type_idx
    ON keyword_action_jobs(tenant_id,action_type,status,available_at);
