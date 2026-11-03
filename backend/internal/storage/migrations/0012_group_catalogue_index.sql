-- Support tenant-scoped catalogue scans and stable pagination.
DROP INDEX IF EXISTS tenant_message_chats_recent_idx;
CREATE INDEX tenant_message_chats_recent_idx
  ON tenant_message_chats(tenant_id, last_seen_at DESC, chat_title ASC, telegram_account_id ASC, chat_id ASC)
  INCLUDE (chat_type, username, member_count, is_verified);
