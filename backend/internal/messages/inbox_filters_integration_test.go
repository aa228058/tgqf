package messages

import (
	"context"
	"testing"
	"time"
)

func TestMessageListSupportsTenantScopedInboxFilters(t *testing.T) {
	db,cleanup:=openGroupIntegrationDB(t)
	defer cleanup()
	ctx:=context.Background()
	tenantID:="11111111-1111-4111-8111-111111111111"
	accountA:="aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	accountB:="bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	if _,err:=db.ExecContext(ctx,`CREATE TABLE unified_messages (
		id UUID PRIMARY KEY,tenant_id UUID NOT NULL,telegram_account_id UUID NOT NULL,chat_id BIGINT NOT NULL,
		message_id BIGINT NOT NULL,chat_type TEXT NOT NULL,chat_title TEXT NOT NULL DEFAULT '',sender_id BIGINT NOT NULL DEFAULT 0,
		reply_to_message_id BIGINT,message_text TEXT NOT NULL DEFAULT '',media_type TEXT NOT NULL DEFAULT '',
		media_metadata JSONB NOT NULL DEFAULT '{}'::jsonb,message_date TIMESTAMPTZ NOT NULL,processed_at TIMESTAMPTZ)`);err!=nil{t.Fatal(err)}
	if _,err:=db.ExecContext(ctx,`INSERT INTO tenants(id,status,expires_at) VALUES($1,'active',NOW()+INTERVAL '30 days')`,tenantID);err!=nil{t.Fatal(err)}
	for _,a:=range []struct{id,phone,name string}{{accountA,"+10000000001","Alpha"},{accountB,"+10000000002","Beta"}}{
		if _,err:=db.ExecContext(ctx,`INSERT INTO telegram_accounts(tenant_id,id,status,session_ciphertext,phone,display_name) VALUES($1,$2,'authorized',decode('01','hex'),$3,$4)`,tenantID,a.id,a.phone,a.name);err!=nil{t.Fatal(err)}
	}
	now:=time.Now().UTC()
	rows:=[]struct{id,account string;chat int64;chatType,text,media string;processed bool}{
		{"10000000-0000-4000-8000-000000000001",accountA,101,"private","hello Alice","","{}",false},
		{"10000000-0000-4000-8000-000000000002",accountA,101,"private","photo caption","photo",`{"width":1280,"height":720,"file_size":4096}`,false},
		{"10000000-0000-4000-8000-000000000003",accountB,202,"group","hello Bob","","{}",false},
		{"10000000-0000-4000-8000-000000000004",accountA,101,"private","old hello","","{}",true},
	}
	for i,row:=range rows{
		var processedAt any
		if row.processed{processedAt=now.Add(-time.Hour)}
		if _,err:=db.ExecContext(ctx,`INSERT INTO unified_messages(id,tenant_id,telegram_account_id,chat_id,message_id,chat_type,chat_title,message_text,media_type,media_metadata,message_date,processed_at)
			VALUES($1,$2,$3,$4,$5,$6,'Chat',$7,$8,$9::jsonb,$10,$11)`,row.id,tenantID,row.account,row.chat,i+1,row.chatType,row.text,row.media,row.metadata,now.Add(-time.Duration(i)*time.Minute),processedAt);err!=nil{t.Fatal(err)}
	}
	service:=New(db)
	items,total,err:=service.List(ctx,tenantID,ListOptions{Limit:10,AccountID:accountA,ChatID:101,ChatType:"private",Query:"hello"})
	if err!=nil{t.Fatal(err)}
	if total!=1||len(items)!=1||items[0].Text!="hello Alice"{t.Fatalf("filtered inbox result total=%d items=%#v",total,items)}
	items,total,err=service.List(ctx,tenantID,ListOptions{Limit:10,AccountID:accountA,ChatID:101,ChatType:"private",MediaType:"photo"})
	if err!=nil{t.Fatal(err)}
	if total!=1||len(items)!=1||items[0].MediaType!="photo"{t.Fatalf("media filter result total=%d items=%#v",total,items)}
	if string(items[0].MediaMetadata)!=`{"width":1280,"height":720,"file_size":4096}`{t.Fatalf("media metadata=%s, want persisted dimensions and file size",items[0].MediaMetadata)}
	items,total,err=service.List(ctx,tenantID,ListOptions{Limit:10,AccountID:accountA,ChatID:101,IncludeProcessed:true})
	if err!=nil{t.Fatal(err)}
	if total!=3||len(items)!=3{t.Fatalf("include_processed result total=%d items=%d, want 3",total,len(items))}
	if _,_,err:=service.List(ctx,tenantID,ListOptions{Limit:10,AccountID:"not-a-uuid"});err==nil{t.Fatal("invalid account filter should be rejected")}
}
