ALTER TABLE tenant_keyword_rules
    ADD COLUMN IF NOT EXISTS chat_titles JSONB NOT NULL DEFAULT '[]'::jsonb
        CHECK (jsonb_typeof(chat_titles) = 'array'),
    ADD COLUMN IF NOT EXISTS chat_usernames JSONB NOT NULL DEFAULT '[]'::jsonb
        CHECK (jsonb_typeof(chat_usernames) = 'array'),
    ADD COLUMN IF NOT EXISTS sender_ids JSONB NOT NULL DEFAULT '[]'::jsonb
        CHECK (jsonb_typeof(sender_ids) = 'array'),
    ADD COLUMN IF NOT EXISTS sender_usernames JSONB NOT NULL DEFAULT '[]'::jsonb
        CHECK (jsonb_typeof(sender_usernames) = 'array'),
    ADD COLUMN IF NOT EXISTS sender_names JSONB NOT NULL DEFAULT '[]'::jsonb
        CHECK (jsonb_typeof(sender_names) = 'array');
