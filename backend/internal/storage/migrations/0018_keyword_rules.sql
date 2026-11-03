CREATE TABLE tenant_keyword_rules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),\n    UNIQUE(tenant_id, id),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name TEXT NOT NULL CHECK (length(name) BETWEEN 1 AND 80),
    match_mode TEXT NOT NULL CHECK (match_mode IN ('any','all','regex')),
    keywords JSONB NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(keywords)='array'),
    exclude_terms JSONB NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(exclude_terms)='array'),
    chat_types JSONB NOT NULL DEFAULT '["private","group","channel"]'::jsonb CHECK (jsonb_typeof(chat_types)='array'),
    case_sensitive BOOLEAN NOT NULL DEFAULT FALSE,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX tenant_keyword_rules_active_idx ON tenant_keyword_rules(tenant_id, enabled, created_at) WHERE enabled=TRUE;
ALTER TABLE tenant_keyword_rules ENABLE ROW LEVEL SECURITY;
ALTER TABLE tenant_keyword_rules FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_keyword_rules_tenant_policy ON tenant_keyword_rules
 USING (tenant_id = NULLIF(current_setting('app.tenant_id',true),'')::uuid)
 WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id',true),'')::uuid);

CREATE TABLE tenant_keyword_matches (
    id BIGSERIAL PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    rule_id UUID NOT NULL,
    telegram_account_id UUID NOT NULL,
    chat_id BIGINT NOT NULL,
    message_id BIGINT NOT NULL,
    matched_terms JSONB NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(matched_terms)='array'),
    matched_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (tenant_id, rule_id) REFERENCES tenant_keyword_rules(tenant_id, id) ON DELETE CASCADE,
    FOREIGN KEY (tenant_id, telegram_account_id) REFERENCES telegram_accounts(tenant_id, id) ON DELETE CASCADE,
    UNIQUE(rule_id, telegram_account_id, chat_id, message_id)
);
CREATE INDEX tenant_keyword_matches_recent_idx ON tenant_keyword_matches(tenant_id, matched_at DESC, id DESC);
CREATE INDEX tenant_keyword_matches_rule_idx ON tenant_keyword_matches(tenant_id, rule_id, matched_at DESC);
ALTER TABLE tenant_keyword_matches ENABLE ROW LEVEL SECURITY;
ALTER TABLE tenant_keyword_matches FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_keyword_matches_tenant_policy ON tenant_keyword_matches
 USING (tenant_id = NULLIF(current_setting('app.tenant_id',true),'')::uuid)
 WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id',true),'')::uuid);
