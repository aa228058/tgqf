-- Persist ordered album payloads on rules and queued keyword actions.
ALTER TABLE tenant_keyword_rules
    ADD COLUMN IF NOT EXISTS media_album_items JSONB NOT NULL DEFAULT '[]'::jsonb;
ALTER TABLE keyword_action_jobs
    ADD COLUMN IF NOT EXISTS media_album_items JSONB NOT NULL DEFAULT '[]'::jsonb;

ALTER TABLE tenant_keyword_rules DROP CONSTRAINT IF EXISTS tenant_keyword_rules_action_type_check;
ALTER TABLE tenant_keyword_rules ADD CONSTRAINT tenant_keyword_rules_action_type_check
    CHECK (action_type IN ('text','contact','media','media_album'));
ALTER TABLE tenant_keyword_rules DROP CONSTRAINT IF EXISTS tenant_keyword_rules_action_payload_check;
ALTER TABLE tenant_keyword_rules ADD CONSTRAINT tenant_keyword_rules_action_payload_check CHECK (
    (action_type='text' AND contact_phone='' AND contact_first_name='' AND contact_last_name='' AND media_asset_id IS NULL AND media_type='' AND media_caption='' AND media_album_items='[]'::jsonb)
 OR (action_type='contact' AND length(contact_phone) BETWEEN 1 AND 64 AND length(contact_first_name) BETWEEN 1 AND 128 AND length(contact_last_name)<=128 AND reply_text='' AND reply_text_by_chat_type='{}'::jsonb AND media_asset_id IS NULL AND media_type='' AND media_caption='' AND media_album_items='[]'::jsonb)
 OR (action_type='media' AND contact_phone='' AND contact_first_name='' AND contact_last_name='' AND media_asset_id IS NOT NULL AND media_type IN ('photo','video','document','audio','voice','video_note') AND length(media_caption)<=4096 AND reply_text='' AND reply_text_by_chat_type='{}'::jsonb AND media_album_items='[]'::jsonb)
 OR (action_type='media_album' AND contact_phone='' AND contact_first_name='' AND contact_last_name='' AND media_asset_id IS NULL AND media_type='' AND media_caption='' AND reply_text='' AND reply_text_by_chat_type='{}'::jsonb AND jsonb_typeof(media_album_items)='array' AND jsonb_array_length(media_album_items) BETWEEN 2 AND 10)
);

ALTER TABLE keyword_action_jobs DROP CONSTRAINT IF EXISTS keyword_action_jobs_action_type_check;
ALTER TABLE keyword_action_jobs ADD CONSTRAINT keyword_action_jobs_action_type_check
    CHECK (action_type IN ('text','contact','media','media_album'));
ALTER TABLE keyword_action_jobs DROP CONSTRAINT IF EXISTS keyword_action_jobs_action_payload_check;
ALTER TABLE keyword_action_jobs ADD CONSTRAINT keyword_action_jobs_action_payload_check CHECK (
    (action_type='text' AND length(message_text) BETWEEN 1 AND 4096 AND contact_phone='' AND contact_first_name='' AND contact_last_name='' AND media_asset_id IS NULL AND media_type='' AND media_caption='' AND media_album_items='[]'::jsonb)
 OR (action_type='contact' AND message_text='' AND length(contact_phone) BETWEEN 1 AND 64 AND length(contact_first_name) BETWEEN 1 AND 128 AND length(contact_last_name)<=128 AND media_asset_id IS NULL AND media_type='' AND media_caption='' AND media_album_items='[]'::jsonb)
 OR (action_type='media' AND message_text='' AND contact_phone='' AND contact_first_name='' AND contact_last_name='' AND media_asset_id IS NOT NULL AND media_type IN ('photo','video','document','audio','voice','video_note') AND length(media_caption)<=4096 AND media_album_items='[]'::jsonb)
 OR (action_type='media_album' AND message_text='' AND contact_phone='' AND contact_first_name='' AND contact_last_name='' AND media_asset_id IS NULL AND media_type='' AND media_caption='' AND jsonb_typeof(media_album_items)='array' AND jsonb_array_length(media_album_items) BETWEEN 2 AND 10)
);

-- Keep every album asset tenant-owned and prevent deletion while a rule or job references it.
CREATE OR REPLACE FUNCTION enforce_keyword_album_assets_tenant() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE item JSONB;
BEGIN
  IF NEW.action_type='media_album' THEN
    FOR item IN SELECT value FROM jsonb_array_elements(NEW.media_album_items) LOOP
      IF NOT EXISTS (SELECT 1 FROM tenant_media_assets a WHERE a.tenant_id=NEW.tenant_id AND a.id=(item->>'asset_id')::uuid) THEN
        RAISE EXCEPTION 'keyword album asset does not belong to tenant';
      END IF;
      IF item->>'media_type' NOT IN ('photo','video','document') THEN RAISE EXCEPTION 'invalid album media type'; END IF;
    END LOOP;
  END IF;
  RETURN NEW;
END $$;
DROP TRIGGER IF EXISTS tenant_keyword_rules_album_assets_tenant ON tenant_keyword_rules;
CREATE TRIGGER tenant_keyword_rules_album_assets_tenant BEFORE INSERT OR UPDATE OF media_album_items,action_type ON tenant_keyword_rules FOR EACH ROW EXECUTE FUNCTION enforce_keyword_album_assets_tenant();
DROP TRIGGER IF EXISTS keyword_action_jobs_album_assets_tenant ON keyword_action_jobs;
CREATE TRIGGER keyword_action_jobs_album_assets_tenant BEFORE INSERT OR UPDATE OF media_album_items,action_type ON keyword_action_jobs FOR EACH ROW EXECUTE FUNCTION enforce_keyword_album_assets_tenant();

CREATE INDEX IF NOT EXISTS keyword_action_jobs_album_idx ON keyword_action_jobs(tenant_id) WHERE action_type='media_album';

-- Protect referenced album files from deletion, including references embedded in JSONB.
CREATE OR REPLACE FUNCTION prevent_referenced_keyword_album_asset_delete() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM tenant_keyword_rules r
    WHERE r.tenant_id=OLD.tenant_id AND r.action_type='media_album'
      AND EXISTS (SELECT 1 FROM jsonb_array_elements(r.media_album_items) item WHERE item->>'asset_id'=OLD.id::text)
  ) OR EXISTS (
    SELECT 1 FROM keyword_action_jobs j
    WHERE j.tenant_id=OLD.tenant_id AND j.action_type='media_album'
      AND EXISTS (SELECT 1 FROM jsonb_array_elements(j.media_album_items) item WHERE item->>'asset_id'=OLD.id::text)
  ) THEN
    RAISE EXCEPTION 'media asset is referenced by a keyword album';
  END IF;
  RETURN OLD;
END $$;
DROP TRIGGER IF EXISTS protect_keyword_album_asset_delete ON tenant_media_assets;
CREATE TRIGGER protect_keyword_album_asset_delete BEFORE DELETE ON tenant_media_assets
FOR EACH ROW EXECUTE FUNCTION prevent_referenced_keyword_album_asset_delete();
