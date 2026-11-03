ALTER TABLE tenant_keyword_rules
    ADD COLUMN IF NOT EXISTS reply_text_by_chat_type JSONB NOT NULL DEFAULT '{}'::jsonb
        CHECK (jsonb_typeof(reply_text_by_chat_type)='object');
