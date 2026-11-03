ALTER TABLE message_reply_jobs
    ADD COLUMN message_type TEXT NOT NULL DEFAULT 'text'
        CHECK (message_type IN ('text','contact')),
    ADD COLUMN contact_phone TEXT NOT NULL DEFAULT '',
    ADD COLUMN contact_first_name TEXT NOT NULL DEFAULT '',
    ADD COLUMN contact_last_name TEXT NOT NULL DEFAULT '';

ALTER TABLE message_reply_jobs
    DROP CONSTRAINT IF EXISTS message_reply_jobs_message_text_check;
ALTER TABLE message_reply_jobs
    ADD CONSTRAINT message_reply_jobs_payload_check CHECK (
        (message_type='text' AND length(message_text) BETWEEN 1 AND 4096
            AND contact_phone='' AND contact_first_name='' AND contact_last_name='')
        OR
        (message_type='contact' AND message_text=''
            AND length(contact_phone) BETWEEN 3 AND 32
            AND length(contact_first_name) BETWEEN 1 AND 64
            AND length(contact_last_name)<=64)
    );
