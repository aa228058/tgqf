ALTER TABLE tenant_keyword_rules
    ADD COLUMN IF NOT EXISTS account_ids JSONB NOT NULL DEFAULT '[]'::jsonb
        CHECK (jsonb_typeof(account_ids) = 'array'),
    ADD COLUMN IF NOT EXISTS chat_ids JSONB NOT NULL DEFAULT '[]'::jsonb
        CHECK (jsonb_typeof(chat_ids) = 'array'),
    ADD COLUMN IF NOT EXISTS media_types JSONB NOT NULL DEFAULT '[]'::jsonb
        CHECK (jsonb_typeof(media_types) = 'array');
