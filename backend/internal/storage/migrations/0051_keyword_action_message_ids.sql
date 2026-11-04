-- Preserve every Telegram message ID returned by multi-message actions (albums).
ALTER TABLE keyword_action_jobs
    ADD COLUMN IF NOT EXISTS telegram_message_ids JSONB NOT NULL DEFAULT '[]'::jsonb;

ALTER TABLE keyword_action_jobs
    ADD CONSTRAINT keyword_action_jobs_telegram_message_ids_array_check
    CHECK (jsonb_typeof(telegram_message_ids) = 'array');

-- Backfill historical single-message results without changing the legacy column.
UPDATE keyword_action_jobs
SET telegram_message_ids = jsonb_build_array(telegram_message_id)
WHERE telegram_message_id IS NOT NULL
  AND telegram_message_ids = '[]'::jsonb;
