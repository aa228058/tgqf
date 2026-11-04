-- Connect keyword rules and queued actions to tenant-owned media assets.
ALTER TABLE tenant_media_assets
    ADD CONSTRAINT tenant_media_assets_tenant_id_id_key UNIQUE (tenant_id,id);

ALTER TABLE tenant_keyword_rules
    ADD COLUMN IF NOT EXISTS media_asset_id UUID,
    ADD COLUMN IF NOT EXISTS media_type TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS media_caption TEXT NOT NULL DEFAULT '';

ALTER TABLE tenant_keyword_rules
    DROP CONSTRAINT IF EXISTS tenant_keyword_rules_action_type_check,
    DROP CONSTRAINT IF EXISTS tenant_keyword_rules_contact_config_check,
    ADD CONSTRAINT tenant_keyword_rules_action_type_check CHECK (action_type IN ('text','contact','media')),
    ADD CONSTRAINT tenant_keyword_rules_action_payload_check CHECK (
        (action_type='text' AND contact_phone='' AND contact_first_name='' AND contact_last_name='' AND media_asset_id IS NULL AND media_type='' AND media_caption='')
        OR (action_type='contact' AND length(contact_phone) BETWEEN 1 AND 64 AND length(contact_first_name) BETWEEN 1 AND 128 AND length(contact_last_name)<=128 AND reply_text='' AND reply_text_by_chat_type='{}'::jsonb AND media_asset_id IS NULL AND media_type='' AND media_caption='')
        OR (action_type='media' AND contact_phone='' AND contact_first_name='' AND contact_last_name='' AND media_asset_id IS NOT NULL AND media_type IN ('photo','video','document','audio','voice','video_note') AND length(media_caption)<=4096 AND reply_text='' AND reply_text_by_chat_type='{}'::jsonb)
    ),
    ADD CONSTRAINT tenant_keyword_rules_media_asset_fk FOREIGN KEY (tenant_id,media_asset_id) REFERENCES tenant_media_assets(tenant_id,id) ON DELETE RESTRICT;

ALTER TABLE keyword_action_jobs
    ADD COLUMN IF NOT EXISTS media_asset_id UUID,
    ADD COLUMN IF NOT EXISTS media_type TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS media_caption TEXT NOT NULL DEFAULT '';

ALTER TABLE keyword_action_jobs
    DROP CONSTRAINT IF EXISTS keyword_action_jobs_action_type_check,
    DROP CONSTRAINT IF EXISTS keyword_action_jobs_action_payload_check,
    ADD CONSTRAINT keyword_action_jobs_action_type_check CHECK (action_type IN ('text','contact','media')),
    ADD CONSTRAINT keyword_action_jobs_action_payload_check CHECK (
        (action_type='text' AND length(message_text) BETWEEN 1 AND 4096 AND contact_phone='' AND contact_first_name='' AND contact_last_name='' AND media_asset_id IS NULL AND media_type='' AND media_caption='')
        OR (action_type='contact' AND message_text='' AND length(contact_phone) BETWEEN 1 AND 64 AND length(contact_first_name) BETWEEN 1 AND 128 AND length(contact_last_name)<=128 AND media_asset_id IS NULL AND media_type='' AND media_caption='')
        OR (action_type='media' AND message_text='' AND contact_phone='' AND contact_first_name='' AND contact_last_name='' AND media_asset_id IS NOT NULL AND media_type IN ('photo','video','document','audio','voice','video_note') AND length(media_caption)<=4096)
    ),
    ADD CONSTRAINT keyword_action_jobs_media_asset_fk FOREIGN KEY (tenant_id,media_asset_id) REFERENCES tenant_media_assets(tenant_id,id) ON DELETE RESTRICT;

CREATE INDEX IF NOT EXISTS keyword_action_jobs_media_asset_idx ON keyword_action_jobs(tenant_id,media_asset_id) WHERE media_asset_id IS NOT NULL;
