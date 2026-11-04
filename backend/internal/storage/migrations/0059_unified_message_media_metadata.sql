-- Preserve the structured MTGo media object alongside each collected message.
-- This stores Telegram metadata only; binary downloads remain an explicit,
-- separately authorized operation and are not performed in the update callback.
ALTER TABLE unified_messages
    ADD COLUMN IF NOT EXISTS media_metadata JSONB NOT NULL DEFAULT '{}'::jsonb;

COMMENT ON COLUMN unified_messages.media_metadata IS
    'Tenant-scoped structured metadata serialized from the MTGo message media object; does not contain downloaded binary content.';
