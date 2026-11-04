package messages

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestKeywordActionClaimAndTenantExpiryPreserveLiveRPC(t *testing.T) {
	db,cleanup:=openGroupIntegrationDB(t)
	defer cleanup()
	ctx:=context.Background()
	activeTenant:="11111111-1111-4111-8111-111111111111"
	inactiveTenant:="22222222-2222-4222-8222-222222222222"
	accountA:="aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	accountB:="bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	accountRevoked:="cccccccc-cccc-4ccc-8ccc-cccccccccccc"
	inactivePendingAccount:="dddddddd-dddd-4ddd-8ddd-dddddddddddd"
	inactiveExpiredAccount:="eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee"
	inactiveLiveAccount:="ffffffff-ffff-4fff-8fff-ffffffffffff"
	ddl:=[]string{
		`CREATE TABLE message_reply_jobs (tenant_id UUID,telegram_account_id UUID,status TEXT,lease_until TIMESTAMPTZ)`,
		`CREATE TABLE account_profile_update_jobs (tenant_id UUID,account_id UUID,status TEXT,lease_until TIMESTAMPTZ)`,
		`CREATE TABLE keyword_action_jobs (
			id UUID PRIMARY KEY,tenant_id UUID NOT NULL,telegram_account_id UUID NOT NULL,
			chat_id BIGINT NOT NULL DEFAULT 1,reply_to_message_id BIGINT NOT NULL DEFAULT 1,
			message_text TEXT NOT NULL DEFAULT '',action_type TEXT NOT NULL DEFAULT 'text',
			contact_phone TEXT NOT NULL DEFAULT '',contact_first_name TEXT NOT NULL DEFAULT '',contact_last_name TEXT NOT NULL DEFAULT '',
			media_asset_id UUID,media_type TEXT NOT NULL DEFAULT '',media_caption TEXT NOT NULL DEFAULT '',
			media_album_items JSONB NOT NULL DEFAULT '[]'::jsonb,attempts INTEGER NOT NULL DEFAULT 0,
			status TEXT NOT NULL DEFAULT 'pending',available_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			lease_until TIMESTAMPTZ,last_error TEXT NOT NULL DEFAULT '',telegram_message_id BIGINT,
			telegram_message_ids JSONB NOT NULL DEFAULT '[]'::jsonb,created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW())`,
	}
	for _,statement:=range ddl{if _,err:=db.ExecContext(ctx,statement);err!=nil{t.Fatal(err)}}
	for _,tenant:=range []struct{id,status string;expires string}{
		{activeTenant,"active","30 days"},{inactiveTenant,"expired","-1 hour"},
	}{if _,err:=db.ExecContext(ctx,`INSERT INTO tenants(id,status,expires_at) VALUES($1,$2,NOW()+$3::interval)`,tenant.id,tenant.status,tenant.expires);err!=nil{t.Fatal(err)}}
	accounts:=[]struct{tenant,id,status string;session bool}{
		{activeTenant,accountA,"authorized",true},{activeTenant,accountB,"authorized",true},{activeTenant,accountRevoked,"revoked",false},
		{inactiveTenant,inactivePendingAccount,"authorized",true},{inactiveTenant,inactiveExpiredAccount,"authorized",true},{inactiveTenant,inactiveLiveAccount,"authorized",true},
	}
	for _,a:=range accounts{
		var session any
		if a.session{session=[]byte{1}}
		if _,err:=db.ExecContext(ctx,`INSERT INTO telegram_accounts(tenant_id,id,status,session_ciphertext) VALUES($1,$2,$3,$4)`,a.tenant,a.id,a.status,session);err!=nil{t.Fatal(err)}
	}
	insertJob:=func(id,tenant,account,status string,lease time.Time){
		t.Helper()
		var leaseValue any
		if !lease.IsZero(){leaseValue=lease}
		if _,err:=db.ExecContext(ctx,`INSERT INTO keyword_action_jobs(id,tenant_id,telegram_account_id,status,lease_until,message_text)
			VALUES($1,$2,$3,$4,$5,'hello')`,id,tenant,account,status,leaseValue);err!=nil{t.Fatal(err)}
	}
	jobA:="10000000-0000-4000-8000-000000000001"
	jobB:="10000000-0000-4000-8000-000000000002"
	jobRevoked:="10000000-0000-4000-8000-000000000003"
	jobInactivePending:="20000000-0000-4000-8000-000000000001"
	jobInactiveExpired:="20000000-0000-4000-8000-000000000002"
	jobInactiveLive:="20000000-0000-4000-8000-000000000003"
	insertJob(jobA,activeTenant,accountA,"pending",time.Time{})
	insertJob(jobB,activeTenant,accountB,"pending",time.Time{})
	insertJob(jobRevoked,activeTenant,accountRevoked,"pending",time.Time{})
	insertJob(jobInactivePending,inactiveTenant,inactivePendingAccount,"pending",time.Time{})
	insertJob(jobInactiveExpired,inactiveTenant,inactiveExpiredAccount,"processing",time.Now().Add(-time.Minute))
	liveLease:=time.Now().Add(2*time.Minute)
	insertJob(jobInactiveLive,inactiveTenant,inactiveLiveAccount,"processing",liveLease)

	service:=New(db)
	jobs,err:=service.ClaimKeywordActionJobs(ctx,10)
	if err!=nil{t.Fatal(err)}
	if len(jobs)!=2{details:=make([]string,0,len(jobs));for _,job:=range jobs{details=append(details,job.ID+"/"+job.TenantID+"/"+job.AccountID)};var tenantRows string;_ = db.QueryRowContext(ctx,`SELECT string_agg(id::text||':'||status||':'||expires_at::text,',') FROM tenants`).Scan(&tenantRows);t.Fatalf("claimed %d jobs (%v), tenants=%s, want only the 2 authorized active-tenant accounts",len(jobs),details,tenantRows)}
	claimed:=map[string]KeywordActionJob{}
	for _,job:=range jobs{claimed[job.ID]=job}
	if _,ok:=claimed[jobA];!ok{t.Fatal("authorized account A job was not claimed")}
	if _,ok:=claimed[jobB];!ok{t.Fatal("authorized account B job was not claimed")}
	for _,id:=range []string{jobRevoked,jobInactivePending,jobInactiveExpired,jobInactiveLive}{
		var status string
		if err:=db.QueryRowContext(ctx,`SELECT status FROM keyword_action_jobs WHERE id=$1`,id).Scan(&status);err!=nil{t.Fatal(err)}
		want:="pending"
		if id==jobInactivePending||id==jobInactiveExpired{want="paused"}
		if id==jobInactiveLive{want="processing"}
		if status!=want{t.Fatalf("job %s status=%q, want %q",id,status,want)}
	}
	var savedLease time.Time
	if err:=db.QueryRowContext(ctx,`SELECT lease_until FROM keyword_action_jobs WHERE id=$1`,jobInactiveLive).Scan(&savedLease);err!=nil{t.Fatal(err)}
	if !savedLease.Equal(liveLease){t.Fatalf("inactive tenant sweep changed live lease: %s != %s",savedLease,liveLease)}

	// Subscription expiry after Telegram has accepted the send must not erase
	// the successful result, while an in-flight failure is paused, not retried.
	if _,err:=db.ExecContext(ctx,`UPDATE tenants SET status='expired',expires_at=NOW()-INTERVAL '1 second' WHERE id=$1`,activeTenant);err!=nil{t.Fatal(err)}
	if err:=service.FinishKeywordAction(ctx,claimed[jobA],501,nil,0,[]int64{501});err!=nil{t.Fatalf("persist successful send after tenant expiry: %v",err)}
	if err:=service.FinishKeywordAction(ctx,claimed[jobB],0,errors.New("temporary Telegram failure"),0);err!=nil{t.Fatalf("pause failed send after tenant expiry: %v",err)}
	for id,want:=range map[string]string{jobA:"sent",jobB:"paused"}{
		var status string
		if err:=db.QueryRowContext(ctx,`SELECT status FROM keyword_action_jobs WHERE id=$1`,id).Scan(&status);err!=nil{t.Fatal(err)}
		if status!=want{t.Fatalf("job %s status=%q, want %q",id,status,want)}
	}
}
