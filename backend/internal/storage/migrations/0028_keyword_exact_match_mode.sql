ALTER TABLE tenant_keyword_rules
    DROP CONSTRAINT IF EXISTS tenant_keyword_rules_match_mode_check;

ALTER TABLE tenant_keyword_rules
    ADD CONSTRAINT tenant_keyword_rules_match_mode_check
    CHECK (match_mode IN ('any', 'all', 'exact', 'regex'));
