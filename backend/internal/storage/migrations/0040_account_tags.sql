ALTER TABLE telegram_accounts
    ADD COLUMN IF NOT EXISTS tags TEXT[] NOT NULL DEFAULT '{}';

CREATE INDEX IF NOT EXISTS telegram_accounts_tags_gin_idx
    ON telegram_accounts USING GIN (tags);
