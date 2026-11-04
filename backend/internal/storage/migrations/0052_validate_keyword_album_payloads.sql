-- Enforce the complete ordered album contract at the database boundary.
-- The application validates the same rules, but triggers also protect direct SQL writers.
CREATE OR REPLACE FUNCTION enforce_keyword_album_assets_tenant() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
  item JSONB;
  asset_id UUID;
  media_category TEXT := '';
  item_category TEXT;
  seen_assets UUID[] := ARRAY[]::UUID[];
  caption_text TEXT;
BEGIN
  IF NEW.action_type <> 'media_album' THEN
    IF NEW.media_album_items <> '[]'::jsonb THEN
      RAISE EXCEPTION 'media album items require media_album action';
    END IF;
    RETURN NEW;
  END IF;

  IF jsonb_typeof(NEW.media_album_items) <> 'array'
     OR jsonb_array_length(NEW.media_album_items) NOT BETWEEN 2 AND 10 THEN
    RAISE EXCEPTION 'keyword media album must contain 2 to 10 items';
  END IF;

  FOR item IN SELECT value FROM jsonb_array_elements(NEW.media_album_items) AS elements(value) LOOP
    IF jsonb_typeof(item) <> 'object'
       OR NOT (item ? 'asset_id')
       OR NOT (item ? 'media_type')
       OR (item ? 'caption' AND jsonb_typeof(item->'caption') <> 'string')
       OR jsonb_typeof(item->'asset_id') <> 'string'
       OR jsonb_typeof(item->'media_type') <> 'string' THEN
      RAISE EXCEPTION 'invalid keyword album item structure';
    END IF;

    IF COALESCE(item->>'asset_id','') !~* '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$' THEN
      RAISE EXCEPTION 'invalid keyword album asset id';
    END IF;
    asset_id := (item->>'asset_id')::uuid;

    IF item->>'media_type' NOT IN ('photo','video','document') THEN
      RAISE EXCEPTION 'invalid keyword album media type';
    END IF;
    item_category := CASE WHEN item->>'media_type'='document' THEN 'document' ELSE 'visual' END;
    IF media_category <> '' AND media_category <> item_category THEN
      RAISE EXCEPTION 'keyword albums cannot mix documents with photos/videos';
    END IF;
    media_category := item_category;

    caption_text := COALESCE(item->>'caption','');
    IF char_length(caption_text) > 1024 THEN
      RAISE EXCEPTION 'keyword album caption exceeds 1024 characters';
    END IF;

    IF asset_id = ANY(seen_assets) THEN
      RAISE EXCEPTION 'duplicate keyword album asset';
    END IF;
    seen_assets := array_append(seen_assets,asset_id);

    IF NOT EXISTS (
      SELECT 1 FROM tenant_media_assets a
      WHERE a.tenant_id=NEW.tenant_id AND a.id=asset_id
    ) THEN
      RAISE EXCEPTION 'keyword album asset does not belong to tenant';
    END IF;
  END LOOP;
  RETURN NEW;
END $$;

-- Recreate triggers so both rule writes and queued-job writes use the strict validator.
DROP TRIGGER IF EXISTS tenant_keyword_rules_album_assets_tenant ON tenant_keyword_rules;
CREATE TRIGGER tenant_keyword_rules_album_assets_tenant
BEFORE INSERT OR UPDATE OF media_album_items,action_type ON tenant_keyword_rules
FOR EACH ROW EXECUTE FUNCTION enforce_keyword_album_assets_tenant();

DROP TRIGGER IF EXISTS keyword_action_jobs_album_assets_tenant ON keyword_action_jobs;
CREATE TRIGGER keyword_action_jobs_album_assets_tenant
BEFORE INSERT OR UPDATE OF media_album_items,action_type ON keyword_action_jobs
FOR EACH ROW EXECUTE FUNCTION enforce_keyword_album_assets_tenant();
