ALTER TABLE tenant_keyword_rules
    ADD COLUMN IF NOT EXISTS keyword_groups JSONB NOT NULL DEFAULT '[]'::jsonb
        CHECK (jsonb_typeof(keyword_groups)='array'),
    ADD COLUMN IF NOT EXISTS group_operator TEXT NOT NULL DEFAULT 'any'
        CHECK (group_operator IN ('any','all'));
