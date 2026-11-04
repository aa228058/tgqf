-- A voice-call action stores the sender as the call target and records the
-- native Telegram call identity separately from outgoing chat messages.
ALTER TABLE keyword_action_jobs
    ADD COLUMN IF NOT EXISTS target_user_id BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS telegram_call_id BIGINT,
    ADD COLUMN IF NOT EXISTS telegram_call_access_hash BIGINT,
    ADD COLUMN IF NOT EXISTS voice_call_state TEXT NOT NULL DEFAULT ''
        CHECK (voice_call_state IN ('','ringing','accepted','discarded','ended','failed','unknown'));

ALTER TABLE tenant_keyword_rules
    DROP CONSTRAINT IF EXISTS tenant_keyword_rules_action_type_check,
    ADD CONSTRAINT tenant_keyword_rules_action_type_check
        CHECK (action_type IN ('text','contact','media','media_album','voice_call')),
    DROP CONSTRAINT IF EXISTS tenant_keyword_rules_action_payload_check,
    ADD CONSTRAINT tenant_keyword_rules_action_payload_check CHECK (
        (action_type='text' AND contact_phone='' AND contact_first_name='' AND contact_last_name='' AND media_asset_id IS NULL AND media_type='' AND media_caption='' AND media_album_items='[]'::jsonb)
     OR (action_type='contact' AND length(contact_phone) BETWEEN 1 AND 64 AND length(contact_first_name) BETWEEN 1 AND 128 AND length(contact_last_name)<=128 AND reply_text='' AND reply_text_by_chat_type='{}'::jsonb AND media_asset_id IS NULL AND media_type='' AND media_caption='' AND media_album_items='[]'::jsonb)
     OR (action_type='media' AND contact_phone='' AND contact_first_name='' AND contact_last_name='' AND media_asset_id IS NOT NULL AND media_type IN ('photo','video','document','audio','voice','video_note') AND length(media_caption)<=4096 AND reply_text='' AND reply_text_by_chat_type='{}'::jsonb AND media_album_items='[]'::jsonb)
     OR (action_type='media_album' AND contact_phone='' AND contact_first_name='' AND contact_last_name='' AND media_asset_id IS NULL AND media_type='' AND media_caption='' AND reply_text='' AND reply_text_by_chat_type='{}'::jsonb AND jsonb_typeof(media_album_items)='array' AND jsonb_array_length(media_album_items) BETWEEN 2 AND 10)
     OR (action_type='voice_call' AND contact_phone='' AND contact_first_name='' AND contact_last_name='' AND media_asset_id IS NULL AND media_type='' AND media_caption='' AND media_album_items='[]'::jsonb AND reply_text='' AND reply_text_by_chat_type='{}'::jsonb)
    );

ALTER TABLE keyword_action_jobs
    DROP CONSTRAINT IF EXISTS keyword_action_jobs_action_type_check,
    ADD CONSTRAINT keyword_action_jobs_action_type_check
        CHECK (action_type IN ('text','contact','media','media_album','voice_call')),
    DROP CONSTRAINT IF EXISTS keyword_action_jobs_action_payload_check,
    ADD CONSTRAINT keyword_action_jobs_action_payload_check CHECK (
        (action_type='text' AND length(message_text) BETWEEN 1 AND 4096 AND contact_phone='' AND contact_first_name='' AND contact_last_name='' AND media_asset_id IS NULL AND media_type='' AND media_caption='' AND media_album_items='[]'::jsonb AND target_user_id=0)
     OR (action_type='contact' AND message_text='' AND length(contact_phone) BETWEEN 1 AND 64 AND length(contact_first_name) BETWEEN 1 AND 128 AND length(contact_last_name)<=128 AND media_asset_id IS NULL AND media_type='' AND media_caption='' AND media_album_items='[]'::jsonb AND target_user_id=0)
     OR (action_type='media' AND message_text='' AND contact_phone='' AND contact_first_name='' AND contact_last_name='' AND media_asset_id IS NOT NULL AND media_type IN ('photo','video','document','audio','voice','video_note') AND length(media_caption)<=4096 AND media_album_items='[]'::jsonb AND target_user_id=0)
     OR (action_type='media_album' AND message_text='' AND contact_phone='' AND contact_first_name='' AND contact_last_name='' AND media_asset_id IS NULL AND media_type='' AND media_caption='' AND jsonb_typeof(media_album_items)='array' AND jsonb_array_length(media_album_items) BETWEEN 2 AND 10 AND target_user_id=0)
     OR (action_type='voice_call' AND message_text='' AND contact_phone='' AND contact_first_name='' AND contact_last_name='' AND media_asset_id IS NULL AND media_type='' AND media_caption='' AND media_album_items='[]'::jsonb AND target_user_id>0)
    );

CREATE INDEX IF NOT EXISTS keyword_action_jobs_voice_call_target_idx
    ON keyword_action_jobs(tenant_id,target_user_id,created_at)
    WHERE action_type='voice_call';
