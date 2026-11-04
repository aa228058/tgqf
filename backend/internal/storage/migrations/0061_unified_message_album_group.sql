-- Telegram sends album members as individual messages sharing GroupedID.
-- Preserve that identity so inbox clients can render and paginate albums together.
ALTER TABLE unified_messages
    ADD COLUMN IF NOT EXISTS grouped_id BIGINT;

CREATE INDEX IF NOT EXISTS unified_messages_album_group_idx
    ON unified_messages (tenant_id, telegram_account_id, chat_id, grouped_id, message_date)
    WHERE grouped_id IS NOT NULL AND grouped_id <> 0;

COMMENT ON COLUMN unified_messages.grouped_id IS
    'MTGo Telegram GroupedID; non-zero values identify messages belonging to one media album.';
