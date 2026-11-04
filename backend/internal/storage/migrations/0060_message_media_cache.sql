-- Cache downloaded Telegram attachments by tenant and source message.
-- The binary payload remains in tenant_media_assets; this table only links the
-- source Telegram message to the tenant-owned, SHA-256 deduplicated asset.
CREATE TABLE IF NOT EXISTS message_media_cache (
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    telegram_account_id UUID NOT NULL,
    chat_id BIGINT NOT NULL,
    message_id BIGINT NOT NULL,
    asset_id UUID NOT NULL REFERENCES tenant_media_assets(id) ON DELETE CASCADE,
    cached_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (tenant_id, telegram_account_id, chat_id, message_id)
);

CREATE INDEX IF NOT EXISTS message_media_cache_asset_idx
    ON message_media_cache (tenant_id, asset_id);

COMMENT ON TABLE message_media_cache IS
    'Tenant-scoped cache index from Telegram message identity to a deduplicated tenant_media_assets binary object.';
