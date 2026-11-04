package messages

import (
    "context"
    "errors"
    "testing"
)

func TestMessageMediaCacheDeduplicatesWithinTenantAndScopesSource(t *testing.T) {
    db,cleanup:=openGroupIntegrationDB(t)
    defer cleanup()
    ctx:=context.Background()
    tenantID:="11111111-1111-4111-8111-111111111111"
    accountID:="aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
    if _,err:=db.ExecContext(ctx,`CREATE TABLE tenant_media_assets (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),tenant_id UUID NOT NULL,file_name TEXT NOT NULL,
        mime_type TEXT NOT NULL,file_size BIGINT NOT NULL,sha256 BYTEA NOT NULL,content BYTEA NOT NULL,
        created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),UNIQUE(tenant_id,sha256))`);err!=nil{t.Fatal(err)}
    if _,err:=db.ExecContext(ctx,`CREATE TABLE message_media_cache (
        tenant_id UUID NOT NULL,telegram_account_id UUID NOT NULL,chat_id BIGINT NOT NULL,message_id BIGINT NOT NULL,
        asset_id UUID NOT NULL REFERENCES tenant_media_assets(id),cached_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
        PRIMARY KEY(tenant_id,telegram_account_id,chat_id,message_id))`);err!=nil{t.Fatal(err)}
    if _,err:=db.ExecContext(ctx,`INSERT INTO tenants(id,status,expires_at) VALUES($1,'active',NOW()+INTERVAL '30 days')`,tenantID);err!=nil{t.Fatal(err)}

    service:=New(db)
    payload:=[]byte("telegram-media-payload")
    first,err:=service.CacheMessageMedia(ctx,tenantID,accountID,101,501,"photo.jpg","image/jpeg",payload)
    if err!=nil{t.Fatal(err)}
    second,err:=service.CacheMessageMedia(ctx,tenantID,accountID,101,502,"photo-copy.jpg","image/jpeg",payload)
    if err!=nil{t.Fatal(err)}
    if first.ID!=second.ID||first.SHA256!=second.SHA256{t.Fatalf("identical bytes should deduplicate: first=%#v second=%#v",first,second)}
    var assets int
    if err:=db.QueryRowContext(ctx,`SELECT COUNT(*) FROM tenant_media_assets WHERE tenant_id=$1`,tenantID).Scan(&assets);err!=nil{t.Fatal(err)}
    if assets!=1{t.Fatalf("asset rows=%d, want one tenant-deduplicated object",assets)}

    cached,err:=service.GetCachedMessageMedia(ctx,tenantID,accountID,101,501)
    if err!=nil{t.Fatal(err)}
    if cached.ID!=first.ID||string(cached.Content)!=string(payload){t.Fatalf("cached media mismatch: %#v",cached)}
    _,err=service.GetCachedMessageMedia(ctx,tenantID,accountID,999,501)
    if !errors.Is(err,ErrMediaAssetNotFound){t.Fatalf("wrong chat cache lookup err=%v, want not found",err)}
}
