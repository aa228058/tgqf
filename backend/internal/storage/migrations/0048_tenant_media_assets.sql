CREATE TABLE IF NOT EXISTS tenant_media_assets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    file_name TEXT NOT NULL CHECK (length(file_name) BETWEEN 1 AND 255),
    mime_type TEXT NOT NULL CHECK (length(mime_type) BETWEEN 1 AND 160),
    file_size BIGINT NOT NULL CHECK (file_size BETWEEN 1 AND 20971520),
    sha256 BYTEA NOT NULL CHECK (octet_length(sha256)=32),
    content BYTEA NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, sha256)
);
ALTER TABLE tenant_media_assets ENABLE ROW LEVEL SECURITY;
ALTER TABLE tenant_media_assets FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_media_assets_tenant_policy ON tenant_media_assets
    USING (tenant_id = NULLIF(current_setting('app.tenant_id',true),'')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id',true),'')::uuid);
CREATE INDEX IF NOT EXISTS tenant_media_assets_tenant_created_idx
    ON tenant_media_assets(tenant_id,created_at DESC);
